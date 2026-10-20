package service_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository/sqlite"
	"github.com/example/ecommerce-support/internal/service"
)

// fixedNow keeps window/expiry assertions deterministic.
var fixedNow = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

type fixture struct {
	t      *testing.T
	db     *sql.DB
	store  *sqlite.Store
	svc    *service.Services
	custID string
	convID string
	ctx    context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "svc.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := sqlite.New(db)
	svc := service.New(store, service.Options{
		PendingActionTTL:      10 * time.Minute,
		AfterSalesWindow:      7 * 24 * time.Hour,
		PriceProtectionWindow: 15 * 24 * time.Hour,
		Now:                   func() time.Time { return fixedNow },
	})

	f := &fixture{t: t, db: db, store: store, svc: svc, ctx: ctx}
	f.custID = f.insertCustomer("CUST-1001")
	f.convID = f.insertConversation(f.custID)
	return f
}

func (f *fixture) insertCustomer(customerNo string) string {
	f.t.Helper()
	id := domain.NewID()
	now := domain.FormatTime(fixedNow.Add(-30 * 24 * time.Hour))
	_, err := f.db.Exec(`INSERT INTO customers (id, customer_no, display_name, phone_masked, status, created_at, updated_at)
		VALUES (?,?,?,?,'active',?,?)`, id, customerNo, "测试客户", "138****0000", now, now)
	if err != nil {
		f.t.Fatalf("insert customer: %v", err)
	}
	return id
}

func (f *fixture) insertConversation(customerID string) string {
	f.t.Helper()
	id := domain.NewID()
	if err := f.store.CreateConversation(f.ctx, &domain.Conversation{
		ID: id, ConversationNo: domain.HumanNo("CONV", fixedNow), CustomerID: customerID,
		Status: domain.ConversationOpen, Channel: "web_demo",
		CreatedAt: fixedNow, UpdatedAt: fixedNow,
	}); err != nil {
		f.t.Fatalf("insert conversation: %v", err)
	}
	return id
}

type orderOpts struct {
	status      string
	paidMinor   int64
	paidAt      *time.Time
	shippedAt   *time.Time
	deliveredAt *time.Time
	quantity    int
	unitPrice   int64
	address     string
}

// insertOrderWithItem creates an order plus one item and returns both IDs.
func (f *fixture) insertOrderWithItem(orderNo string, opts orderOpts) (orderID, itemID string) {
	f.t.Helper()
	if opts.status == "" {
		opts.status = domain.OrderPaid
	}
	if opts.quantity == 0 {
		opts.quantity = 1
	}
	if opts.unitPrice == 0 {
		opts.unitPrice = 129900
	}
	if opts.address == "" {
		opts.address = "深圳市南山区科技园 1 号"
	}
	orderID = domain.NewID()
	created := fixedNow.Add(-10 * 24 * time.Hour)
	_, err := f.db.Exec(`INSERT INTO orders (
		id, order_no, customer_id, order_status, currency, item_amount_minor, paid_amount_minor, refunded_amount_minor,
		receiver_name, receiver_phone_masked, shipping_province, shipping_city, shipping_district, shipping_address,
		shipping_method, created_at, paid_at, shipped_at, delivered_at, updated_at, version)
		VALUES (?,?,?,?,'CNY',?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)`,
		orderID, orderNo, f.custID, opts.status, opts.unitPrice*int64(opts.quantity), opts.paidMinor,
		0, "张三", "138****0000", "广东省", "深圳市", "南山区", opts.address,
		"standard", domain.FormatTime(created), timePtrString(opts.paidAt), timePtrString(opts.shippedAt),
		timePtrString(opts.deliveredAt), domain.FormatTime(created))
	if err != nil {
		f.t.Fatalf("insert order: %v", err)
	}

	itemID = domain.NewID()
	_, err = f.db.Exec(`INSERT INTO order_items (id, order_id, product_name_snapshot, sku_name_snapshot, sku_attributes_json,
		quantity, refunded_quantity, unit_price_minor, discount_amount_minor, paid_amount_minor, item_status, created_at, updated_at)
		VALUES (?,?,?,?,'{}',?,0,?,0,?, 'active', ?, ?)`,
		itemID, orderID, "示例咖啡机", "标准版", opts.quantity, opts.unitPrice, opts.unitPrice*int64(opts.quantity),
		domain.FormatTime(created), domain.FormatTime(created))
	if err != nil {
		f.t.Fatalf("insert order item: %v", err)
	}
	return orderID, itemID
}

func (f *fixture) insertShipment(orderID, status, trackingNo string, pickedUpAt time.Time) string {
	f.t.Helper()
	id := domain.NewID()
	_, err := f.db.Exec(`INSERT INTO shipments (id, shipment_no, order_id, carrier_code, carrier_name, tracking_no,
		shipment_status, last_event_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, "SHP-"+trackingNo, orderID, "SF", "顺丰速运", trackingNo, status,
		domain.FormatTime(pickedUpAt), domain.FormatTime(pickedUpAt), domain.FormatTime(pickedUpAt))
	if err != nil {
		f.t.Fatalf("insert shipment: %v", err)
	}
	_, err = f.db.Exec(`INSERT INTO shipment_events (id, shipment_id, event_time, event_code, description, location, source, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, domain.NewID(), id, domain.FormatTime(pickedUpAt), "picked_up",
		"快件已揽收", "深圳", "mock_logistics", domain.FormatTime(pickedUpAt))
	if err != nil {
		f.t.Fatalf("insert shipment event: %v", err)
	}
	return id
}

func (f *fixture) insertPayment(orderID, paymentNo string, amount int64) string {
	f.t.Helper()
	id := domain.NewID()
	_, err := f.db.Exec(`INSERT INTO payments (id, payment_no, order_id, payment_method, payment_status,
		amount_minor, refunded_amount_minor, paid_at, created_at, updated_at)
		VALUES (?,?,?,'wechat_pay','succeeded',?,0,?,?,?)`,
		id, paymentNo, orderID, amount, domain.FormatTime(fixedNow.Add(-10*24*time.Hour)),
		domain.FormatTime(fixedNow.Add(-10*24*time.Hour)), domain.FormatTime(fixedNow.Add(-10*24*time.Hour)))
	if err != nil {
		f.t.Fatalf("insert payment: %v", err)
	}
	return id
}

func (f *fixture) orderStatus(orderID string) string {
	f.t.Helper()
	var status string
	if err := f.db.QueryRow(`SELECT order_status FROM orders WHERE id = ?`, orderID).Scan(&status); err != nil {
		f.t.Fatalf("read order status: %v", err)
	}
	return status
}

func (f *fixture) auditCount(targetID string) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(1) FROM action_audit_logs WHERE target_id = ?`, targetID).Scan(&n); err != nil {
		f.t.Fatalf("count audit: %v", err)
	}
	return n
}

func timePtr(t time.Time) *time.Time { return &t }

func timePtrString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return domain.FormatTime(*t)
}
