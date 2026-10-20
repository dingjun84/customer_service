package httpapi

import (
	"net/http"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

func parseRFC3339(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// ---- products ----

func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireIdentity(w, r); !ok {
		return
	}
	limit, err := parseLimit(r, 20, 50)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	q := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	products, err := s.svc.Catalog.Search(r.Context(), q, category, limit)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	views := make([]productView, 0, len(products))
	for i := range products {
		views = append(views, newProductView(&products[i]))
	}
	writeData(w, r, views, map[string]any{"limit": limit, "count": len(views)})
}

func (s *Server) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireIdentity(w, r); !ok {
		return
	}
	product, err := s.svc.Catalog.Detail(r.Context(), r.PathValue("productID"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, newProductView(product), nil)
}

// ---- orders ----

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r, 20, 100)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	orders, err := s.svc.Orders.List(r.Context(), id.CustomerID, r.URL.Query().Get("status"), limit, cursor)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	views := make([]orderView, 0, len(orders))
	for i := range orders {
		views = append(views, newOrderView(&orders[i]))
	}
	next := ""
	if len(orders) == limit && len(orders) > 0 {
		last := orders[len(orders)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	writeData(w, r, views, listMeta(limit, next))
}

func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	order, err := s.svc.Orders.Detail(r.Context(), id.CustomerID, r.PathValue("orderNo"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, newOrderView(order), nil)
}

func (s *Server) handleOrderShipments(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	shipments, err := s.svc.Logistics.ForOrder(r.Context(), id.CustomerID, r.PathValue("orderNo"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	views := make([]shipmentView, 0, len(shipments))
	for i := range shipments {
		views = append(views, newShipmentView(&shipments[i]))
	}
	writeData(w, r, views, map[string]any{"count": len(views)})
}

func (s *Server) handleOrderServiceCases(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	orderNo := r.PathValue("orderNo")
	cases, err := s.svc.AfterSales.ListByOrder(r.Context(), id.CustomerID, orderNo)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	views := make([]serviceCaseView, 0, len(cases))
	for i := range cases {
		view := newServiceCaseView(&cases[i])
		view.OrderNo = orderNo
		views = append(views, view)
	}
	writeData(w, r, views, map[string]any{"count": len(views)})
}

func (s *Server) handleGetServiceCase(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	cs, err := s.svc.AfterSales.Get(r.Context(), id.CustomerID, r.PathValue("caseNo"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, newServiceCaseView(cs), nil)
}

func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	orderNo := r.URL.Query().Get("order_no")
	if orderNo == "" {
		writeError(w, r, s.logger, domain.ValidationError("order_no 查询参数必填"))
		return
	}
	invoices, err := s.svc.Invoices.ListByOrder(r.Context(), id.CustomerID, orderNo)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	views := make([]invoiceView, 0, len(invoices))
	for _, inv := range invoices {
		views = append(views, invoiceView{
			InvoiceNo:   inv.InvoiceNo,
			Type:        inv.InvoiceType,
			Status:      inv.InvoiceStatus,
			Title:       inv.Title,
			AmountMinor: inv.AmountMinor,
			Amount:      domain.FormatAmount(inv.AmountMinor),
			RequestedAt: tstr(inv.RequestedAt),
			IssuedAt:    tptr(inv.IssuedAt),
			FileURL:     inv.FileURL,
		})
	}
	writeData(w, r, views, map[string]any{"count": len(views)})
}
