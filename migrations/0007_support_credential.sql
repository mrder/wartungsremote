-- Dedicated local OS account ("remotewartung") for the SSH/RDP tunnel
-- feature — the tunnel only forwards raw network traffic to the
-- device's own existing SSH/RDP service, which has its own separate
-- login unrelated to our Ed25519 device identity. Without a known
-- account, the tunnel is useless without the customer's own credentials.
-- Password is encrypted at rest (see internal/support), never stored or
-- logged in plaintext.
CREATE TABLE device_support_credentials (
    device_id           uuid PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    username            text NOT NULL,
    password_ciphertext bytea NOT NULL,
    password_nonce      bytea NOT NULL,
    updated_at          timestamptz NOT NULL DEFAULT now()
);
