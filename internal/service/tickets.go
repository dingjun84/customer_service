package service

import (
	"context"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
)

// Tickets creates human-transfer, complaint and quality tickets. These are the
// only writes that execute immediately: the customer explicitly asked for them.
type Tickets struct{ core }

// TicketRequest describes one ticket to create.
type TicketRequest struct {
	CustomerID     string
	ConversationID string
	OrderNo        string
	TicketType     string
	Priority       string
	Summary        string
	Details        string
}

// Create writes a ticket and, for human transfers, marks the conversation as transferred.
func (s *Tickets) Create(ctx context.Context, req TicketRequest) (*domain.ServiceTicket, error) {
	customerID := req.CustomerID
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	ticketType := strings.TrimSpace(req.TicketType)
	switch ticketType {
	case domain.TicketHumanTransfer, domain.TicketComplaint, domain.TicketQualityIssue, domain.TicketOther:
	default:
		return nil, domain.ValidationError("ticket_type 取值不合法")
	}
	priority := strings.TrimSpace(req.Priority)
	if priority == "" {
		priority = domain.PriorityNormal
	}
	switch priority {
	case domain.PriorityLow, domain.PriorityNormal, domain.PriorityHigh, domain.PriorityUrgent:
	default:
		return nil, domain.ValidationError("priority 取值不合法")
	}
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		return nil, domain.ValidationError("summary 不能为空")
	}
	if len([]rune(summary)) > 200 {
		return nil, domain.ValidationError("summary 不超过 200 个字符")
	}
	if len([]rune(req.Details)) > 2000 {
		return nil, domain.ValidationError("details 不超过 2000 个字符")
	}

	orderID := ""
	if strings.TrimSpace(req.OrderNo) != "" {
		order, err := s.ownedOrder(ctx, customerID, req.OrderNo)
		if err != nil {
			return nil, err
		}
		orderID = order.ID
	}

	now := s.nowUTC()
	ticket := &domain.ServiceTicket{
		ID:             domain.NewID(),
		TicketNo:       domain.HumanNo("TKT", now),
		CustomerID:     customerID,
		ConversationID: req.ConversationID,
		OrderID:        orderID,
		TicketType:     ticketType,
		Status:         "open",
		Priority:       priority,
		Summary:        summary,
		Details:        req.Details,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.store.CreateTicket(ctx, ticket); err != nil {
		return nil, domain.AsAppError(err)
	}

	if ticketType == domain.TicketHumanTransfer && req.ConversationID != "" {
		if err := s.store.SetConversationStatus(ctx, req.ConversationID, domain.ConversationTransferred, now); err != nil {
			return nil, domain.AsAppError(err)
		}
	}

	s.audit(ctx, &domain.ActionAuditLog{
		CustomerID: customerID, ConversationID: req.ConversationID,
		ActionType: ticketType, TargetType: "service_ticket", TargetID: ticket.ID,
		ActorType: domain.ActorAssistant, ActorID: "assistant",
		Result: "succeeded", Reason: summary,
	})
	return ticket, nil
}
