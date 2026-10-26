// Package seed builds the fictional, fully synthetic dataset used for local
// demos, evaluation and tool tests.
//
// Design rules that matter for correctness:
//   - one fixed data anchor (2026-10-20T04:00:00Z) and a fixed RNG seed, so two
//     runs produce byte-identical rows;
//   - every business story is expressed as a sequence of business actions
//     (pay, ship, deliver, cancel, open case, refund, invoice) instead of
//     independently randomised status columns;
//   - the whole dataset is built in memory and validated before a single
//     transaction writes it: a violated invariant aborts the import.
package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"
)

// DataAnchor is the instant the dataset describes. Everything after it would be
// a future timestamp and is rejected by the validator.
var DataAnchor = time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)

// Seed is the fixed pseudo-random seed required by the spec.
const Seed int64 = 20261020

// Summary reports what the generator wrote plus the validation outcome.
type Summary struct {
	Customers      int
	Products       int
	SKUs           int
	Orders         int
	Items          int
	Payments       int
	Refunds        int
	Shipments      int
	Events         int
	Cases          int
	Invoices       int
	Promotions     int
	Benefits       int
	Articles       int
	Conversations  int
	Messages       int
	ToolExecs      int
	Tickets        int
	PendingAction  int
	Audits         int
	Annotations    int
	Checks         int
	DatasetChecks  int
	DatabaseChecks int
	Violations     int
	Skipped        bool
}

// Seeder writes the dataset into an already migrated database.
type Seeder struct {
	db     *sql.DB
	log    *slog.Logger
	anchor time.Time
	gen    *gen
}

// New builds a Seeder. The anchor can be overridden with SEED_ANCHOR for local
// experiments, but the default keeps datasets reproducible.
func New(db *sql.DB, logger *slog.Logger) *Seeder {
	if logger == nil {
		logger = slog.Default()
	}
	anchor := DataAnchor
	if raw := strings.TrimSpace(os.Getenv("SEED_ANCHOR")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			anchor = parsed.UTC()
		}
	}
	return &Seeder{db: db, log: logger, anchor: anchor}
}

// Anchor exposes the data anchor (handy for tests and reports).
func (s *Seeder) Anchor() time.Time { return s.anchor }

// Run generates and imports the dataset. reset=true clears business tables
// first; without reset the generator refuses to mix with foreign rows.
func (s *Seeder) Run(ctx context.Context, reset bool) (Summary, error) {
	if reset {
		if err := s.reset(ctx); err != nil {
			return Summary{}, err
		}
	}
	present, err := s.datasetPresent(ctx)
	if err != nil {
		return Summary{}, err
	}
	if present && !reset {
		s.log.Info("dataset already present, skipping generation")
		sum, err := s.countSummary(ctx)
		sum.Skipped = true
		return sum, err
	}
	if !reset {
		if n, err := s.tableCount(ctx, "orders"); err != nil {
			return Summary{}, err
		} else if n > 0 {
			return Summary{}, fmt.Errorf("数据库已有其他来源的数据（orders=%d），请使用 --reset 重建", n)
		}
	}

	g := newGen(s.anchor)
	g.build()

	if errs := g.errs; len(errs) > 0 {
		sort.Strings(errs)
		return Summary{}, fmt.Errorf("生成阶段发现 %d 处不一致，已中止导入：\n%s", len(errs), strings.Join(limit(errs, 12), "\n"))
	}
	report := validateDataset(g.ds, s.anchor)
	if report.Violations > 0 {
		return Summary{}, fmt.Errorf("校验发现 %d 处不一致，已中止导入：\n%s", report.Violations, strings.Join(limit(report.Messages(), 12), "\n"))
	}

	if err := s.write(ctx, g.ds); err != nil {
		return Summary{}, err
	}
	dbReport, err := validateDatabase(ctx, s.db, s.anchor)
	if err != nil {
		return Summary{}, err
	}
	if dbReport.Violations > 0 {
		return Summary{}, fmt.Errorf("落库后校验发现 %d 处不一致：\n%s", dbReport.Violations, strings.Join(limit(dbReport.Messages(), 12), "\n"))
	}

	s.gen = g
	sum := summaryOf(g.ds)
	sum.DatasetChecks = report.Checks
	sum.DatabaseChecks = dbReport.Checks
	sum.Checks = report.Checks + dbReport.Checks
	return sum, nil
}

