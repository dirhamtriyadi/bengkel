-- +goose Up
-- Automatic Fee Imposition is not available to every Merchant ID. Earlier
-- versions enabled it by default, which could leave all Snap channels disabled.
UPDATE settings
SET value = jsonb_set(value, '{automatic_fee}', 'false'::jsonb, true),
    updated_at = now()
WHERE key = 'payment.midtrans.channels'
  AND deleted_at IS NULL;

-- +goose Down
UPDATE settings
SET value = jsonb_set(value, '{automatic_fee}', 'true'::jsonb, true),
    updated_at = now()
WHERE key = 'payment.midtrans.channels'
  AND deleted_at IS NULL;
