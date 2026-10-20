package admin

import (
	"context"
	"database/sql"
	"fmt"
)

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

// CustomerFilter narrows the customer list.
type CustomerFilter struct {
	Query  string
	Status string
	Page   Page
}

// Customers pages customers with their lifetime counters.
func (s *Store) Customers(ctx context.Context, f CustomerFilter) (List[CustomerRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("cu.status = ?", f.Status)
	}
	w.like([]string{"cu.customer_no", "COALESCE(cu.display_name, '')", "COALESCE(cu.phone_masked, '')"}, f.Query)

	countQ := `SELECT COUNT(*) FROM customers cu` + w.clause()
	listQ := `SELECT cu.id, cu.customer_no, COALESCE(cu.display_name, ''), COALESCE(cu.phone_masked, ''),
		cu.status,
		(SELECT COUNT(*) FROM orders o WHERE o.customer_id = cu.id),
		(SELECT COALESCE(SUM(o.paid_amount_minor), 0) FROM orders o
		   WHERE o.customer_id = cu.id AND o.order_status NOT IN ('cancelled','closed')),
		(SELECT COUNT(*) FROM conversations c WHERE c.customer_id = cu.id),
		(SELECT COUNT(*) FROM service_tickets t WHERE t.customer_id = cu.id),
		cu.created_at
		FROM customers cu` + w.clause() + ` ORDER BY cu.customer_no`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (CustomerRow, error) {
		var c CustomerRow
		err := r.Scan(&c.ID, &c.CustomerNo, &c.DisplayName, &c.PhoneMasked, &c.Status,
			&c.OrderCount, &c.PaidAmount, &c.ConversationCount, &c.TicketCount, &c.CreatedAt)
		c.PaidDisplay = Money(c.PaidAmount)
		return c, err
	})
}

