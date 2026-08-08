-- +goose Up
INSERT INTO settings (id, branch_id, key, value, is_public, created_at, updated_at)
SELECT gen_random_uuid(), b.id, 'integration.midtrans',
       '{"enabled":false,"environment":"sandbox","merchant_id":"","server_key_ciphertext":"","client_key_ciphertext":""}'::jsonb,
       false, now(), now()
FROM branches b
WHERE b.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM settings s
      WHERE s.branch_id = b.id
        AND s.key = 'integration.midtrans'
        AND s.deleted_at IS NULL
  );

-- +goose Down
DELETE FROM settings WHERE key = 'integration.midtrans';
