-- Reusable ("site") enrollment tokens: install as many devices as needed
-- with the same token until it expires or is explicitly revoked, instead
-- of generating one per device. Deliberate risk acceptance: anyone with
-- the token can enroll additional devices for as long as it's valid, but
-- the blast radius stays bounded to "an extra device shows up in the
-- list" — it grants no access beyond what enrolling a device normally
-- does (docs/AGENT.md §5).
ALTER TABLE enrollment_tokens ADD COLUMN is_reusable boolean NOT NULL DEFAULT false;
ALTER TABLE enrollment_tokens ADD COLUMN use_count integer NOT NULL DEFAULT 0;
ALTER TABLE enrollment_tokens ADD COLUMN last_used_at timestamptz;