// CustomerDetail assembles the customer drawer.
func (s *Store) CustomerDetail(ctx context.Context, customerNo string) (CustomerDetail, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT cu.id, cu.customer_no, COALESCE(cu.display_name, ''),
		COALESCE(cu.phone_masked, ''), cu.status,
		(SELECT COUNT(*) FROM orders o WHERE o.customer_id = cu.id),
		(SELECT COALESCE(SUM(o.paid_amount_minor), 0) FROM orders o
		   WHERE o.customer_id = cu.id AND o.order_status NOT IN ('cancelled','closed')),
		(SELECT COUNT(*) FROM conversations c WHERE c.customer_id = cu.id),
		(SELECT COUNT(*) FROM service_tickets t WHERE t.customer_id = cu.id),
		cu.created_at
		FROM customers cu WHERE cu.customer_no = ?`, customerNo)
	if err != nil {
		return CustomerDetail{}, fmt.Errorf("load customer: %w", err)
	}
	var c CustomerRow
	if rows.Next() {
		if err := rows.Scan(&c.ID, &c.CustomerNo, &c.DisplayName, &c.PhoneMasked, &c.Status,
			&c.OrderCount, &c.PaidAmount, &c.ConversationCount, &c.TicketCount, &c.CreatedAt); err != nil {
			rows.Close()
			return CustomerDetail{}, err
		}
	} else {
		rows.Close()
		return CustomerDetail{}, errNotFound("客户不存在")
	}
	rows.Close()
	c.PaidDisplay = Money(c.PaidAmount)

	orders, err := s.Orders(ctx, OrderFilter{CustomerID: c.ID, Page: Page{Page: 1, PageSize: 20}})
	if err != nil {
		return CustomerDetail{}, err
	}
	convs, err := s.conversationsByCustomer(ctx, c.ID, 20)
	if err != nil {
		return CustomerDetail{}, err
	}
	tickets, err := s.ticketsByCustomer(ctx, c.ID, 20)
	if err != nil {
		return CustomerDetail{}, err
	}
	benefits, err := s.benefitsByCustomer(ctx, c.ID)
	if err != nil {
		return CustomerDetail{}, err
	}
	return CustomerDetail{
		Customer: c, Orders: orders.Items, Conversations: convs, Tickets: tickets, Benefits: benefits,
	}, nil
}

func (s *Store) conversationsByCustomer(ctx context.Context, customerID string, limit int) ([]ConversationRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.conversation_no, c.customer_id, cu.customer_no,
		COALESCE(cu.display_name, ''), c.status, c.channel, COALESCE(c.summary, ''),
		(SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id),
		(SELECT COUNT(*) FROM tool_executions t WHERE t.conversation_id = c.id),
		(SELECT COUNT(*) FROM pending_actions p WHERE p.conversation_id = c.id AND p.status = 'pending'),
		c.created_at, COALESCE(c.last_message_at, '')
		FROM conversations c JOIN customers cu ON cu.id = c.customer_id
		WHERE c.customer_id = ? ORDER BY COALESCE(c.last_message_at, c.created_at) DESC LIMIT ?`,
		customerID, limit)
	if err != nil {
		return nil, fmt.Errorf("customer conversations: %w", err)
	}
	defer rows.Close()

	out := []ConversationRow{}
	for rows.Next() {
		var c ConversationRow
		if err := rows.Scan(&c.ID, &c.ConversationNo, &c.CustomerID, &c.CustomerNo, &c.CustomerName,
			&c.Status, &c.Channel, &c.Summary, &c.MessageCount, &c.ToolCount, &c.PendingCount,
			&c.CreatedAt, &c.LastMessageAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ticketsByCustomer(ctx context.Context, customerID string, limit int) ([]TicketRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.ticket_no, t.customer_id, cu.customer_no,
		COALESCE(cu.display_name, ''), COALESCE(t.conversation_id, ''), COALESCE(o.order_no, ''),
		t.ticket_type, t.status, t.priority, COALESCE(t.category, ''), t.summary,
		COALESCE(t.assigned_to, ''), t.created_at, t.updated_at
		FROM service_tickets t
		JOIN customers cu ON cu.id = t.customer_id
		LEFT JOIN orders o ON o.id = t.order_id
		WHERE t.customer_id = ? ORDER BY t.created_at DESC LIMIT ?`, customerID, limit)
	if err != nil {
		return nil, fmt.Errorf("customer tickets: %w", err)
	}
	defer rows.Close()

	out := []TicketRow{}
	for rows.Next() {
		var t TicketRow
		if err := rows.Scan(&t.ID, &t.TicketNo, &t.CustomerID, &t.CustomerNo, &t.CustomerName,
			&t.ConversationID, &t.OrderNo, &t.TicketType, &t.Status, &t.Priority,
			&t.Category, &t.Summary, &t.AssignedTo, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.AgeHours = ageHours(t.CreatedAt)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) benefitsByCustomer(ctx context.Context, customerID string) ([]BenefitRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, benefit_type, COALESCE(benefit_code, ''), status,
		amount_minor, points, COALESCE(expires_at, ''), created_at
		FROM customer_benefits WHERE customer_id = ? ORDER BY created_at DESC LIMIT 50`, customerID)
	if err != nil {
		return nil, fmt.Errorf("customer benefits: %w", err)
	}
	defer rows.Close()

	out := []BenefitRow{}
	for rows.Next() {
		var b BenefitRow
		var amount, points sql.NullInt64
		if err := rows.Scan(&b.ID, &b.BenefitType, &b.BenefitCode, &b.Status,
			&amount, &points, &b.ExpiresAt, &b.CreatedAt); err != nil {
			return nil, err
		}
		if amount.Valid {
			b.Amount = Money(amount.Int64)
		}
		b.Points = nullInt(points)
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Orders
// ---------------------------------------------------------------------------

// OrderFilter narrows the order list.
type OrderFilter struct {
	Query      string
	Status     string
	CustomerID string
	Page       Page
}

// Orders pages orders across all customers.
func (s *Store) Orders(ctx context.Context, f OrderFilter) (List[OrderRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("o.order_status = ?", f.Status)
	}
	if f.CustomerID != "" {
		w.add("o.customer_id = ?", f.CustomerID)
	}
	w.like([]string{"o.order_no", "cu.customer_no", "COALESCE(cu.display_name, '')", "COALESCE(o.receiver_name, '')"}, f.Query)

	from := ` FROM orders o JOIN customers cu ON cu.id = o.customer_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT o.id, o.order_no, cu.customer_no, COALESCE(cu.display_name, ''), o.order_status,
		o.order_channel, o.paid_amount_minor, o.refunded_amount_minor,
		(SELECT COUNT(*) FROM order_items i WHERE i.order_id = o.id),
		o.is_presale, o.created_at, COALESCE(o.paid_at, '')` + from + w.clause() +
		` ORDER BY o.created_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (OrderRow, error) {
		var o OrderRow
		err := r.Scan(&o.ID, &o.OrderNo, &o.CustomerNo, &o.CustomerName, &o.Status, &o.Channel,
			&o.Paid, &o.Refunded, &o.ItemCount, &o.IsPresale, &o.CreatedAt, &o.PaidAt)
		o.PaidDisplay = Money(o.Paid)
		o.RefundedDisplay = Money(o.Refunded)
		return o, err
	})
}

// OrderDetail assembles the order drawer: items, parcels, money, after-sales.
func (s *Store) OrderDetail(ctx context.Context, orderNo string) (OrderDetail, error) {
	var out OrderDetail
	var orderID, customerID string
	var receiverName, receiverPhone, address, method, remark sql.NullString
	var province, city, district sql.NullString

	err := s.db.QueryRowContext(ctx, `SELECT o.id, o.customer_id, o.order_no, cu.customer_no,
		COALESCE(cu.display_name, ''), o.order_status, o.order_channel, o.paid_amount_minor,
		o.refunded_amount_minor, (SELECT COUNT(*) FROM order_items i WHERE i.order_id = o.id),
		o.is_presale, o.created_at, COALESCE(o.paid_at, ''),
		o.receiver_name, o.receiver_phone_masked, o.shipping_province, o.shipping_city,
		o.shipping_district, o.shipping_address, o.shipping_method, o.order_remark
		FROM orders o JOIN customers cu ON cu.id = o.customer_id WHERE o.order_no = ?`, orderNo).
		Scan(&orderID, &customerID, &out.Order.OrderNo, &out.Order.CustomerNo, &out.Order.CustomerName,
			&out.Order.Status, &out.Order.Channel, &out.Order.Paid, &out.Order.Refunded,
			&out.Order.ItemCount, &out.Order.IsPresale, &out.Order.CreatedAt, &out.Order.PaidAt,
			&receiverName, &receiverPhone, &province, &city, &district, &address, &method, &remark)
	if err == sql.ErrNoRows {
		return OrderDetail{}, errNotFound("订单不存在")
	}
	if err != nil {
		return OrderDetail{}, fmt.Errorf("load order: %w", err)
	}
	out.Order.ID = orderID
	out.Order.PaidDisplay = Money(out.Order.Paid)
	out.Order.RefundedDisplay = Money(out.Order.Refunded)

	addr := ""
	for _, part := range []string{nullStr(province), nullStr(city), nullStr(district), nullStr(address)} {
		if part != "" {
			addr += part
		}
	}
	out.Receiver = ReceiverInfo{
		Name: nullStr(receiverName), Phone: nullStr(receiverPhone), Address: addr,
		Method: nullStr(method), Remark: nullStr(remark),
	}

	if out.Items, err = s.orderItems(ctx, orderID); err != nil {
		return OrderDetail{}, err
	}
	if out.Shipments, err = s.shipments(ctx, orderID); err != nil {
		return OrderDetail{}, err
	}
	if out.Payments, err = s.payments(ctx, orderID); err != nil {
		return OrderDetail{}, err
	}
	if out.Refunds, err = s.refundsByOrder(ctx, orderID); err != nil {
		return OrderDetail{}, err
	}
	if out.Cases, err = s.casesByOrder(ctx, orderID); err != nil {
		return OrderDetail{}, err
	}
	if out.Invoices, err = s.invoicesByOrder(ctx, orderID); err != nil {
		return OrderDetail{}, err
	}
	audit, err := s.AuditLogs(ctx, AuditFilter{TargetType: "order", TargetID: orderID, Page: Page{Page: 1, PageSize: 20}})
	if err != nil {
		return OrderDetail{}, err
	}
	out.Audit = audit.Items
	return out, nil
}

func (s *Store) orderItems(ctx context.Context, orderID string) ([]OrderItemRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, product_name_snapshot, COALESCE(sku_name_snapshot, ''),
		COALESCE(product_code_snapshot, ''), quantity, refunded_quantity, unit_price_minor,
		paid_amount_minor, item_status
		FROM order_items WHERE order_id = ? ORDER BY created_at`, orderID)
	if err != nil {
		return nil, fmt.Errorf("order items: %w", err)
	}
	defer rows.Close()

	out := []OrderItemRow{}
	for rows.Next() {
		var i OrderItemRow
		if err := rows.Scan(&i.ID, &i.ProductName, &i.SKUName, &i.ProductCode, &i.Quantity,
			&i.RefundedQty, &i.UnitPrice, &i.PaidAmount, &i.Status); err != nil {
			return nil, err
		}
		i.UnitDisplay = Money(i.UnitPrice)
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) shipments(ctx context.Context, orderID string) ([]ShipmentRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.shipment_no, COALESCE(s.carrier_name, ''),
		COALESCE(s.tracking_no, ''), s.shipment_status, COALESCE(s.estimated_delivery_at, ''),
		COALESCE(s.last_event_at, ''),
		COALESCE((SELECT e.description FROM shipment_events e WHERE e.shipment_id = s.id
		          ORDER BY e.event_time DESC LIMIT 1), '')
		FROM shipments s WHERE s.order_id = ? ORDER BY s.created_at`, orderID)
	if err != nil {
		return nil, fmt.Errorf("shipments: %w", err)
	}
	defer rows.Close()

	out := []ShipmentRow{}
	for rows.Next() {
		var p ShipmentRow
		if err := rows.Scan(&p.ID, &p.ShipmentNo, &p.CarrierName, &p.TrackingNo, &p.Status,
			&p.EstimatedAt, &p.LastEventAt, &p.LatestEvent); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) payments(ctx context.Context, orderID string) ([]PaymentRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, payment_no, payment_method, payment_status,
		amount_minor, refunded_amount_minor, COALESCE(paid_at, ''), created_at
		FROM payments WHERE order_id = ? ORDER BY created_at`, orderID)
	if err != nil {
		return nil, fmt.Errorf("payments: %w", err)
	}
	defer rows.Close()

	out := []PaymentRow{}
	for rows.Next() {
		var p PaymentRow
		if err := rows.Scan(&p.ID, &p.PaymentNo, &p.Method, &p.Status, &p.Amount,
			&p.Refunded, &p.PaidAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.AmountDisplay = Money(p.Amount)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) refundsByOrder(ctx context.Context, orderID string) ([]RefundRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.refund_no, o.order_no, cu.customer_no,
		r.refund_status, COALESCE(r.refund_channel, ''), r.amount_minor, COALESCE(r.reason, ''),
		COALESCE(r.failure_reason, ''), r.requested_at, COALESCE(r.completed_at, '')
		FROM refunds r JOIN orders o ON o.id = r.order_id JOIN customers cu ON cu.id = o.customer_id
		WHERE r.order_id = ? ORDER BY r.requested_at DESC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("order refunds: %w", err)
	}
	defer rows.Close()
	return scanRefunds(rows)
}

func (s *Store) casesByOrder(ctx context.Context, orderID string) ([]ServiceCaseRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sc.id, sc.case_no, o.order_no, cu.customer_no,
		sc.case_type, sc.case_status, COALESCE(sc.reason_code, ''), COALESCE(sc.description, ''),
		sc.requested_quantity, COALESCE(sc.resolution, ''), sc.created_at, COALESCE(sc.closed_at, '')
		FROM service_cases sc JOIN orders o ON o.id = sc.order_id
		JOIN customers cu ON cu.id = sc.customer_id
		WHERE sc.order_id = ? ORDER BY sc.created_at DESC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("order cases: %w", err)
	}
	defer rows.Close()
	return scanCases(rows)
}

func (s *Store) invoicesByOrder(ctx context.Context, orderID string) ([]InvoiceRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.id, COALESCE(i.invoice_no, ''), o.order_no, cu.customer_no,
		i.invoice_type, i.invoice_status, i.title, i.amount_minor, i.requested_at, COALESCE(i.issued_at, '')
		FROM invoices i JOIN orders o ON o.id = i.order_id JOIN customers cu ON cu.id = i.customer_id
		WHERE i.order_id = ? ORDER BY i.created_at DESC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("order invoices: %w", err)
	}
	defer rows.Close()
	return scanInvoices(rows)
}

// ---------------------------------------------------------------------------
// Products
// ---------------------------------------------------------------------------

// ProductFilter narrows the catalog list.
type ProductFilter struct {
	Query    string
	Category string
	Status   string
	Page     Page
}

// Products pages the catalog with SKU rollups.
func (s *Store) Products(ctx context.Context, f ProductFilter) (List[ProductRow], error) {
	w := &where{}
	if f.Category != "" {
		w.add("p.category = ?", f.Category)
	}
	if f.Status != "" {
		w.add("p.status = ?", f.Status)
	}
	w.like([]string{"p.name", "p.product_code", "COALESCE(p.brand, '')"}, f.Query)

	countQ := `SELECT COUNT(*) FROM products p` + w.clause()
	listQ := `SELECT p.id, p.product_code, p.name, COALESCE(p.brand, ''), COALESCE(p.category, ''),
		p.status,
		(SELECT COUNT(*) FROM product_skus s WHERE s.product_id = p.id),
		(SELECT COALESCE(SUM(s.stock_quantity), 0) FROM product_skus s WHERE s.product_id = p.id),
		(SELECT COALESCE(MIN(s.list_price_minor), 0) FROM product_skus s WHERE s.product_id = p.id),
		(SELECT COALESCE(MAX(s.list_price_minor), 0) FROM product_skus s WHERE s.product_id = p.id),
		p.created_at
		FROM products p` + w.clause() + ` ORDER BY p.category, p.name`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (ProductRow, error) {
		var p ProductRow
		err := r.Scan(&p.ID, &p.ProductCode, &p.Name, &p.Brand, &p.Category, &p.Status,
			&p.SKUCount, &p.StockTotal, &p.PriceFrom, &p.PriceTo, &p.CreatedAt)
		if p.PriceFrom == p.PriceTo {
			p.PriceRange = Money(p.PriceFrom)
		} else {
			p.PriceRange = Money(p.PriceFrom) + " – " + Money(p.PriceTo)
		}
		return p, err
	})
}

