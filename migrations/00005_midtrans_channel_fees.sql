-- +goose Up
ALTER TABLE payments
    ADD COLUMN base_amount bigint NOT NULL DEFAULT 0 CHECK(base_amount >= 0),
    ADD COLUMN customer_fee bigint NOT NULL DEFAULT 0 CHECK(customer_fee >= 0),
    ADD COLUMN provider_fee bigint NOT NULL DEFAULT 0 CHECK(provider_fee >= 0),
    ADD COLUMN payment_channel varchar(50) NOT NULL DEFAULT '';

UPDATE payments
SET customer_fee = CASE
        WHEN method <> 'midtrans' THEN 0
        WHEN fee_bearer = 'customer' THEN fee
        WHEN fee_bearer = 'split' THEN CEIL(fee::numeric / 2)::bigint
        ELSE 0
    END;

UPDATE payments
SET base_amount = GREATEST(amount - customer_fee, 0),
    provider_fee = CASE WHEN method = 'midtrans' THEN fee ELSE 0 END,
    payment_channel = COALESCE(metadata->>'payment_type', '');

INSERT INTO accounts (id, branch_id, code, name, type, is_active)
SELECT gen_random_uuid(), b.id, '4201', 'Pemulihan Biaya Payment Gateway', 'revenue', true
FROM branches b
WHERE b.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM accounts a
      WHERE a.branch_id = b.id AND a.code = '4201' AND a.deleted_at IS NULL
  );

INSERT INTO settings (id, branch_id, key, value, is_public)
SELECT gen_random_uuid(), b.id, 'payment.midtrans.channels',
       '{
         "automatic_fee": true,
         "channels": [
           {"payment_type":"bca_va","label":"BCA Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},
           {"payment_type":"bni_va","label":"BNI Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},
           {"payment_type":"bri_va","label":"BRI Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},
           {"payment_type":"permata_va","label":"Permata Virtual Account","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},
           {"payment_type":"echannel","label":"Mandiri Bill Payment","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":4000,"tax_percentage":11},
           {"payment_type":"gopay","label":"GoPay","enabled":true,"customer_percentage":100,"fee_percentage":2,"fixed_fee":0,"tax_percentage":11},
           {"payment_type":"qris","acquirer":"gopay","label":"QRIS","enabled":true,"customer_percentage":100,"fee_percentage":0.7,"fixed_fee":0,"tax_percentage":0},
           {"payment_type":"shopeepay","label":"ShopeePay","enabled":true,"customer_percentage":100,"fee_percentage":2,"fixed_fee":0,"tax_percentage":11},
           {"payment_type":"credit_card","label":"Kartu kredit","enabled":true,"customer_percentage":100,"fee_percentage":2.9,"fixed_fee":2000,"tax_percentage":11},
           {"payment_type":"indomaret","label":"Indomaret","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":1000,"tax_percentage":0},
           {"payment_type":"alfamart","label":"Alfamart / Alfamidi / DAN+DAN","enabled":true,"customer_percentage":100,"fee_percentage":0,"fixed_fee":5000,"tax_percentage":0},
           {"payment_type":"akulaku","label":"Akulaku PayLater","enabled":true,"customer_percentage":100,"fee_percentage":1.7,"fixed_fee":0,"tax_percentage":11}
         ]
       }'::jsonb,
       false
FROM branches b
WHERE b.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM settings s
      WHERE s.branch_id = b.id
        AND s.key = 'payment.midtrans.channels'
        AND s.deleted_at IS NULL
  );

DELETE FROM settings WHERE key = 'payment.midtrans.fee_bearer';

-- +goose Down
DELETE FROM settings WHERE key = 'payment.midtrans.channels';
DELETE FROM accounts a
WHERE code = '4201'
  AND NOT EXISTS (SELECT 1 FROM journal_lines jl WHERE jl.account_id = a.id);

ALTER TABLE payments
    DROP COLUMN payment_channel,
    DROP COLUMN provider_fee,
    DROP COLUMN customer_fee,
    DROP COLUMN base_amount;
