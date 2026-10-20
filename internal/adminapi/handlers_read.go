package adminapi

import (
	"net/http"

	"github.com/example/ecommerce-support/internal/admin"
)

// writeList writes a page as data + meta, matching the customer API's shape.
func writeList[T any](w http.ResponseWriter, l admin.List[T]) {
	totalPages := 0
	if l.PageSize > 0 {
		totalPages = (l.Total + l.PageSize - 1) / l.PageSize
	}
	writeData(w, l.Items, map[string]any{
		"total":       l.Total,
		"page":        l.Page,
		"page_size":   l.PageSize,
		"total_pages": totalPages,
	})
}

// ---- overview ----

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.svc.Overview(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, ov, nil)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	h, err := s.svc.Health(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, h, nil)
}

// ---- conversations ----

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.ListConversations(r.Context(), admin.ConversationFilter{
		Query: query(r, "q"), Status: query(r, "status"), Channel: query(r, "channel"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	detail, err := s.svc.ConversationDetail(r.Context(), r.PathValue("conversationID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

// ---- tickets ----

func (s *Server) handleTickets(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.ListTickets(r.Context(), admin.TicketFilter{
		Query: query(r, "q"), Status: query(r, "status"),
		Type: query(r, "type"), Priority: query(r, "priority"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleTicket(w http.ResponseWriter, r *http.Request) {
	detail, err := s.svc.TicketDetail(r.Context(), r.PathValue("ticketNo"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

// ---- pending actions ----

func (s *Server) handlePendingActions(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.PendingActions(r.Context(), admin.PendingFilter{
		Query: query(r, "q"), Status: query(r, "status"),
		ActionType: query(r, "action_type"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handlePendingAction(w http.ResponseWriter, r *http.Request) {
	action, err := s.svc.PendingActionByID(r.Context(), r.PathValue("actionID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, action, nil)
}

// ---- audit / tools / analytics ----

func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.AuditLogs(r.Context(), admin.AuditFilter{
		Query: query(r, "q"), Result: query(r, "result"), ActorType: query(r, "actor_type"),
		TargetType: query(r, "target_type"), TargetID: query(r, "target_id"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleToolExecutions(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.ToolCalls(r.Context(), admin.ToolCallFilter{
		ConversationID: query(r, "conversation_id"), ToolName: query(r, "tool_name"),
		Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleToolStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.svc.ToolStats(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, stats, nil)
}

func (s *Server) handleIntentStats(w http.ResponseWriter, r *http.Request) {
	intents, err := s.svc.IntentStats(r.Context(), 50)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, intents, nil)
}

// ---- knowledge ----

func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.Knowledge(r.Context(), admin.KnowledgeFilter{
		Query: query(r, "q"), Category: query(r, "category"), Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleKnowledgeCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.svc.KnowledgeCategories(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, cats, nil)
}

func (s *Server) handleKnowledgeDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.svc.KnowledgeDetail(r.Context(), r.PathValue("articleID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

// ---- business data ----

func (s *Server) handleCustomers(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.Customers(r.Context(), admin.CustomerFilter{
		Query: query(r, "q"), Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleCustomerDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.svc.CustomerDetail(r.Context(), r.PathValue("customerNo"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.Orders(r.Context(), admin.OrderFilter{
		Query: query(r, "q"), Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleOrderDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.svc.OrderDetail(r.Context(), r.PathValue("orderNo"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.Products(r.Context(), admin.ProductFilter{
		Query: query(r, "q"), Category: query(r, "category"), Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleProductDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := s.svc.ProductDetail(r.Context(), r.PathValue("productID"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

func (s *Server) handleServiceCases(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.ServiceCases(r.Context(), admin.CaseFilter{
		Query: query(r, "q"), Status: query(r, "status"), Type: query(r, "type"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleRefunds(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.Refunds(r.Context(), admin.RefundFilter{
		Query: query(r, "q"), Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handleInvoices(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.Invoices(r.Context(), admin.InvoiceFilter{
		Query: query(r, "q"), Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}

func (s *Server) handlePromotions(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	list, err := s.svc.Promotions(r.Context(), admin.PromotionFilter{
		Query: query(r, "q"), Status: query(r, "status"), Page: page,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeList(w, list)
}