// ProductDetail loads one product by UUID or product_code.
func (s *Store) ProductDetail(ctx context.Context, idOrCode string) (ProductDetail, error) {
	var out ProductDetail
	var attrs string
	err := s.db.QueryRowContext(ctx, `SELECT id, product_code, name, COALESCE(brand, ''),
		COALESCE(category, ''), status, attributes_json, COALESCE(description, ''), created_at
		FROM products WHERE id = ? OR product_code = ?`, idOrCode, idOrCode).
		Scan(&out.Product.ID, &out.Product.ProductCode, &out.Product.Name, &out.Product.Brand,
			&out.Product.Category, &out.Product.Status, &attrs, &out.Description, &out.Product.CreatedAt)
	if err == sql.ErrNoRows {
		return ProductDetail{}, errNotFound("商品不存在")
	}
	if err != nil {
		return ProductDetail{}, fmt.Errorf("load product: %w", err)
	}
	out.Attributes = attrs

	rows, err := s.db.QueryContext(ctx, `SELECT id, sku_code, sku_name, specifications_json,
		list_price_minor, stock_quantity, status FROM product_skus WHERE product_id = ? ORDER BY sku_code`,
		out.Product.ID)
	if err != nil {
		return ProductDetail{}, fmt.Errorf("load skus: %w", err)
	}
	defer rows.Close()

	out.SKUs = []SKURow{}
	for rows.Next() {
		var k SKURow
		if err := rows.Scan(&k.ID, &k.SKUCode, &k.SKUName, &k.Specs, &k.Price, &k.Stock, &k.Status); err != nil {
			return ProductDetail{}, err
		}
		k.PriceDisplay = Money(k.Price)
		out.SKUs = append(out.SKUs, k)
		out.Product.SKUCount++
		out.Product.StockTotal += k.Stock
		if out.Product.PriceFrom == 0 || k.Price < out.Product.PriceFrom {
			out.Product.PriceFrom = k.Price
		}
		if k.Price > out.Product.PriceTo {
			out.Product.PriceTo = k.Price
		}
	}
	if out.Product.PriceFrom == out.Product.PriceTo {
		out.Product.PriceRange = Money(out.Product.PriceFrom)
	} else {
		out.Product.PriceRange = Money(out.Product.PriceFrom) + " – " + Money(out.Product.PriceTo)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// After-sales, refunds, invoices
// ---------------------------------------------------------------------------

// CaseFilter narrows after-sales cases.
type CaseFilter struct {
	Query  string
	Status string
	Type   string
	Page   Page
}

// ServiceCases pages after-sales cases.
func (s *Store) ServiceCases(ctx context.Context, f CaseFilter) (List[ServiceCaseRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("sc.case_status = ?", f.Status)
	}
	if f.Type != "" {
		w.add("sc.case_type = ?", f.Type)
	}
	w.like([]string{"sc.case_no", "o.order_no", "cu.customer_no", "COALESCE(sc.description, '')"}, f.Query)

	from := ` FROM service_cases sc
		JOIN orders o ON o.id = sc.order_id
		JOIN customers cu ON cu.id = sc.customer_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT sc.id, sc.case_no, o.order_no, cu.customer_no, sc.case_type, sc.case_status,
		COALESCE(sc.reason_code, ''), COALESCE(sc.description, ''), sc.requested_quantity,
		COALESCE(sc.resolution, ''), sc.created_at, COALESCE(sc.closed_at, '')` + from + w.clause() +
		` ORDER BY sc.created_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (ServiceCaseRow, error) {
		var c ServiceCaseRow
		err := r.Scan(&c.ID, &c.CaseNo, &c.OrderNo, &c.CustomerNo, &c.CaseType, &c.Status,
			&c.ReasonCode, &c.Description, &c.Quantity, &c.Resolution, &c.CreatedAt, &c.ClosedAt)
		return c, err
	})
}

// RefundFilter narrows refunds.
type RefundFilter struct {
	Query  string
	Status string
	Page   Page
}

// Refunds pages refunds across orders.
func (s *Store) Refunds(ctx context.Context, f RefundFilter) (List[RefundRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("r.refund_status = ?", f.Status)
	}
	w.like([]string{"r.refund_no", "o.order_no", "cu.customer_no"}, f.Query)

	from := ` FROM refunds r JOIN orders o ON o.id = r.order_id JOIN customers cu ON cu.id = o.customer_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT r.id, r.refund_no, o.order_no, cu.customer_no, r.refund_status,
		COALESCE(r.refund_channel, ''), r.amount_minor, COALESCE(r.reason, ''),
		COALESCE(r.failure_reason, ''), r.requested_at, COALESCE(r.completed_at, '')` + from + w.clause() +
		` ORDER BY r.requested_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (RefundRow, error) {
		var x RefundRow
		err := r.Scan(&x.ID, &x.RefundNo, &x.OrderNo, &x.CustomerNo, &x.Status, &x.Channel,
			&x.Amount, &x.Reason, &x.FailureReason, &x.RequestedAt, &x.CompletedAt)
		x.AmountDisplay = Money(x.Amount)
		return x, err
	})
}

// InvoiceFilter narrows invoices.
type InvoiceFilter struct {
	Query  string
	Status string
	Page   Page
}

// Invoices pages invoices.
func (s *Store) Invoices(ctx context.Context, f InvoiceFilter) (List[InvoiceRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("i.invoice_status = ?", f.Status)
	}
	w.like([]string{"COALESCE(i.invoice_no, '')", "o.order_no", "cu.customer_no", "i.title"}, f.Query)

	from := ` FROM invoices i JOIN orders o ON o.id = i.order_id JOIN customers cu ON cu.id = i.customer_id`
	countQ := `SELECT COUNT(*)` + from + w.clause()
	listQ := `SELECT i.id, COALESCE(i.invoice_no, ''), o.order_no, cu.customer_no, i.invoice_type,
		i.invoice_status, i.title, i.amount_minor, i.requested_at, COALESCE(i.issued_at, '')` +
		from + w.clause() + ` ORDER BY i.requested_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (InvoiceRow, error) {
		var x InvoiceRow
		err := r.Scan(&x.ID, &x.InvoiceNo, &x.OrderNo, &x.CustomerNo, &x.Type, &x.Status,
			&x.Title, &x.Amount, &x.RequestedAt, &x.IssuedAt)
		x.AmountDisplay = Money(x.Amount)
		return x, err
	})
}

// PromotionFilter narrows campaigns.
type PromotionFilter struct {
	Query  string
	Status string
	Page   Page
}

// Promotions pages campaigns.
func (s *Store) Promotions(ctx context.Context, f PromotionFilter) (List[PromotionRow], error) {
	w := &where{}
	if f.Status != "" {
		w.add("status = ?", f.Status)
	}
	w.like([]string{"promotion_code", "title", "COALESCE(description, '')"}, f.Query)

	countQ := `SELECT COUNT(*) FROM promotions` + w.clause()
	listQ := `SELECT id, promotion_code, title, promotion_type, status, COALESCE(starts_at, ''),
		COALESCE(ends_at, ''), rules_json, COALESCE(description, '') FROM promotions` + w.clause() +
		` ORDER BY COALESCE(starts_at, created_at) DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (PromotionRow, error) {
		var p PromotionRow
		err := r.Scan(&p.ID, &p.PromotionCode, &p.Title, &p.Type, &p.Status,
			&p.StartsAt, &p.EndsAt, &p.Rules, &p.Description)
		return p, err
	})
}

