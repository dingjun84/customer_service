package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// KnowledgeFilter narrows the article list.
type KnowledgeFilter struct {
	Query    string
	Category string
	Status   string
	Page     Page
}

// Knowledge pages articles. Content stays out of the list response; a 20 KB body
// per row would dominate the payload for no benefit.
func (s *Store) Knowledge(ctx context.Context, f KnowledgeFilter) (List[KnowledgeRow], error) {
	w := &where{}
	if f.Category != "" {
		w.add("category = ?", f.Category)
	}
	if f.Status != "" {
		w.add("status = ?", f.Status)
	}
	w.like([]string{"title", "article_code", "COALESCE(source, '')", "keywords_json", "content"}, f.Query)

	countQ := `SELECT COUNT(*) FROM knowledge_articles` + w.clause()
	listQ := `SELECT id, article_code, category, title, status, COALESCE(source, ''), version,
		keywords_json, substr(content, 1, 160), created_at, updated_at
		FROM knowledge_articles` + w.clause() + ` ORDER BY updated_at DESC`

	return listPage(ctx, s.db, countQ, listQ, w.args, f.Page, func(r rowScanner) (KnowledgeRow, error) {
		var k KnowledgeRow
		err := r.Scan(&k.ID, &k.ArticleCode, &k.Category, &k.Title, &k.Status, &k.Source,
			&k.Version, &k.Keywords, &k.Excerpt, &k.CreatedAt, &k.UpdatedAt)
		k.Excerpt = collapse(k.Excerpt)
		return k, err
	})
}

// KnowledgeDetail loads one article body.
func (s *Store) KnowledgeDetail(ctx context.Context, id string) (KnowledgeDetail, error) {
	var k KnowledgeDetail
	err := s.db.QueryRowContext(ctx, `SELECT id, article_code, category, title, status,
		COALESCE(source, ''), version, keywords_json, substr(content, 1, 160),
		created_at, updated_at, content
		FROM knowledge_articles WHERE id = ? OR article_code = ?`, id, id).
		Scan(&k.ID, &k.ArticleCode, &k.Category, &k.Title, &k.Status, &k.Source, &k.Version,
			&k.Keywords, &k.Excerpt, &k.CreatedAt, &k.UpdatedAt, &k.Content)
	if err == sql.ErrNoRows {
		return KnowledgeDetail{}, errNotFound("知识库文章不存在")
	}
	if err != nil {
		return KnowledgeDetail{}, fmt.Errorf("load article: %w", err)
	}
	k.Excerpt = collapse(k.Excerpt)
	return k, nil
}

// KnowledgeCategories lists distinct categories with counts, so the filter bar is
// built from real data instead of a hardcoded list.
func (s *Store) KnowledgeCategories(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT category, COUNT(*) FROM knowledge_articles
		GROUP BY category ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("knowledge categories: %w", err)
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"category": name, "count": n})
	}
	return out, rows.Err()
}

