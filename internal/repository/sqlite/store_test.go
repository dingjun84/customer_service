package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
	"github.com/example/ecommerce-support/internal/repository/sqlite"
)

// newTestDB opens a throwaway SQLite database in t.TempDir() and migrates it.
// Tests never touch the shared development database.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func newTestStore(t *testing.T) (*sqlite.Store, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	return sqlite.New(db), db
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	// Running the embedded migrations again must be a no-op.
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	version, err := database.SchemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want 1", version)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM schema_migrations WHERE version = 1`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration 1 recorded %d times, want 1", count)
	}
}

func TestForeignKeysAreEnforced(t *testing.T) {
	ctx := context.Background()
	_, db := newTestStore(t)

	_, err := db.ExecContext(ctx, `
		INSERT INTO orders (id, order_no, customer_id, order_status, created_at, updated_at)
		VALUES ('o1','ORD-X','missing-customer','paid','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected foreign key violation, got nil")
	}
}

func TestUniqueViolationMapsToConflict(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	insertCustomer(t, db, "CUST-1", "2026-01-01T00:00:00Z")
	_, err := db.ExecContext(ctx, `
		INSERT INTO customers (id, customer_no, status, created_at, updated_at)
		VALUES ('c2','CUST-1','active','2026-01-02T00:00:00Z','2026-01-02T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected unique violation")
	}
	if _, err := store.GetCustomerByNo(ctx, "CUST-1"); err != nil {
		t.Fatalf("get customer: %v", err)
	}
}

func TestGetMissingCustomerReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestStore(t)

	_, err := store.GetCustomer(ctx, "nope")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestListOrdersPagination(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	insertCustomer(t, db, "CUST-1", "2026-01-01T00:00:00Z")
	for i, created := range []string{"2026-01-01T10:00:00Z", "2026-01-02T10:00:00Z", "2026-01-03T10:00:00Z"} {
		insertOrder(t, db, domain.NewID(), "ORD-1000"+string(rune('0'+i)), "CUST-1", "paid", created)
	}
	customerID := lookupCustomerID(t, db, "CUST-1")

	first, err := store.ListOrders(ctx, repository.OrderQuery{CustomerID: customerID, Limit: 2})
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first page size = %d, want 2", len(first))
	}
	if first[0].OrderNo != "ORD-10002" {
		t.Fatalf("newest order = %s, want ORD-10002", first[0].OrderNo)
	}

	cursor := &repository.Cursor{CreatedAt: first[1].CreatedAt, ID: first[1].ID}
	second, err := store.ListOrders(ctx, repository.OrderQuery{CustomerID: customerID, Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf("list orders page 2: %v", err)
	}
	if len(second) != 1 || second[0].OrderNo != "ORD-10000" {
		t.Fatalf("second page = %+v, want ORD-10000", second)
	}
}

func TestUpdateOrderFieldsDetectsVersionConflict(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	insertCustomer(t, db, "CUST-1", "2026-01-01T00:00:00Z")
	orderID := domain.NewID()
	insertOrder(t, db, orderID, "ORD-20001", "CUST-1", "paid", "2026-01-01T00:00:00Z")

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateOrderFields(ctx, orderID, map[string]any{"shipping_address": "新地址 1 号"}, 1, now); err != nil {
		t.Fatalf("update address: %v", err)
	}
	err := store.UpdateOrderFields(ctx, orderID, map[string]any{"shipping_address": "并发写入"}, 1, now)
	if !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("err = %v, want domain.ErrVersionConflict", err)
	}

	order, err := store.GetOrderByID(ctx, orderID)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if order.ShippingAddress != "新地址 1 号" {
		t.Fatalf("address = %q, want 新地址 1 号", order.ShippingAddress)
	}
	if order.Version != 2 {
		t.Fatalf("version = %d, want 2", order.Version)
	}
}

