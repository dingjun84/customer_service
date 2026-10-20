package service

import (
	"context"
	"errors"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
)

// knownOrderStatuses guards the status filter against arbitrary SQL-ish input.
var knownOrderStatuses = map[string]bool{
	domain.OrderPendingPayment: true,
	domain.OrderPaid:           true,
	domain.OrderProcessing:     true,
	domain.OrderShipped:        true,
	domain.OrderCompleted:      true,
	domain.OrderCancelled:      true,
	domain.OrderClosed:         true,
}

// Orders serves customer-scoped order reads.
type Orders struct{ core }

// List returns the current customer's orders, newest first. The cursor enables
// keyset pagination without OFFSET scans.
func (s *Orders) List(ctx context.Context, customerID, status string, limit int, cursor *repository.Cursor) ([]domain.Order, error) {
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	status = strings.TrimSpace(status)
	if status != "" && !knownOrderStatuses[status] {
		return nil, domain.ValidationError("order status 取值不合法")
	}
	orders, err := s.store.ListOrders(ctx, repository.OrderQuery{
		CustomerID: customerID, Status: status, Limit: limit, Cursor: cursor,
	})
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return orders, nil
}

// Detail returns one order with its item snapshots.
func (s *Orders) Detail(ctx context.Context, customerID, orderNo string) (*domain.Order, error) {
	order, err := s.ownedOrder(ctx, customerID, orderNo)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListOrderItems(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	order.Items = items
	return order, nil
}

// Logistics serves parcel and tracking-node reads.
type Logistics struct{ core }

// ForOrder returns the parcels of an order with their tracking events.
func (s *Logistics) ForOrder(ctx context.Context, customerID, orderNo string) ([]domain.Shipment, error) {
	order, err := s.ownedOrder(ctx, customerID, orderNo)
	if err != nil {
		return nil, err
	}
	shipments, err := s.store.ListShipments(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	for i := range shipments {
		events, err := s.store.ListShipmentEvents(ctx, shipments[i].ID)
		if err != nil {
			return nil, domain.AsAppError(err)
		}
		shipments[i].Events = events
	}
	return shipments, nil
}

// ByTrackingNo resolves a parcel by carrier tracking number after verifying that
// the parcel's order belongs to the current customer.
func (s *Logistics) ByTrackingNo(ctx context.Context, customerID, trackingNo string) (*domain.Shipment, error) {
	trackingNo = strings.TrimSpace(trackingNo)
	if trackingNo == "" {
		return nil, domain.ValidationError("tracking_no 不能为空")
	}
	shipment, err := s.store.GetShipmentByTrackingNo(ctx, trackingNo)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该运单")
		}
		return nil, domain.AsAppError(err)
	}
	order, err := s.ownedOrderByID(ctx, customerID, shipment.OrderID)
	if err != nil {
		return nil, err
	}
	_ = order
	events, err := s.store.ListShipmentEvents(ctx, shipment.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	shipment.Events = events
	return shipment, nil
}