// CreateKnowledge inserts a draft or published article.
func (s *Store) CreateKnowledge(ctx context.Context, in KnowledgeInput, adminID, adminName string) (KnowledgeDetail, error) {
	norm, err := normaliseKnowledge(in)
	if err != nil {
		return KnowledgeDetail{}, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	id := newID()
	code := domain.HumanNo("KB", now)
	keywords, err := json.Marshal(norm.Keywords)
	if err != nil {
		return KnowledgeDetail{}, fmt.Errorf("marshal keywords: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return KnowledgeDetail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_articles
		(id, article_code, category, title, content, keywords_json, status, source, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		id, code, norm.Category, norm.Title, norm.Content, string(keywords), norm.Status,
		nullStr2(norm.Source), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return KnowledgeDetail{}, fmt.Errorf("insert article: %w", err)
	}
	if err := appendAuditTx(ctx, tx, auditEntry{
		ActionType: "knowledge_create", TargetType: "knowledge_article", TargetID: id,
		ActorType: "admin", ActorID: adminID,
		Before: nil, After: map[string]any{"title": norm.Title, "status": norm.Status, "code": code, "operator": adminName},
	}); err != nil {
		return KnowledgeDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return KnowledgeDetail{}, fmt.Errorf("commit article: %w", err)
	}
	return s.KnowledgeDetail(ctx, id)
}

// UpdateKnowledge rewrites an article and bumps its version.
func (s *Store) UpdateKnowledge(ctx context.Context, id string, in KnowledgeInput, adminID, adminName string) (KnowledgeDetail, error) {
	norm, err := normaliseKnowledge(in)
	if err != nil {
		return KnowledgeDetail{}, err
	}
	current, err := s.KnowledgeDetail(ctx, id)
	if err != nil {
		return KnowledgeDetail{}, err
	}
	keywords, err := json.Marshal(norm.Keywords)
	if err != nil {
		return KnowledgeDetail{}, fmt.Errorf("marshal keywords: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return KnowledgeDetail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_articles
		SET category = ?, title = ?, content = ?, keywords_json = ?, status = ?, source = ?,
		    version = version + 1, updated_at = ?
		WHERE id = ?`,
		norm.Category, norm.Title, norm.Content, string(keywords), norm.Status,
		nullStr2(norm.Source), time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), current.ID); err != nil {
		return KnowledgeDetail{}, fmt.Errorf("update article: %w", err)
	}
	if err := appendAuditTx(ctx, tx, auditEntry{
		ActionType: "knowledge_update", TargetType: "knowledge_article", TargetID: current.ID,
		ActorType: "admin", ActorID: adminID,
		Before: map[string]any{"title": current.Title, "status": current.Status, "version": current.Version},
		After:  map[string]any{"title": norm.Title, "status": norm.Status, "version": current.Version + 1, "operator": adminName},
	}); err != nil {
		return KnowledgeDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return KnowledgeDetail{}, fmt.Errorf("commit article: %w", err)
	}
	return s.KnowledgeDetail(ctx, current.ID)
}

// SetKnowledgeStatus publishes, archives or reverts an article.
func (s *Store) SetKnowledgeStatus(ctx context.Context, id, status, adminID, adminName string) (KnowledgeDetail, error) {
	switch status {
	case "draft", "published", "archived":
	default:
		return KnowledgeDetail{}, errValidation("status 只支持 draft/published/archived")
	}
	current, err := s.KnowledgeDetail(ctx, id)
	if err != nil {
		return KnowledgeDetail{}, err
	}
	if current.Status == status {
		return current, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return KnowledgeDetail{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE knowledge_articles SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), current.ID); err != nil {
		return KnowledgeDetail{}, fmt.Errorf("set article status: %w", err)
	}
	if err := appendAuditTx(ctx, tx, auditEntry{
		ActionType: "knowledge_status", TargetType: "knowledge_article", TargetID: current.ID,
		ActorType: "admin", ActorID: adminID,
		Before: map[string]any{"status": current.Status},
		After:  map[string]any{"status": status, "operator": adminName},
	}); err != nil {
		return KnowledgeDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return KnowledgeDetail{}, fmt.Errorf("commit status: %w", err)
	}
	return s.KnowledgeDetail(ctx, current.ID)
}

// normaliseKnowledge validates and trims the input.
func normaliseKnowledge(in KnowledgeInput) (KnowledgeInput, error) {
	out := KnowledgeInput{
		Category: strings.TrimSpace(in.Category),
		Title:    strings.TrimSpace(in.Title),
		Content:  strings.TrimSpace(in.Content),
		Source:   strings.TrimSpace(in.Source),
		Status:   strings.TrimSpace(in.Status),
	}
	if out.Category == "" {
		return out, errValidation("分类不能为空")
	}
	if len([]rune(out.Category)) > 40 {
		return out, errValidation("分类不超过 40 个字符")
	}
	if out.Title == "" {
		return out, errValidation("标题不能为空")
	}
	if len([]rune(out.Title)) > 200 {
		return out, errValidation("标题不超过 200 个字符")
	}
	if out.Content == "" {
		return out, errValidation("正文不能为空")
	}
	if len([]rune(out.Content)) > 20000 {
		return out, errValidation("正文不超过 20000 个字符")
	}
	if out.Status == "" {
		out.Status = "draft"
	}
	switch out.Status {
	case "draft", "published", "archived":
	default:
		return out, errValidation("status 只支持 draft/published/archived")
	}
	if len(in.Keywords) > 20 {
		return out, errValidation("关键词最多 20 个")
	}
	seen := map[string]bool{}
	out.Keywords = []string{}
	for _, k := range in.Keywords {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		if len([]rune(k)) > 50 {
			return out, errValidation("单个关键词不超过 50 个字符")
		}
		seen[k] = true
		out.Keywords = append(out.Keywords, k)
	}
	return out, nil
}

// collapse flattens whitespace so an excerpt renders on one line in a table.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// nullStr2 trims then converts to NullString.
func nullStr2(s string) sql.NullString { return nullable(s) }
