package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
)

const orderColumns = `id, order_no, customer_id, order_status, order_channel, currency,
	item_amount_minor, discount_amount_minor, shipping_fee_minor, paid_amount_minor, refunded_amount_minor,
	is_presale,
	COALESCE(receiver_name,''), COALESCE(receiver_phone_masked,''),
	COALESCE(shipping_province,''), COALESCE(shipping_city,''), COALESCE(shipping_district,''),
	COALESCE(shipping_address,''), COALESCE(shipping_postal_code,''), shipping_method,
	COALESCE(requested_delivery_at,''), COALESCE(invoice_type_requested,''), COALESCE(order_remark,''),
	created_at, COALESCE(paid_at,''), COALESCE(shipped_at,''), COALESCE(delivered_at,''),
	COALESCE(completed_at,''), COALESCE(cancelled_at,''), updated_at, version`

func scanOrder(scan func(dest ...any) error) (*domain.Order, error) {
	var o domain.Order
	var isPresale int
	var createdAt, paidAt, shippedAt, deliveredAt, completedAt, cancelledAt, updatedAt string
	if err := scan(&o.ID, &o.OrderNo, &o.CustomerID, &o.OrderStatus, &o.OrderChannel, &o.Currency,
		&o.ItemAmountMinor, &o.DiscountAmountMinor, &o.ShippingFeeMinor, &o.PaidAmountMinor, &o.RefundedAmountMinor,
		&isPresale,
		&o.ReceiverName, &o.ReceiverPhoneMasked,
		&o.ShippingProvince, &o.ShippingCity, &o.ShippingDistrict,
		&o.ShippingAddress, &o.ShippingPostalCode, &o.ShippingMethod,
		&o.RequestedDeliveryAt, &o.InvoiceTypeRequested, &o.OrderRemark,
		&createdAt, &paidAt, &shippedAt, &deliveredAt, &completedAt, &cancelledAt, &updatedAt, &o.Version); err != nil {
		return nil, translateErr(err)
	}
	o.IsPresale = isPresale == 1
	o.CreatedAt = parseTS(createdAt)
	o.PaidAt = parseTSPtr(paidAt)
	o.ShippedAt = parseTSPtr(shippedAt)
	o.DeliveredAt = parseTSPtr(deliveredAt)
	o.CompletedAt = parseTSPtr(completedAt)
	o.CancelledAt = parseTSPtr(cancelledAt)
	o.UpdatedAt = parseTS(updatedAt)
	return &o, nil
}

