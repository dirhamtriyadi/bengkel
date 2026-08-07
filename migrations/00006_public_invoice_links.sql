-- +goose Up
CREATE TABLE public_invoice_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    sale_id uuid NOT NULL REFERENCES sales(id) ON DELETE RESTRICT,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    token_hash char(64) NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    source varchar(30) NOT NULL CHECK (source IN ('midtrans_snap', 'whatsapp')),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    delivered_at timestamptz,
    recipient_phone varchar(20) NOT NULL DEFAULT '',
    provider_message_id varchar(255) NOT NULL DEFAULT '',
    delivery_error varchar(500) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_public_invoice_links_sale
    ON public_invoice_links(branch_id, sale_id, created_at DESC);
CREATE INDEX idx_public_invoice_links_active_expiry
    ON public_invoice_links(expires_at)
    WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS public_invoice_links;