// ---------------------------------------------------------------------------
// Row scanners shared by several queries
// ---------------------------------------------------------------------------

func scanRefunds(rows *sql.Rows) ([]RefundRow, error) {
	out := []RefundRow{}
	for rows.Next() {
		var x RefundRow
		if err := rows.Scan(&x.ID, &x.RefundNo, &x.OrderNo, &x.CustomerNo, &x.Status, &x.Channel,
			&x.Amount, &x.Reason, &x.FailureReason, &x.RequestedAt, &x.CompletedAt); err != nil {
			return nil, err
		}
		x.AmountDisplay = Money(x.Amount)
		out = append(out, x)
	}
	return out, rows.Err()
}

func scanCases(rows *sql.Rows) ([]ServiceCaseRow, error) {
	out := []ServiceCaseRow{}
	for rows.Next() {
		var c ServiceCaseRow
		if err := rows.Scan(&c.ID, &c.CaseNo, &c.OrderNo, &c.CustomerNo, &c.CaseType, &c.Status,
			&c.ReasonCode, &c.Description, &c.Quantity, &c.Resolution, &c.CreatedAt, &c.ClosedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanInvoices(rows *sql.Rows) ([]InvoiceRow, error) {
	out := []InvoiceRow{}
	for rows.Next() {
		var x InvoiceRow
		if err := rows.Scan(&x.ID, &x.InvoiceNo, &x.OrderNo, &x.CustomerNo, &x.Type, &x.Status,
			&x.Title, &x.Amount, &x.RequestedAt, &x.IssuedAt); err != nil {
			return nil, err
		}
		x.AmountDisplay = Money(x.Amount)
		out = append(out, x)
	}
	return out, rows.Err()
}
