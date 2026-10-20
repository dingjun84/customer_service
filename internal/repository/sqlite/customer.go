package sqlite

import (
	"context"

	"github.com/example/ecommerce-support/internal/domain"
)

const customerColumns = `id, customer_no, COALESCE(display_name,''), COALESCE(phone_masked,''), status, created_at, updated_at`

func scanCustomer(scan func(dest ...any) error) (*domain.Customer, error) {
	var c domain.Customer
	var created, updated string
	if err := scan(&c.ID, &c.CustomerNo, &c.DisplayName, &c.PhoneMasked, &c.Status, &created, &updated); err != nil {
		return nil, translateErr(err)
	}
	c.CreatedAt = parseTS(created)
	c.UpdatedAt = parseTS(updated)
	return &c, nil
}

// GetCustomer loads a customer by internal ID.
func (s *Store) GetCustomer(ctx context.Context, id string) (*domain.Customer, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+customerColumns+` FROM customers WHERE id = ?`, id)
	return scanCustomer(row.Scan)
}

// GetCustomerByNo loads a customer by business number (e.g. CUST-1001).
func (s *Store) GetCustomerByNo(ctx context.Context, customerNo string) (*domain.Customer, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+customerColumns+` FROM customers WHERE customer_no = ?`, customerNo)
	return scanCustomer(row.Scan)
}
