-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE branches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(30) NOT NULL UNIQUE,
    name varchar(150) NOT NULL,
    address text NOT NULL DEFAULT '',
    phone varchar(30) NOT NULL DEFAULT '',
    timezone varchar(60) NOT NULL DEFAULT 'Asia/Jakarta',
    currency char(3) NOT NULL DEFAULT 'IDR',
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_branches_deleted_at ON branches(deleted_at);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid REFERENCES branches(id),
    name varchar(150) NOT NULL,
    email varchar(255) NOT NULL,
    phone varchar(30) NOT NULL DEFAULT '',
    password_hash text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX users_email_unique ON users(lower(email)) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_branch_id ON users(branch_id);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(60) NOT NULL UNIQUE,
    name varchar(100) NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE TABLE permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(100) NOT NULL UNIQUE,
    name varchar(150) NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE TABLE role_permissions (
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(role_id, permission_id)
);
CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    branch_id uuid REFERENCES branches(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX user_roles_scope_unique ON user_roles(user_id, role_id, COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'));

CREATE TABLE refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash varchar(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    user_agent text NOT NULL DEFAULT '',
    ip_address inet,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

CREATE TABLE customers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id),
    code varchar(50) NOT NULL,
    name varchar(150) NOT NULL,
    phone varchar(30) NOT NULL DEFAULT '',
    email varchar(255) NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX customers_branch_code_unique ON customers(branch_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_customers_search ON customers(branch_id, lower(name), phone);

CREATE TABLE vehicles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id),
    customer_id uuid REFERENCES customers(id),
    identifier varchar(100) NOT NULL,
    plate_number varchar(30) NOT NULL DEFAULT '',
    brand varchar(80) NOT NULL DEFAULT '',
    model varchar(100) NOT NULL DEFAULT '',
    year integer NOT NULL DEFAULT 0 CHECK(year >= 0),
    color varchar(50) NOT NULL DEFAULT '',
    odometer bigint NOT NULL DEFAULT 0 CHECK(odometer >= 0),
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX vehicles_branch_identifier_unique ON vehicles(branch_id, lower(identifier)) WHERE deleted_at IS NULL;
CREATE INDEX idx_vehicles_customer_id ON vehicles(customer_id);

CREATE TABLE products (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid REFERENCES branches(id),
    sku varchar(80) NOT NULL,
    name varchar(180) NOT NULL,
    type varchar(20) NOT NULL CHECK(type IN ('part','service','other')),
    unit varchar(30) NOT NULL DEFAULT 'pcs',
    cost_price bigint NOT NULL DEFAULT 0 CHECK(cost_price >= 0),
    selling_price bigint NOT NULL DEFAULT 0 CHECK(selling_price >= 0),
    min_stock numeric(15,3) NOT NULL DEFAULT 0 CHECK(min_stock >= 0),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX products_scope_sku_unique ON products(COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'), sku) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_search ON products(lower(name), type);

CREATE TABLE inventory_balances (
    branch_id uuid NOT NULL REFERENCES branches(id),
    product_id uuid NOT NULL REFERENCES products(id),
    quantity numeric(15,3) NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(branch_id, product_id)
);
CREATE TABLE inventory_movements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id),
    product_id uuid NOT NULL REFERENCES products(id),
    reference_type varchar(30) NOT NULL,
    reference_id uuid,
    direction varchar(10) NOT NULL CHECK(direction IN ('in','out','adjustment')),
    quantity numeric(15,3) NOT NULL CHECK(quantity > 0),
    unit_cost bigint NOT NULL DEFAULT 0 CHECK(unit_cost >= 0),
    notes text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_inventory_movements_lookup ON inventory_movements(branch_id, product_id, created_at DESC);

CREATE TABLE work_orders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id),
    number varchar(50) NOT NULL,
    customer_id uuid NOT NULL REFERENCES customers(id),
    vehicle_id uuid NOT NULL REFERENCES vehicles(id),
    mechanic_id uuid REFERENCES users(id),
    status varchar(30) NOT NULL CHECK(status IN ('draft','inspection','approval','in_progress','completed','invoiced','cancelled')),
    complaint text NOT NULL DEFAULT '',
    diagnosis text NOT NULL DEFAULT '',
    odometer bigint NOT NULL DEFAULT 0 CHECK(odometer >= 0),
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX work_orders_branch_number_unique ON work_orders(branch_id, number) WHERE deleted_at IS NULL;
CREATE INDEX idx_work_orders_queue ON work_orders(branch_id, status, created_at DESC);

CREATE TABLE work_order_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    work_order_id uuid NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id),
    description varchar(200) NOT NULL,
    type varchar(20) NOT NULL CHECK(type IN ('part','service','other')),
    quantity numeric(15,3) NOT NULL CHECK(quantity > 0),
    unit_price bigint NOT NULL CHECK(unit_price >= 0),
    unit_cost bigint NOT NULL DEFAULT 0 CHECK(unit_cost >= 0),
    discount bigint NOT NULL DEFAULT 0 CHECK(discount >= 0),
    subtotal bigint NOT NULL CHECK(subtotal >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TABLE sales (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id),
    number varchar(50) NOT NULL,
    customer_id uuid REFERENCES customers(id),
    work_order_id uuid REFERENCES work_orders(id),
    cashier_id uuid NOT NULL REFERENCES users(id),
    status varchar(20) NOT NULL CHECK(status IN ('draft','pending','paid','void','refunded')),
    subtotal bigint NOT NULL DEFAULT 0,
    discount bigint NOT NULL DEFAULT 0,
    tax bigint NOT NULL DEFAULT 0,
    gateway_fee bigint NOT NULL DEFAULT 0,
    grand_total bigint NOT NULL DEFAULT 0,
    amount_paid bigint NOT NULL DEFAULT 0,
    change_amount bigint NOT NULL DEFAULT 0,
    notes text NOT NULL DEFAULT '',
    paid_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX sales_branch_number_unique ON sales(branch_id, number) WHERE deleted_at IS NULL;
CREATE INDEX idx_sales_report ON sales(branch_id, status, paid_at DESC);

CREATE TABLE sale_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id uuid NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
    product_id uuid NOT NULL REFERENCES products(id),
    description varchar(200) NOT NULL,
    type varchar(20) NOT NULL CHECK(type IN ('part','service','other')),
    quantity numeric(15,3) NOT NULL CHECK(quantity > 0),
    unit_price bigint NOT NULL CHECK(unit_price >= 0),
    unit_cost bigint NOT NULL DEFAULT 0 CHECK(unit_cost >= 0),
    discount bigint NOT NULL DEFAULT 0,
    subtotal bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE TABLE payments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id),
    sale_id uuid NOT NULL REFERENCES sales(id),
    method varchar(30) NOT NULL CHECK(method IN ('cash','midtrans')),
    provider varchar(30) NOT NULL DEFAULT '',
    provider_reference varchar(150) NOT NULL DEFAULT '',
    status varchar(30) NOT NULL CHECK(status IN ('pending','paid','failed','expired','refunded')),
    amount bigint NOT NULL CHECK(amount >= 0),
    fee bigint NOT NULL DEFAULT 0 CHECK(fee >= 0),
    fee_bearer varchar(20) NOT NULL CHECK(fee_bearer IN ('merchant','customer','split')),
    paid_at timestamptz,
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_payments_sale_id ON payments(sale_id);
CREATE UNIQUE INDEX payments_provider_reference_unique ON payments(provider, provider_reference) WHERE provider_reference <> '';

CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid REFERENCES branches(id),
    code varchar(30) NOT NULL,
    name varchar(150) NOT NULL,
    type varchar(30) NOT NULL CHECK(type IN ('asset','liability','equity','revenue','expense')),
    parent_id uuid REFERENCES accounts(id),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX accounts_scope_code_unique ON accounts(COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'), code) WHERE deleted_at IS NULL;
CREATE TABLE journal_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid NOT NULL REFERENCES branches(id),
    number varchar(50) NOT NULL,
    entry_date date NOT NULL,
    description text NOT NULL,
    reference_type varchar(30) NOT NULL DEFAULT '',
    reference_id uuid,
    status varchar(20) NOT NULL CHECK(status IN ('draft','posted','void')),
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX journal_entries_branch_number_unique ON journal_entries(branch_id, number) WHERE deleted_at IS NULL;
CREATE TABLE journal_lines (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_entry_id uuid NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES accounts(id),
    description text NOT NULL DEFAULT '',
    debit bigint NOT NULL DEFAULT 0 CHECK(debit >= 0),
    credit bigint NOT NULL DEFAULT 0 CHECK(credit >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CHECK((debit > 0 AND credit = 0) OR (credit > 0 AND debit = 0))
);
CREATE INDEX idx_journal_lines_account ON journal_lines(account_id, journal_entry_id);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid REFERENCES branches(id),
    user_id uuid REFERENCES users(id),
    action varchar(50) NOT NULL,
    resource varchar(80) NOT NULL,
    resource_id uuid,
    ip_address inet,
    user_agent text NOT NULL DEFAULT '',
    request_id varchar(100) NOT NULL DEFAULT '',
    before jsonb,
    after jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_lookup ON audit_logs(branch_id, resource, resource_id, created_at DESC);

CREATE TABLE cms_pages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug varchar(180) NOT NULL,
    title varchar(200) NOT NULL,
    meta_title varchar(200) NOT NULL DEFAULT '',
    meta_description varchar(320) NOT NULL DEFAULT '',
    content jsonb NOT NULL DEFAULT '{}',
    status varchar(20) NOT NULL CHECK(status IN ('draft','published','archived')),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX cms_pages_slug_unique ON cms_pages(slug) WHERE deleted_at IS NULL;
CREATE TABLE settings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    branch_id uuid REFERENCES branches(id),
    key varchar(150) NOT NULL,
    value jsonb NOT NULL DEFAULT '{}',
    is_public boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE UNIQUE INDEX settings_scope_key_unique ON settings(COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'), key) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS settings, cms_pages, audit_logs, journal_lines, journal_entries, accounts,
payments, sale_items, sales, work_order_items, work_orders, inventory_movements, inventory_balances,
products, vehicles, customers, refresh_tokens, user_roles, role_permissions, permissions, roles, users, branches CASCADE;
