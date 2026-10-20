-- Ecommerce AI customer-support MVP schema for SQLite.
-- Schema only: intentionally contains no seed/test data.
-- Target: Go + database/sql + modernc.org/sqlite.
-- IDs are application-generated UUID strings; money is stored in minor units (CNY fen).
-- All timestamps are UTC RFC3339 strings. Keep SQLite foreign_keys enabled on every connection.


CREATE TABLE IF NOT EXISTS schema_migrations (
    version       INTEGER PRIMARY KEY,
    applied_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS customers (
    id             TEXT PRIMARY KEY,
    customer_no    TEXT NOT NULL UNIQUE,
    display_name   TEXT,
    phone_masked   TEXT,
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','deleted')),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_customers_status ON customers(status);

CREATE TABLE IF NOT EXISTS products (
    id             TEXT PRIMARY KEY,
    product_code   TEXT NOT NULL UNIQUE,
    name           TEXT NOT NULL,
    brand          TEXT,
    category       TEXT,
    description    TEXT,
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','discontinued')),
    attributes_json TEXT NOT NULL DEFAULT '{}',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_products_name ON products(name);
CREATE INDEX IF NOT EXISTS idx_products_category_status ON products(category, status);

CREATE TABLE IF NOT EXISTS product_skus (
    id                 TEXT PRIMARY KEY,
    product_id         TEXT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    sku_code           TEXT NOT NULL UNIQUE,
    sku_name           TEXT NOT NULL,
    specifications_json TEXT NOT NULL DEFAULT '{}',
    list_price_minor   INTEGER NOT NULL CHECK (list_price_minor >= 0),
    stock_quantity     INTEGER NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
    status             TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','out_of_stock')),
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_product_skus_product ON product_skus(product_id);
CREATE INDEX IF NOT EXISTS idx_product_skus_status_stock ON product_skus(status, stock_quantity);

CREATE TABLE IF NOT EXISTS orders (
    id                    TEXT PRIMARY KEY,
    order_no              TEXT NOT NULL UNIQUE,
    customer_id           TEXT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    order_status          TEXT NOT NULL CHECK (order_status IN ('pending_payment','paid','processing','shipped','completed','cancelled','closed')),
    order_channel         TEXT NOT NULL DEFAULT 'mock_store',
    currency              TEXT NOT NULL DEFAULT 'CNY' CHECK (length(currency) = 3),
    item_amount_minor     INTEGER NOT NULL DEFAULT 0 CHECK (item_amount_minor >= 0),
    discount_amount_minor INTEGER NOT NULL DEFAULT 0 CHECK (discount_amount_minor >= 0),
    shipping_fee_minor    INTEGER NOT NULL DEFAULT 0 CHECK (shipping_fee_minor >= 0),
    paid_amount_minor     INTEGER NOT NULL DEFAULT 0 CHECK (paid_amount_minor >= 0),
    refunded_amount_minor INTEGER NOT NULL DEFAULT 0 CHECK (refunded_amount_minor >= 0),
    is_presale            INTEGER NOT NULL DEFAULT 0 CHECK (is_presale IN (0,1)),
    receiver_name         TEXT,
    receiver_phone_masked TEXT,
    shipping_province     TEXT,
    shipping_city         TEXT,
    shipping_district     TEXT,
    shipping_address      TEXT,
    shipping_postal_code  TEXT,
    shipping_method       TEXT NOT NULL DEFAULT 'standard',
    requested_delivery_at TEXT,
    invoice_type_requested TEXT CHECK (invoice_type_requested IS NULL OR invoice_type_requested IN ('none','electronic','paper','vat_special')),
    order_remark          TEXT,
    created_at            TEXT NOT NULL,
    paid_at               TEXT,
    shipped_at            TEXT,
    delivered_at          TEXT,
    completed_at          TEXT,
    cancelled_at          TEXT,
    updated_at            TEXT NOT NULL,
    version               INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1)
);
CREATE INDEX IF NOT EXISTS idx_orders_customer_created ON orders(customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orders_customer_status ON orders(customer_id, order_status);
CREATE INDEX IF NOT EXISTS idx_orders_status_created ON orders(order_status, created_at DESC);

CREATE TABLE IF NOT EXISTS order_items (
    id                      TEXT PRIMARY KEY,
    order_id                TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    product_id              TEXT REFERENCES products(id) ON DELETE SET NULL,
    sku_id                  TEXT REFERENCES product_skus(id) ON DELETE SET NULL,
    product_code_snapshot   TEXT,
    product_name_snapshot   TEXT NOT NULL,
    sku_code_snapshot       TEXT,
    sku_name_snapshot       TEXT,
    sku_attributes_json     TEXT NOT NULL DEFAULT '{}',
    quantity                INTEGER NOT NULL CHECK (quantity > 0),
    refunded_quantity       INTEGER NOT NULL DEFAULT 0 CHECK (refunded_quantity >= 0 AND refunded_quantity <= quantity),
    unit_price_minor        INTEGER NOT NULL CHECK (unit_price_minor >= 0),
    discount_amount_minor   INTEGER NOT NULL DEFAULT 0 CHECK (discount_amount_minor >= 0),
    paid_amount_minor       INTEGER NOT NULL CHECK (paid_amount_minor >= 0),
    item_status             TEXT NOT NULL DEFAULT 'active' CHECK (item_status IN ('active','cancelled','completed')),
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_order_items_product ON order_items(product_id);

CREATE TABLE IF NOT EXISTS shipments (
    id                     TEXT PRIMARY KEY,
    shipment_no            TEXT NOT NULL UNIQUE,
    order_id               TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    carrier_code           TEXT,
    carrier_name           TEXT,
    tracking_no            TEXT UNIQUE,
    shipment_status        TEXT NOT NULL CHECK (shipment_status IN ('pending','picked_up','in_transit','delivering','delivered','exception','returned','intercepted','delivery_failed')),
    estimated_delivery_at  TEXT,
    last_event_at          TEXT,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shipments_order ON shipments(order_id);
CREATE INDEX IF NOT EXISTS idx_shipments_tracking ON shipments(tracking_no);
CREATE INDEX IF NOT EXISTS idx_shipments_status ON shipments(shipment_status, last_event_at);

CREATE TABLE IF NOT EXISTS shipment_events (
    id             TEXT PRIMARY KEY,
    shipment_id    TEXT NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    event_time     TEXT NOT NULL,
    event_code     TEXT,
    description    TEXT NOT NULL,
    location       TEXT,
    source         TEXT NOT NULL DEFAULT 'mock_logistics',
    created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_shipment_events_shipment_time ON shipment_events(shipment_id, event_time DESC);

CREATE TABLE IF NOT EXISTS service_cases (
    id                 TEXT PRIMARY KEY,
    case_no            TEXT NOT NULL UNIQUE,
    customer_id        TEXT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    order_id           TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    order_item_id      TEXT REFERENCES order_items(id) ON DELETE RESTRICT,
    case_type          TEXT NOT NULL CHECK (case_type IN ('return','exchange','repair','refund','reship','price_protection')),
    case_status        TEXT NOT NULL CHECK (case_status IN ('submitted','pending_review','approved','rejected','awaiting_customer','awaiting_return_shipment','processing','completed','cancelled')),
    reason_code        TEXT,
    reason_text        TEXT,
    description        TEXT,
    requested_quantity INTEGER NOT NULL DEFAULT 1 CHECK (requested_quantity > 0),
    resolution         TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    closed_at          TEXT
);
CREATE INDEX IF NOT EXISTS idx_service_cases_customer_created ON service_cases(customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_service_cases_order ON service_cases(order_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_service_cases_status ON service_cases(case_status, updated_at DESC);

CREATE TABLE IF NOT EXISTS payments (
    id                       TEXT PRIMARY KEY,
    payment_no               TEXT NOT NULL UNIQUE,
    order_id                 TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    payment_method           TEXT NOT NULL CHECK (payment_method IN ('wechat_pay','alipay','bank_card','cash_on_delivery','whitebar','account_balance','mixed','other')),
    payment_status           TEXT NOT NULL CHECK (payment_status IN ('pending','succeeded','failed','closed','partially_refunded','refunded')),
    amount_minor             INTEGER NOT NULL CHECK (amount_minor >= 0),
    refunded_amount_minor    INTEGER NOT NULL DEFAULT 0 CHECK (refunded_amount_minor >= 0),
    provider_transaction_ref TEXT,
    paid_at                  TEXT,
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_payments_order ON payments(order_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(payment_status);

CREATE TABLE IF NOT EXISTS refunds (
    id                 TEXT PRIMARY KEY,
    refund_no          TEXT NOT NULL UNIQUE,
    order_id           TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    payment_id         TEXT REFERENCES payments(id) ON DELETE RESTRICT,
    service_case_id    TEXT REFERENCES service_cases(id) ON DELETE RESTRICT,
    refund_status      TEXT NOT NULL CHECK (refund_status IN ('requested','pending_review','approved','processing','succeeded','failed','rejected','cancelled')),
    refund_channel     TEXT,
    amount_minor       INTEGER NOT NULL CHECK (amount_minor >= 0),
    reason             TEXT,
    requested_at       TEXT NOT NULL,
    completed_at       TEXT,
    failure_reason     TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_refunds_order ON refunds(order_id, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_refunds_status ON refunds(refund_status, updated_at DESC);

CREATE TABLE IF NOT EXISTS invoices (
    id              TEXT PRIMARY KEY,
    invoice_no      TEXT UNIQUE,
    customer_id     TEXT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    order_id        TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    invoice_type    TEXT NOT NULL CHECK (invoice_type IN ('electronic','paper','vat_special')),
    invoice_status  TEXT NOT NULL CHECK (invoice_status IN ('requested','processing','issued','rejected','voided','red_flushed')),
    title           TEXT NOT NULL,
    tax_no          TEXT,
    amount_minor    INTEGER NOT NULL CHECK (amount_minor >= 0),
    email           TEXT,
    file_url        TEXT,
    requested_at    TEXT NOT NULL,
    issued_at       TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_invoices_order ON invoices(order_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_invoices_customer ON invoices(customer_id, created_at DESC);

CREATE TABLE IF NOT EXISTS promotions (
    id               TEXT PRIMARY KEY,
    promotion_code   TEXT NOT NULL UNIQUE,
    title            TEXT NOT NULL,
    description      TEXT,
    promotion_type   TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('draft','active','expired','disabled')),
    starts_at        TEXT,
    ends_at          TEXT,
    rules_json       TEXT NOT NULL DEFAULT '{}',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_promotions_status_dates ON promotions(status, starts_at, ends_at);

CREATE TABLE IF NOT EXISTS customer_benefits (
    id                TEXT PRIMARY KEY,
    customer_id       TEXT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    benefit_type      TEXT NOT NULL CHECK (benefit_type IN ('coupon','points','freight_insurance','whitebar_status','store_balance','other')),
    benefit_code      TEXT,
    status            TEXT NOT NULL CHECK (status IN ('available','used','expired','processing','disabled')),
    amount_minor      INTEGER CHECK (amount_minor IS NULL OR amount_minor >= 0),
    points            INTEGER CHECK (points IS NULL OR points >= 0),
    expires_at        TEXT,
    attributes_json   TEXT NOT NULL DEFAULT '{}',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_customer_benefits_customer_type ON customer_benefits(customer_id, benefit_type, status);

CREATE TABLE IF NOT EXISTS knowledge_articles (
    id             TEXT PRIMARY KEY,
    article_code   TEXT NOT NULL UNIQUE,
    category       TEXT NOT NULL,
    title          TEXT NOT NULL,
    content        TEXT NOT NULL,
    keywords_json  TEXT NOT NULL DEFAULT '[]',
    status         TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('draft','published','archived')),
    source         TEXT,
    version        INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_knowledge_category_status ON knowledge_articles(category, status);
CREATE INDEX IF NOT EXISTS idx_knowledge_title ON knowledge_articles(title);

CREATE TABLE IF NOT EXISTS conversations (
    id                       TEXT PRIMARY KEY,
    conversation_no          TEXT NOT NULL UNIQUE,
    customer_id              TEXT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    status                   TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed','transferred')),
    channel                  TEXT NOT NULL DEFAULT 'web_demo',
    active_task_state_json   TEXT,
    summary                  TEXT,
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL,
    last_message_at          TEXT
);
CREATE INDEX IF NOT EXISTS idx_conversations_customer_last_message ON conversations(customer_id, last_message_at DESC);
CREATE INDEX IF NOT EXISTS idx_conversations_status ON conversations(status, updated_at DESC);

CREATE TABLE IF NOT EXISTS messages (
    id                   TEXT PRIMARY KEY,
    conversation_id      TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role                 TEXT NOT NULL CHECK (role IN ('system','user','assistant','tool')),
    content              TEXT NOT NULL DEFAULT '',
    tool_name            TEXT,
    tool_call_id         TEXT,
    tool_arguments_json  TEXT,
    tool_result_json     TEXT,
    metadata_json        TEXT NOT NULL DEFAULT '{}',
    created_at           TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_time ON messages(conversation_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_messages_tool_call ON messages(tool_call_id);

CREATE TABLE IF NOT EXISTS message_annotations (
    id                    TEXT PRIMARY KEY,
    message_id            TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    intent_primary        TEXT,
    intent_secondary      TEXT,
    sentiment             TEXT,
    resolution_status     TEXT CHECK (resolution_status IS NULL OR resolution_status IN ('resolved','unresolved','transferred','unknown')),
    confidence            REAL CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    annotation_source     TEXT NOT NULL DEFAULT 'offline_eval',
    created_at            TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_annotations_intent ON message_annotations(intent_primary, intent_secondary);

CREATE TABLE IF NOT EXISTS tool_executions (
    id                 TEXT PRIMARY KEY,
    conversation_id    TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    message_id         TEXT REFERENCES messages(id) ON DELETE SET NULL,
    tool_call_id       TEXT NOT NULL UNIQUE,
    tool_name          TEXT NOT NULL,
    input_json         TEXT NOT NULL DEFAULT '{}',
    output_json        TEXT,
    status             TEXT NOT NULL CHECK (status IN ('started','succeeded','failed','rejected')),
    error_code         TEXT,
    error_message_safe TEXT,
    latency_ms         INTEGER CHECK (latency_ms IS NULL OR latency_ms >= 0),
    created_at         TEXT NOT NULL,
    finished_at        TEXT
);
CREATE INDEX IF NOT EXISTS idx_tool_executions_conversation_time ON tool_executions(conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tool_executions_name_status ON tool_executions(tool_name, status, created_at DESC);

CREATE TABLE IF NOT EXISTS pending_actions (
    id                    TEXT PRIMARY KEY,
    action_no             TEXT NOT NULL UNIQUE,
    conversation_id       TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    customer_id           TEXT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    action_type           TEXT NOT NULL CHECK (action_type IN ('cancel_order','modify_order','submit_service_request')),
    target_type           TEXT NOT NULL,
    target_id             TEXT NOT NULL,
    payload_json          TEXT NOT NULL,
    preview_json          TEXT NOT NULL DEFAULT '{}',
    status                TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','rejected','expired','executed','failed')),
    expires_at            TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    confirmed_at          TEXT,
    executed_at           TEXT,
    result_json           TEXT
);
CREATE INDEX IF NOT EXISTS idx_pending_actions_customer_status ON pending_actions(customer_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pending_actions_conversation ON pending_actions(conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS action_audit_logs (
    id               TEXT PRIMARY KEY,
    request_id       TEXT,
    customer_id      TEXT REFERENCES customers(id) ON DELETE SET NULL,
    conversation_id  TEXT REFERENCES conversations(id) ON DELETE SET NULL,
    action_type      TEXT NOT NULL,
    target_type      TEXT NOT NULL,
    target_id        TEXT NOT NULL,
    before_json      TEXT,
    after_json       TEXT,
    actor_type       TEXT NOT NULL CHECK (actor_type IN ('assistant','customer','human_agent','system','admin')),
    actor_id         TEXT,
    result           TEXT NOT NULL CHECK (result IN ('succeeded','failed','rejected')),
    reason           TEXT,
    created_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_action_audit_target ON action_audit_logs(target_type, target_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_audit_conversation ON action_audit_logs(conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_audit_request ON action_audit_logs(request_id);

CREATE TABLE IF NOT EXISTS service_tickets (
    id              TEXT PRIMARY KEY,
    ticket_no       TEXT NOT NULL UNIQUE,
    customer_id     TEXT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    conversation_id TEXT REFERENCES conversations(id) ON DELETE SET NULL,
    order_id        TEXT REFERENCES orders(id) ON DELETE SET NULL,
    ticket_type     TEXT NOT NULL CHECK (ticket_type IN ('human_transfer','complaint','quality_issue','other')),
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','assigned','pending_customer','resolved','closed')),
    priority        TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','urgent')),
    category        TEXT,
    summary         TEXT NOT NULL,
    details         TEXT,
    assigned_to     TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    closed_at       TEXT
);
CREATE INDEX IF NOT EXISTS idx_service_tickets_customer_created ON service_tickets(customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_service_tickets_status_priority ON service_tickets(status, priority, created_at DESC);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    idempotency_key   TEXT PRIMARY KEY,
    operation         TEXT NOT NULL,
    request_hash      TEXT NOT NULL,
    response_json     TEXT,
    created_at        TEXT NOT NULL,
    expires_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_idempotency_expiry ON idempotency_keys(expires_at);
