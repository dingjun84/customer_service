package sqlite

import (
	"context"

	"github.com/example/ecommerce-support/internal/domain"
)

const serviceCaseColumns = `id, case_no, customer_id, order_id, COALESCE(order_item_id,''), case_type, case_status,
	COALESCE(reason_code,''), COALESCE(reason_text,''), COALESCE(description,''), requested_quantity,
	COALESCE(resolution,''), created_at, updated_at, COALESCE(closed_at,'')`

func scanServiceCase(scan func(dest ...any) error) (*domain.ServiceCase, error) {
	var c domain.ServiceCase
	var created, updated, closed string
	if err := scan(&c.ID, &c.CaseNo, &c.CustomerID, &c.OrderID, &c.OrderItemID, &c.CaseType, &c.CaseStatus,
		&c.ReasonCode, &c.ReasonText, &c.Description, &c.RequestedQuantity,
		&c.Resolution, &created, &updated, &closed); err != nil {
		return nil, translateErr(err)
	}
	c.CreatedAt = parseTS(created)
	c.UpdatedAt = parseTS(updated)
	c.ClosedAt = parseTSPtr(closed)
	return &c, nil
}

// ListServiceCasesByOrder returns after-sales cases of an order, newest first.
func (s *Store) ListServiceCasesByOrder(ctx context.Context, orderID string) ([]domain.ServiceCase, error) {
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+serviceCaseColumns+` FROM service_cases WHERE order_id = ? ORDER BY created_at DESC, id DESC`, orderID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.ServiceCase{}
	for rows.Next() {
		c, err := scanServiceCase(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, translateErr(rows.Err())
}

// GetServiceCaseByNo loads a case by business number.
func (s *Store) GetServiceCaseByNo(ctx context.Context, caseNo string) (*domain.ServiceCase, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+serviceCaseColumns+` FROM service_cases WHERE case_no = ?`, caseNo)
	return scanServiceCase(row.Scan)
}

// CreateServiceCase inserts a new after-sales case.
func (s *Store) CreateServiceCase(ctx context.Context, c *domain.ServiceCase) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO service_cases (id, case_no, customer_id, order_id, order_item_id, case_type, case_status,
		                           reason_code, reason_text, description, requested_quantity, resolution,
		                           created_at, updated_at, closed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.CaseNo, c.CustomerID, c.OrderID, nullString(c.OrderItemID), c.CaseType, c.CaseStatus,
		nullString(c.ReasonCode), nullString(c.ReasonText), nullString(c.Description), c.RequestedQuantity,
		nullString(c.Resolution), ts(c.CreatedAt), ts(c.UpdatedAt), tsPtr(c.ClosedAt))
	return translateErr(err)
}

const invoiceColumns = `id, COALESCE(invoice_no,''), customer_id, order_id, invoice_type, invoice_status,
	title, COALESCE(tax_no,''), amount_minor, COALESCE(email,''), COALESCE(file_url,''),
	requested_at, COALESCE(issued_at,''), created_at, updated_at`

// ListInvoicesByOrder returns invoices of an order, newest first.
func (s *Store) ListInvoicesByOrder(ctx context.Context, orderID string) ([]domain.Invoice, error) {
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+invoiceColumns+` FROM invoices WHERE order_id = ? ORDER BY created_at DESC, id DESC`, orderID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.Invoice{}
	for rows.Next() {
		var inv domain.Invoice
		var requestedAt, issuedAt, created, updated string
		if err := rows.Scan(&inv.ID, &inv.InvoiceNo, &inv.CustomerID, &inv.OrderID, &inv.InvoiceType, &inv.InvoiceStatus,
			&inv.Title, &inv.TaxNo, &inv.AmountMinor, &inv.Email, &inv.FileURL,
			&requestedAt, &issuedAt, &created, &updated); err != nil {
			return nil, translateErr(err)
		}
		inv.RequestedAt = parseTS(requestedAt)
		inv.IssuedAt = parseTSPtr(issuedAt)
		inv.CreatedAt = parseTS(created)
		inv.UpdatedAt = parseTS(updated)
		out = append(out, inv)
	}
	return out, translateErr(rows.Err())
}