// ExportArtifacts writes manifest.json and the scenario JSONL next to the seed.
// They are evaluation metadata and are never imported into SQLite.
func (s *Seeder) ExportArtifacts(dir string) error {
	if s.gen == nil {
		return fmt.Errorf("no generated dataset in memory; run Run() first")
	}
	return writeArtifacts(dir, s.gen)
}

// BuildArtifacts 只生成评测元数据，不打开数据库。seed 命令在库已存在时会跳过
// 生成阶段（s.gen 为 nil），此时想刷新 test_scenarios.jsonl 就得靠它。
func BuildArtifacts(dir string) error {
	g := newGen(DataAnchor)
	g.build()
	if errs := g.errs; len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("生成阶段发现 %d 处不一致：\n%s", len(errs), strings.Join(limit(errs, 8), "\n"))
	}
	return writeArtifacts(dir, g)
}

func (s *Seeder) write(ctx context.Context, ds *dataset) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable deferred foreign keys: %w", err)
	}
	if err := insertAll(ctx, tx, ds.insertSpecs()); err != nil {
		return fmt.Errorf("import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import: %w", err)
	}
	return nil
}

func (s *Seeder) reset(ctx context.Context) error {
	tables := []string{
		"message_annotations", "tool_executions", "pending_actions", "action_audit_logs",
		"messages", "conversations", "idempotency_keys", "service_tickets",
		"refunds", "invoices", "service_cases", "shipment_events", "shipments",
		"payments", "order_items", "orders", "customer_benefits", "promotions",
		"knowledge_articles", "product_skus", "products", "customers",
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return fmt.Errorf("reset table %s: %w", t, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset: %w", err)
	}
	s.log.Info("demo data cleared")
	return nil
}

func (s *Seeder) datasetPresent(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM customers WHERE customer_no = 'CUST-1020'`).Scan(&n); err != nil {
		return false, fmt.Errorf("lookup marker: %w", err)
	}
	return n > 0, nil
}

func (s *Seeder) tableCount(ctx context.Context, table string) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+table).Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}

func (s *Seeder) countSummary(ctx context.Context) (Summary, error) {
	tables := map[string]*int{}
	sum := Summary{}
	fields := []struct {
		table string
		dst   *int
	}{
		{"customers", &sum.Customers}, {"products", &sum.Products}, {"product_skus", &sum.SKUs},
		{"orders", &sum.Orders}, {"order_items", &sum.Items}, {"payments", &sum.Payments},
		{"refunds", &sum.Refunds}, {"shipments", &sum.Shipments}, {"shipment_events", &sum.Events},
		{"service_cases", &sum.Cases}, {"invoices", &sum.Invoices}, {"promotions", &sum.Promotions},
		{"customer_benefits", &sum.Benefits}, {"knowledge_articles", &sum.Articles},
		{"conversations", &sum.Conversations}, {"messages", &sum.Messages}, {"tool_executions", &sum.ToolExecs},
		{"service_tickets", &sum.Tickets}, {"pending_actions", &sum.PendingAction},
		{"action_audit_logs", &sum.Audits}, {"message_annotations", &sum.Annotations},
	}
	for _, f := range fields {
		n, err := s.tableCount(ctx, f.table)
		if err != nil {
			return Summary{}, err
		}
		*f.dst = n
	}
	_ = tables
	return sum, nil
}

// ---- generator ----

type gen struct {
	anchor       time.Time
	ds           *dataset
	rng          *rand.Rand
	errs         []string
	customerDefs []customerDef
	customers    []*customerRow
	products     map[string]*productRow
	skus         map[string]*skuRow
	productByID  map[string]*productRow
	orderDrafts  []*orderDraft

	orderSeq, paySeq, refundSeq, caseSeq, invoiceSeq, shipSeq, trackSeq int
	ticketSeq, actionSeq, convSeq                                       int
}

func newGen(anchor time.Time) *gen {
	return &gen{
		anchor:      anchor,
		ds:          &dataset{},
		rng:         rand.New(rand.NewSource(Seed)),
		products:    map[string]*productRow{},
		skus:        map[string]*skuRow{},
		productByID: map[string]*productRow{},
	}
}

func (g *gen) fail(format string, args ...any) {
	g.errs = append(g.errs, fmt.Sprintf(format, args...))
}

func (g *gen) build() {
	g.buildCatalog()
	g.buildPromotions()
	g.buildArticles()
	g.buildCustomers()
	g.buildBenefits()
	g.buildOrders()
	g.buildConversations()
}

func (g *gen) buildCatalog() {
	created := g.anchor.AddDate(0, 0, -400)
	updated := g.anchor.AddDate(0, 0, -3)
	for _, def := range catalogDefs() {
		p := productRow{
			ID: stableID("product", def.code), Code: def.code, Name: def.name, Brand: def.brand,
			Category: def.category, Desc: def.desc, Status: "active", Attrs: def.attrs,
			CreatedAt: created, UpdatedAt: updated,
		}
		g.products[def.code] = &p
		g.productByID[p.ID] = &p
		g.ds.products = append(g.ds.products, p)
		for _, sd := range def.skus {
			status := "active"
			if sd.stock == 0 {
				status = "out_of_stock"
			}
			code := "SKU-" + def.code + sd.suffix
			s := skuRow{
				ID: stableID("sku", code), ProductID: p.ID, Code: code, Name: sd.name,
				Specs: sd.specs, ListPrice: sd.price, Stock: sd.stock, Status: status,
				CreatedAt: created, UpdatedAt: updated,
			}
			g.skus[code] = &s
			g.ds.skus = append(g.ds.skus, s)
		}
	}
}

func (g *gen) buildPromotions() {
	created := g.anchor.AddDate(0, 0, -120)
	for _, def := range promotionDefs() {
		starts := g.anchor.Add(time.Duration(def.StartDays * 24 * float64(time.Hour)))
		ends := g.anchor.Add(time.Duration(def.EndDays * 24 * float64(time.Hour)))
		g.ds.promotions = append(g.ds.promotions, promotionRow{
			ID: stableID("promotion", def.Code), Code: def.Code, Title: def.Title, Desc: def.Desc,
			Type: def.Type, Status: def.Status, StartsAt: timePtr(starts), EndsAt: timePtr(ends),
			Rules: def.Rules, CreatedAt: created, UpdatedAt: g.anchor.AddDate(0, 0, -1),
		})
	}
}

func (g *gen) buildArticles() {
	for _, def := range articleDefs() {
		created, updated := knowledgeTime(g.anchor, def)
		g.ds.articles = append(g.ds.articles, articleRow{
			ID: stableID("article", def.Code), Code: def.Code, Category: def.Category, Title: def.Title,
			Content: def.Content, Keywords: def.Keywords, Status: def.Status, Source: "synthetic",
			Version: def.Version, CreatedAt: created, UpdatedAt: updated,
		})
	}
}

func (g *gen) buildCustomers() {
	created := g.anchor.AddDate(0, -8, 0)
	g.customerDefs = customerDefs()
	for i, def := range g.customerDefs {
		row := customerRow{
			ID: stableID("customer", def.No), No: def.No, Name: def.Name,
			PhoneMasked: maskPhone(def.Phone), Status: "active", CreatedAt: created, UpdatedAt: created,
		}
		g.customers = append(g.customers, &row)
		g.ds.customers = append(g.ds.customers, row)
		_ = i
	}
}

func (g *gen) buildBenefits() {
	for i, cust := range g.customers {
		for j, b := range benefitDefs(i, cust.No) {
			row := benefitRow{
				ID: stableID("benefit", cust.No, fmt.Sprintf("%d", j)), CustomerID: cust.ID,
				Type: b.Type, Code: b.Code, Status: b.Status, Attrs: b.Attrs,
				CreatedAt: g.anchor.AddDate(0, 0, -30), UpdatedAt: g.anchor.AddDate(0, 0, -2),
			}
			if b.Amount != 0 {
				row.Amount = i64Ptr(b.Amount)
			}
			if b.Points != 0 {
				row.Points = i64Ptr(b.Points)
			}
			if b.Expires.Set {
				row.ExpiresAt = timePtr(g.anchor.AddDate(0, 0, b.Expires.Days))
			}
			g.ds.benefits = append(g.ds.benefits, row)
		}
	}
}

// ---- numbering ----

func (g *gen) nextOrderNo() string {
	g.orderSeq++
	return fmt.Sprintf("ORD-%05d", 10000+g.orderSeq)
}

func (g *gen) nextPaymentNo() string {
	g.paySeq++
	return fmt.Sprintf("PAY-%05d", 60000+g.paySeq)
}

func (g *gen) nextRefundNo() string {
	g.refundSeq++
	return fmt.Sprintf("RF-%05d", 50000+g.refundSeq)
}

func (g *gen) nextCaseNo() string {
	g.caseSeq++
	return fmt.Sprintf("CASE-%05d", 50000+g.caseSeq)
}

func (g *gen) nextInvoiceNo() string {
	g.invoiceSeq++
	return fmt.Sprintf("INV-%05d", 90000+g.invoiceSeq)
}

func (g *gen) nextShipmentNo(prefix string) string {
	g.shipSeq++
	return fmt.Sprintf("%s%05d", prefix, 10000+g.shipSeq)
}

func (g *gen) nextTrackingNo(carrierCode string) string {
	g.trackSeq++
	short := strings.TrimSuffix(carrierCode, "_MOCK")
	if short == "" {
		short = "XX"
	}
	return fmt.Sprintf("MOCK%s%010d", short, 10000+g.trackSeq)
}

func (g *gen) nextTicketNo() string {
	g.ticketSeq++
	return fmt.Sprintf("TKT-%05d", 70000+g.ticketSeq)
}

func (g *gen) nextActionNo(at time.Time) string {
	g.actionSeq++
	return fmt.Sprintf("ACT-%s-%04d", at.UTC().Format("20060102"), g.actionSeq)
}

func (g *gen) nextConversationNo() string {
	g.convSeq++
	return fmt.Sprintf("CONV-%05d", 80000+g.convSeq)
}

// ---- summary helpers ----

func summaryOf(ds *dataset) Summary {
	return Summary{
		Customers: len(ds.customers), Products: len(ds.products), SKUs: len(ds.skus),
		Orders: len(ds.orders), Items: len(ds.items), Payments: len(ds.payments),
		Refunds: len(ds.refunds), Shipments: len(ds.shipments), Events: len(ds.events),
		Cases: len(ds.cases), Invoices: len(ds.invoices), Promotions: len(ds.promotions),
		Benefits: len(ds.benefits), Articles: len(ds.articles), Conversations: len(ds.conversations),
		Messages: len(ds.messages), ToolExecs: len(ds.toolExecs), Tickets: len(ds.tickets),
		PendingAction: len(ds.pending), Audits: len(ds.audits), Annotations: len(ds.annotations),
	}
}

func limit(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	out := append([]string{}, items[:n]...)
	out = append(out, fmt.Sprintf("…… 另有 %d 条", len(items)-n))
	return out
}
