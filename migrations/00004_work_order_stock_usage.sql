-- +goose Up
ALTER TABLE work_order_items
    ADD COLUMN stock_deducted boolean NOT NULL DEFAULT false;

CREATE INDEX idx_work_order_items_active
    ON work_order_items(work_order_id, created_at)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_work_order_items_active;
ALTER TABLE work_order_items DROP COLUMN IF EXISTS stock_deducted;