// ListOrders returns the given customer's orders, newest first, using keyset pagination.
func (s *Store) ListOrders(ctx context.Context, q repository.OrderQuery) ([]domain.Order, error) {
	limit := limitOrDefault(q.Limit, 20, 100)
	where := []string{"customer_id = ?"}
	args := []any{q.CustomerID}
	if status := strings.TrimSpace(q.Status); status != "" {
		where = append(where, "order_status = ?")
		args = append(args, status)
	}
	if q.Cursor != nil {
		where = append(where, `(created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, ts(q.Cursor.CreatedAt), ts(q.Cursor.CreatedAt), q.Cursor.ID)
	}
	args = append(args, limit)

	sqlText := `SELECT ` + orderColumns + ` FROM orders WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY created_at DESC, id DESC LIMIT ?`
	rows, err := s.conn().QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := make([]domain.Order, 0, limit)
	for rows.Next() {
		o, err := scanOrder(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, translateErr(rows.Err())
}

// GetOrderByNo loads an order by business number. Ownership is enforced by callers.
func (s *Store) GetOrderByNo(ctx context.Context, orderNo string) (*domain.Order, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+orderColumns+` FROM orders WHERE order_no = ?`, orderNo)
	return scanOrder(row.Scan)
}

// GetOrderByID loads an order by internal ID.
func (s *Store) GetOrderByID(ctx context.Context, id string) (*domain.Order, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+orderColumns+` FROM orders WHERE id = ?`, id)
	return scanOrder(row.Scan)
}

const orderItemColumns = `id, order_id, COALESCE(product_id,''), COALESCE(sku_id,''),
	COALESCE(product_code_snapshot,''), product_name_snapshot, COALESCE(sku_code_snapshot,''), COALESCE(sku_name_snapshot,''),
	sku_attributes_json, quantity, refunded_quantity, unit_price_minor, discount_amount_minor, paid_amount_minor,
	item_status, created_at, updated_at`

func scanOrderItem(scan func(dest ...any) error) (*domain.OrderItem, error) {
	var it domain.OrderItem
	var created, updated string
	if err := scan(&it.ID, &it.OrderID, &it.ProductID, &it.SKUID,
		&it.ProductCodeSnapshot, &it.ProductNameSnapshot, &it.SKUCodeSnapshot, &it.SKUNameSnapshot,
		&it.SKUAttributesJSON, &it.Quantity, &it.RefundedQuantity, &it.UnitPriceMinor, &it.DiscountAmountMinor, &it.PaidAmountMinor,
		&it.ItemStatus, &created, &updated); err != nil {
		return nil, translateErr(err)
	}
	it.CreatedAt = parseTS(created)
	it.UpdatedAt = parseTS(updated)
	it.SKUAttributes = jsonObject(it.SKUAttributesJSON)
	return &it, nil
}

// ListOrderItems returns the item snapshots of an order.
func (s *Store) ListOrderItems(ctx context.Context, orderID string) ([]domain.OrderItem, error) {
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+orderItemColumns+` FROM order_items WHERE order_id = ? ORDER BY created_at, id`, orderID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.OrderItem{}
	for rows.Next() {
		it, err := scanOrderItem(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, translateErr(rows.Err())
}

// GetOrderItem loads a single order item.
func (s *Store) GetOrderItem(ctx context.Context, itemID string) (*domain.OrderItem, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+orderItemColumns+` FROM order_items WHERE id = ?`, itemID)
	return scanOrderItem(row.Scan)
}

// orderUpdateSpecs maps the API-level field name to its physical column.
// Only these columns may ever be written by an action confirmation; the service
// layer validates the same whitelist before calling this method.
var orderUpdateSpecs = []struct{ key, column string }{
	{"receiver_name", "receiver_name"},
	{"receiver_phone", "receiver_phone_masked"},
	{"shipping_province", "shipping_province"},
	{"shipping_city", "shipping_city"},
	{"shipping_district", "shipping_district"},
	{"shipping_address", "shipping_address"},
	{"shipping_postal_code", "shipping_postal_code"},
	{"shipping_method", "shipping_method"},
	{"requested_delivery_at", "requested_delivery_at"},
	{"order_remark", "order_remark"},
}

var orderUpdateColumns = func() map[string]bool {
	out := make(map[string]bool, len(orderUpdateSpecs))
	for _, spec := range orderUpdateSpecs {
		out[spec.key] = true
	}
	return out
}()

// UpdateOrderFields applies whitelisted column updates with optimistic locking.
func (s *Store) UpdateOrderFields(ctx context.Context, orderID string, fields map[string]any, expectedVersion int, now time.Time) error {
	if len(fields) == 0 {
		return fmt.Errorf("no fields to update")
	}
	for key := range fields {
		if !orderUpdateColumns[key] {
			return fmt.Errorf("field %q is not updatable", key)
		}
	}
	set := make([]string, 0, len(fields)+2)
	args := make([]any, 0, len(fields)+4)
	// Deterministic ordering keeps SQL stable for tests and logs.
	for _, spec := range orderUpdateSpecs {
		value, ok := fields[spec.key]
		if !ok {
			continue
		}
		set = append(set, spec.column+" = ?")
		args = append(args, value)
	}
	if len(set) == 0 {
		return fmt.Errorf("no whitelisted fields supplied")
	}
	set = append(set, "updated_at = ?", "version = version + 1")
	args = append(args, ts(now), orderID, expectedVersion)

	res, err := s.conn().ExecContext(ctx,
		`UPDATE orders SET `+strings.Join(set, ", ")+` WHERE id = ? AND version = ?`, args...)
	if err != nil {
		return translateErr(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return translateErr(err)
	}
	if affected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

// CancelOrder marks an unshipped order as cancelled with optimistic locking and
// a status guard, so a concurrent shipment cannot be silently overwritten.
func (s *Store) CancelOrder(ctx context.Context, orderID string, expectedVersion int, now time.Time) error {
	res, err := s.conn().ExecContext(ctx, `
		UPDATE orders
		   SET order_status = 'cancelled', cancelled_at = ?, updated_at = ?, version = version + 1
		 WHERE id = ? AND version = ? AND order_status IN ('pending_payment','paid','processing')`,
		ts(now), ts(now), orderID, expectedVersion)
	if err != nil {
		return translateErr(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return translateErr(err)
	}
	if affected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

// terminalCaseStatuses are service case states that no longer block a new request
// of the same type for the same item.
var terminalCaseStatuses = []string{"completed", "cancelled", "rejected"}

// CountActiveServiceCases counts open cases of one type for an order.
func (s *Store) CountActiveServiceCases(ctx context.Context, orderID, caseType string) (int, error) {
	query := `SELECT COUNT(1) FROM service_cases WHERE order_id = ? AND case_type = ? AND case_status NOT IN ('` +
		strings.Join(terminalCaseStatuses, "','") + `')`
	var count int
	if err := s.conn().QueryRowContext(ctx, query, orderID, caseType).Scan(&count); err != nil {
		return 0, translateErr(err)
	}
	return count, nil
}

// SumOpenRequestedQuantity sums requested quantities of open cases for an item.
func (s *Store) SumOpenRequestedQuantity(ctx context.Context, orderItemID string) (int, error) {
	query := `SELECT COALESCE(SUM(requested_quantity),0) FROM service_cases
	          WHERE order_item_id = ? AND case_status NOT IN ('` + strings.Join(terminalCaseStatuses, "','") + `')`
	var total sql.NullInt64
	if err := s.conn().QueryRowContext(ctx, query, orderItemID).Scan(&total); err != nil {
		return 0, translateErr(err)
	}
	return int(total.Int64), nil
}
