package sqlite

import (
	"context"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
)

const productColumns = `id, product_code, name, COALESCE(brand,''), COALESCE(category,''), COALESCE(description,''), status, attributes_json, created_at, updated_at`

func scanProduct(scan func(dest ...any) error) (*domain.Product, error) {
	var p domain.Product
	var created, updated string
	if err := scan(&p.ID, &p.ProductCode, &p.Name, &p.Brand, &p.Category, &p.Description, &p.Status,
		&p.AttributesJSON, &created, &updated); err != nil {
		return nil, translateErr(err)
	}
	p.CreatedAt = parseTS(created)
	p.UpdatedAt = parseTS(updated)
	p.Attributes = jsonObject(p.AttributesJSON)
	return &p, nil
}

// SearchProducts performs a LIKE based search over publicly safe product fields.
// Only active products are returned.
func (s *Store) SearchProducts(ctx context.Context, query, category string, limit int) ([]domain.Product, error) {
	limit = limitOrDefault(limit, 20, 50)
	where := []string{"status = 'active'"}
	args := []any{}
	if q := strings.TrimSpace(query); q != "" {
		like := "%" + escapeLike(q) + "%"
		where = append(where, `(name LIKE ? ESCAPE '\' OR COALESCE(brand,'') LIKE ? ESCAPE '\' OR COALESCE(category,'') LIKE ? ESCAPE '\' OR COALESCE(description,'') LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like, like)
	}
	if c := strings.TrimSpace(category); c != "" {
		where = append(where, "category = ?")
		args = append(args, c)
	}
	args = append(args, limit)

	sqlText := `SELECT ` + productColumns + ` FROM products WHERE ` + strings.Join(where, " AND ") + ` ORDER BY name LIMIT ?`
	rows, err := s.conn().QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := make([]domain.Product, 0, limit)
	for rows.Next() {
		p, err := scanProduct(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, translateErr(rows.Err())
}

// GetProduct loads a product by ID.
func (s *Store) GetProduct(ctx context.Context, id string) (*domain.Product, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id = ?`, id)
	return scanProduct(row.Scan)
}

// GetProductByCode loads a product by its business code (e.g. SKU-P-001).
func (s *Store) GetProductByCode(ctx context.Context, code string) (*domain.Product, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE product_code = ?`, code)
	return scanProduct(row.Scan)
}

const skuColumns = `id, product_id, sku_code, sku_name, specifications_json, list_price_minor, stock_quantity, status, created_at, updated_at`

// ListSKUs returns all SKUs of a product ordered by price.
func (s *Store) ListSKUs(ctx context.Context, productID string) ([]domain.ProductSKU, error) {
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+skuColumns+` FROM product_skus WHERE product_id = ? ORDER BY list_price_minor`, productID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.ProductSKU{}
	for rows.Next() {
		var k domain.ProductSKU
		var created, updated string
		if err := rows.Scan(&k.ID, &k.ProductID, &k.SKUCode, &k.SKUName, &k.SpecificationsJSON,
			&k.ListPriceMinor, &k.StockQuantity, &k.Status, &created, &updated); err != nil {
			return nil, translateErr(err)
		}
		k.CreatedAt = parseTS(created)
		k.UpdatedAt = parseTS(updated)
		k.Specifications = jsonObject(k.SpecificationsJSON)
		out = append(out, k)
	}
	return out, translateErr(rows.Err())
}

// escapeLike neutralises LIKE wildcards in user supplied search text.
func escapeLike(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}
