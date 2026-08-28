// Package notify implements outbound alert notifications. Telegram is the
// first (and currently only) channel — see docs/TODO.md for
// email/ntfy/webhook as possible future additions. Configuration is a
// single global bot (token + chat ID), not per-customer/per-rule, per V1
// simplification: one place to set it up, every newly-opened alert goes
// there.
package notify

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotConfigured = errors.New("notify: telegram not configured")

type TelegramRepo struct {
	pool *pgxpool.Pool
	key  []byte // 32 bytes, shared with auth.MFAStore / internal/support
}

func NewTelegramRepo(pool *pgxpool.Pool, key []byte) *TelegramRepo {
	return &TelegramRepo{pool: pool, key: key}
}

func (r *TelegramRepo) encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(r.key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func (r *TelegramRepo) decrypt(ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(r.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// Set stores (or replaces) the bot configuration. The token is encrypted
// at rest the same way the remote-support account password is
// (internal/support) — same key, same reasoning.
func (r *TelegramRepo) Set(ctx context.Context, botToken, chatID string) error {
	ciphertext, nonce, err := r.encrypt([]byte(botToken))
	if err != nil {
		return fmt.Errorf("notify: encrypt bot token: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO telegram_config (id, bot_token_ciphertext, bot_token_nonce, chat_id, updated_at)
		VALUES (true, $1, $2, $3, now())
		ON CONFLICT (id) DO UPDATE SET
			bot_token_ciphertext = EXCLUDED.bot_token_ciphertext,
			bot_token_nonce = EXCLUDED.bot_token_nonce,
			chat_id = EXCLUDED.chat_id,
			updated_at = now()
	`, ciphertext, nonce, chatID)
	if err != nil {
		return fmt.Errorf("notify: set telegram config: %w", err)
	}
	return nil
}

// Status is what the dashboard needs to show — never the bot token itself,
// which is write-only from the API's perspective (re-enter to change it).
type Status struct {
	Configured bool
	ChatID     string
	UpdatedAt  string
}

func (r *TelegramRepo) GetStatus(ctx context.Context) (Status, error) {
	var chatID, updatedAt string
	err := r.pool.QueryRow(ctx, `SELECT chat_id, updated_at::text FROM telegram_config WHERE id = true`).Scan(&chatID, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{Configured: false}, nil
	}
	if err != nil {
		return Status{}, fmt.Errorf("notify: get telegram status: %w", err)
	}
	return Status{Configured: true, ChatID: chatID, UpdatedAt: updatedAt}, nil
}

func (r *TelegramRepo) get(ctx context.Context) (botToken, chatID string, err error) {
	var ciphertext, nonce []byte
	err = r.pool.QueryRow(ctx, `SELECT bot_token_ciphertext, bot_token_nonce, chat_id FROM telegram_config WHERE id = true`).
		Scan(&ciphertext, &nonce, &chatID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotConfigured
	}
	if err != nil {
		return "", "", fmt.Errorf("notify: get telegram config: %w", err)
	}
	plaintext, err := r.decrypt(ciphertext, nonce)
	if err != nil {
		return "", "", fmt.Errorf("notify: decrypt bot token: %w", err)
	}
	return string(plaintext), chatID, nil
}

// SendMessage posts text to the configured chat via the Telegram Bot API.
// Returns ErrNotConfigured if no bot has been set up yet — callers (the
// alert engine) should treat that as "notifications are simply off", not
// an error worth logging loudly on every alert.
func (r *TelegramRepo) SendMessage(ctx context.Context, text string) error {
	botToken, chatID, err := r.get(ctx)
	if err != nil {
		return err
	}
	return sendTelegramMessage(ctx, botToken, chatID, text)
}

// SendTestMessage is like SendMessage but takes the bot token/chat ID
// directly, so the dashboard can verify a configuration before saving it.
func SendTestMessage(ctx context.Context, botToken, chatID, text string) error {
	return sendTelegramMessage(ctx, botToken, chatID, text)
}

func sendTelegramMessage(ctx context.Context, botToken, chatID, text string) error {
	body, err := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	if err != nil {
		return fmt.Errorf("notify: marshal telegram request: %w", err)
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: build telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify: send telegram message: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("notify: telegram api returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
