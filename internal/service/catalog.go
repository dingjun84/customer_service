package service

import (
	"context"
	"errors"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
)

// Catalog serves product and SKU lookups. Only public catalog fields are exposed.
type Catalog struct{ core }

// Search returns active products matching a free-text query.
func (s *Catalog) Search(ctx context.Context, query, category string, limit int) ([]domain.Product, error) {
	products, err := s.store.SearchProducts(ctx, query, category, limit)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return products, nil
}

// Detail loads a product by UUID or product_code and attaches its SKUs.
func (s *Catalog) Detail(ctx context.Context, productIDOrCode string) (*domain.Product, error) {
	id := strings.TrimSpace(productIDOrCode)
	if id == "" {
		return nil, domain.ValidationError("product_id 或 product_code 不能为空")
	}
	product, err := s.store.GetProduct(ctx, id)
	if err != nil && errors.Is(err, domain.ErrNotFound) {
		product, err = s.store.GetProductByCode(ctx, id)
	}
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该商品")
		}
		return nil, domain.AsAppError(err)
	}
	skus, err := s.store.ListSKUs(ctx, product.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	product.SKUs = skus
	return product, nil
}

// Knowledge performs keyword search over published policy articles. Knowledge is
// for policy explanation only and never substitutes live business data.
type Knowledge struct{ core }

// ArticleHit is a trimmed knowledge result suitable for the model context.
type ArticleHit struct {
	ArticleCode string   `json:"article_code"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Snippet     string   `json:"snippet"`
	Keywords    []string `json:"keywords,omitempty"`
	Source      string   `json:"source,omitempty"`
	UpdatedAt   string   `json:"updated_at"`
}

// Search returns published articles matching the query.
func (s *Knowledge) Search(ctx context.Context, query, category string, limit int) ([]ArticleHit, error) {
	articles, err := s.store.SearchKnowledge(ctx, query, category, limit)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	hits := make([]ArticleHit, 0, len(articles))
	for _, a := range articles {
		hits = append(hits, ArticleHit{
			ArticleCode: a.ArticleCode,
			Category:    a.Category,
			Title:       a.Title,
			Snippet:     snippet(a.Content, 400),
			Keywords:    a.Keywords,
			Source:      a.Source,
			UpdatedAt:   a.UpdatedAt.Format("2006-01-02"),
		})
	}
	return hits, nil
}

// snippet truncates content on a rune boundary.
func snippet(content string, maxRunes int) string {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "…"
}
