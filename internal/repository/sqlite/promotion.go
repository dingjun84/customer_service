package sqlite

import (
	"context"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
)

const promotionColumns = `id, promotion_code, title, COALESCE(description,''), promotion_type, status,
	COALESCE(starts_at,''), COALESCE(ends_at,''), rules_json, created_at, updated_at`

// ListPromotions returns active promotions, optionally filtered by code or free text.
func (s *Store) ListPromotions(ctx context.Context, query, promotionCode string, limit int) ([]domain.Promotion, error) {
	limit = limitOrDefault(limit, 10, 50)
	where := []string{"status = 'active'"}
	args := []any{}
	if code := strings.TrimSpace(promotionCode); code != "" {
		where = append(where, "promotion_code = ?")
		args = append(args, code)
	} else if q := strings.TrimSpace(query); q != "" {
		like := "%" + escapeLike(q) + "%"
		where = append(where, `(title LIKE ? ESCAPE '\' OR promotion_code LIKE ? ESCAPE '\' OR COALESCE(description,'') LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	args = append(args, limit)

	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+promotionColumns+` FROM promotions WHERE `+strings.Join(where, " AND ")+
			` ORDER BY COALESCE(starts_at,'') DESC LIMIT ?`, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.Promotion{}
	for rows.Next() {
		var p domain.Promotion
		var startsAt, endsAt, created, updated string
		if err := rows.Scan(&p.ID, &p.PromotionCode, &p.Title, &p.Description, &p.PromotionType, &p.Status,
			&startsAt, &endsAt, &p.RulesJSON, &created, &updated); err != nil {
			return nil, translateErr(err)
		}
		p.StartsAt = parseTSPtr(startsAt)
		p.EndsAt = parseTSPtr(endsAt)
		p.CreatedAt = parseTS(created)
		p.UpdatedAt = parseTS(updated)
		p.Rules = jsonObject(p.RulesJSON)
		out = append(out, p)
	}
	return out, translateErr(rows.Err())
}

const benefitColumns = `id, customer_id, benefit_type, COALESCE(benefit_code,''), status,
	amount_minor, points, COALESCE(expires_at,''), attributes_json, created_at, updated_at`

// ListBenefits returns the customer's coupons/points/insurance status.
func (s *Store) ListBenefits(ctx context.Context, customerID, benefitType string, limit int) ([]domain.CustomerBenefit, error) {
	limit = limitOrDefault(limit, 20, 100)
	where := []string{"customer_id = ?"}
	args := []any{customerID}
	if t := strings.TrimSpace(benefitType); t != "" {
		where = append(where, "benefit_type = ?")
		args = append(args, t)
	}
	args = append(args, limit)

	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+benefitColumns+` FROM customer_benefits WHERE `+strings.Join(where, " AND ")+
			` ORDER BY COALESCE(expires_at,'') ASC, created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.CustomerBenefit{}
	for rows.Next() {
		var b domain.CustomerBenefit
		var amount, points *int64
		var expiresAt, created, updated string
		if err := rows.Scan(&b.ID, &b.CustomerID, &b.BenefitType, &b.BenefitCode, &b.Status,
			&amount, &points, &expiresAt, &b.AttributesJSON, &created, &updated); err != nil {
			return nil, translateErr(err)
		}
		b.AmountMinor = amount
		b.Points = points
		b.ExpiresAt = parseTSPtr(expiresAt)
		b.CreatedAt = parseTS(created)
		b.UpdatedAt = parseTS(updated)
		b.Attributes = jsonObject(b.AttributesJSON)
		out = append(out, b)
	}
	return out, translateErr(rows.Err())
}

const knowledgeColumns = `id, article_code, category, title, content, keywords_json, status,
	COALESCE(source,''), version, created_at, updated_at`

// SearchKnowledge performs keyword matching over published policy articles.
// The MVP intentionally avoids embeddings/vector search.
func (s *Store) SearchKnowledge(ctx context.Context, query, category string, limit int) ([]domain.KnowledgeArticle, error) {
	limit = limitOrDefault(limit, 5, 20)
	where := []string{"status = 'published'"}
	args := []any{}
	if q := strings.TrimSpace(query); q != "" {
		like := "%" + escapeLike(q) + "%"
		where = append(where, `(title LIKE ? ESCAPE '\' OR content LIKE ? ESCAPE '\' OR keywords_json LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	if c := strings.TrimSpace(category); c != "" {
		where = append(where, "category = ?")
		args = append(args, c)
	}
	args = append(args, limit)

	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+knowledgeColumns+` FROM knowledge_articles WHERE `+strings.Join(where, " AND ")+
			` ORDER BY updated_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.KnowledgeArticle{}
	for rows.Next() {
		var a domain.KnowledgeArticle
		var created, updated string
		if err := rows.Scan(&a.ID, &a.ArticleCode, &a.Category, &a.Title, &a.Content, &a.KeywordsJSON, &a.Status,
			&a.Source, &a.Version, &created, &updated); err != nil {
			return nil, translateErr(err)
		}
		a.CreatedAt = parseTS(created)
		a.UpdatedAt = parseTS(updated)
		a.Keywords = jsonList(a.KeywordsJSON)
		out = append(out, a)
	}
	return out, translateErr(rows.Err())
}