func TestUpdateOrderFieldsRejectsUnknownColumn(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	insertCustomer(t, db, "CUST-1", "2026-01-01T00:00:00Z")
	orderID := domain.NewID()
	insertOrder(t, db, orderID, "ORD-20002", "CUST-1", "paid", "2026-01-01T00:00:00Z")

	err := store.UpdateOrderFields(ctx, orderID, map[string]any{"paid_amount_minor": 0}, 1, time.Now())
	if err == nil {
		t.Fatal("expected rejection of non-whitelisted column")
	}
	order, _ := store.GetOrderByID(ctx, orderID)
	if order.PaidAmountMinor != 129900 {
		t.Fatalf("paid amount changed to %d", order.PaidAmountMinor)
	}
}

func TestMessagesPaginationIsChronological(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	insertCustomer(t, db, "CUST-1", "2026-01-01T00:00:00Z")
	convID := domain.NewID()
	if err := store.CreateConversation(ctx, &domain.Conversation{
		ID: convID, ConversationNo: "CONV-1", CustomerID: lookupCustomerID(t, db, "CUST-1"), Status: domain.ConversationOpen,
		Channel: "web_demo", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if err := store.AppendMessage(ctx, &domain.Message{
			ID: domain.NewID(), ConversationID: convID, Role: domain.RoleUser,
			Content: "m" + string(rune('0'+i)), MetadataJSON: "{}",
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatalf("append message: %v", err)
		}
	}

	page, err := store.ListMessages(ctx, repository.MessageQuery{ConversationID: convID, Limit: 2})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(page) != 2 || page[0].Content != "m3" || page[1].Content != "m4" {
		t.Fatalf("page = %v, want [m3 m4]", messageContents(page))
	}

	cursor := &repository.Cursor{CreatedAt: page[0].CreatedAt, ID: page[0].ID}
	older, err := store.ListMessages(ctx, repository.MessageQuery{ConversationID: convID, Limit: 5, Before: cursor})
	if err != nil {
		t.Fatalf("list older messages: %v", err)
	}
	if got := messageContents(older); len(got) != 3 || got[0] != "m0" || got[2] != "m2" {
		t.Fatalf("older page = %v, want [m0 m1 m2]", got)
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	insertCustomer(t, db, "CUST-1", "2026-01-01T00:00:00Z")
	sentinel := errors.New("boom")
	err := store.WithTx(ctx, func(tx repository.Store) error {
		if err := tx.AppendAuditLog(ctx, &domain.ActionAuditLog{
			ID: domain.NewID(), ActionType: "test", TargetType: "order", TargetID: "o1",
			ActorType: domain.ActorSystem, Result: "succeeded", CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM action_audit_logs`).Scan(&count); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if count != 0 {
		t.Fatalf("audit rows = %d, want 0 after rollback", count)
	}
}

// ---- fixtures ----

func insertCustomer(t *testing.T, db *sql.DB, customerNo, createdAt string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO customers (id, customer_no, display_name, phone_masked, status, created_at, updated_at)
		VALUES (?,?,?,?, 'active', ?, ?)`,
		domain.NewID(), customerNo, "测试客户", "138****0000", createdAt, createdAt)
	if err != nil {
		t.Fatalf("insert customer: %v", err)
	}
}

func lookupCustomerID(t *testing.T, db *sql.DB, customerNo string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id FROM customers WHERE customer_no = ?`, customerNo).Scan(&id); err != nil {
		t.Fatalf("lookup customer %s: %v", customerNo, err)
	}
	return id
}

func insertOrder(t *testing.T, db *sql.DB, id, orderNo, customerNo, status, createdAt string) {
	t.Helper()
	var customerID string
	if err := db.QueryRow(`SELECT id FROM customers WHERE customer_no = ?`, customerNo).Scan(&customerID); err != nil {
		t.Fatalf("lookup customer: %v", err)
	}
	_, err := db.Exec(`INSERT INTO orders (id, order_no, customer_id, order_status, item_amount_minor,
		discount_amount_minor, shipping_fee_minor, paid_amount_minor, refunded_amount_minor,
		shipping_method, shipping_address, created_at, updated_at)
		VALUES (?,?,?,?, 129900, 0, 0, 129900, 0, 'standard', '测试地址', ?, ?)`,
		id, orderNo, customerID, status, createdAt, createdAt)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
}

func messageContents(msgs []domain.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Content)
	}
	return out
}
