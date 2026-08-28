-- Single global Telegram bot configuration used to notify on newly-opened
-- alerts (internal/notify). id is a boolean singleton key (CHECK id)
-- so this table can only ever hold exactly one row.
CREATE TABLE telegram_config (
    id                  boolean PRIMARY KEY DEFAULT true CHECK (id),
    bot_token_ciphertext bytea NOT NULL,
    bot_token_nonce      bytea NOT NULL,
    chat_id              text NOT NULL,
    updated_at           timestamptz NOT NULL DEFAULT now()
);
