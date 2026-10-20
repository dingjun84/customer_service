package httpapi

import (
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// DTOs are explicit projections: no repository struct is serialized directly, so
// internal columns can never leak into the API by accident.

type amountView struct {
	ItemAmountMinor     int64  `json:"item_amount_minor"`
	DiscountAmountMinor int64  `json:"discount_amount_minor"`
	ShippingFeeMinor    int64  `json:"shipping_fee_minor"`
	PaidAmountMinor     int64  `json:"paid_amount_minor"`
	RefundedAmountMinor int64  `json:"refunded_amount_minor"`
	Currency            string `json:"currency"`
	PaidDisplay         string `json:"paid_display"`
	RefundedDisplay     string `json:"refunded_display"`
}

type receiverView struct {
	Name       string `json:"name,omitempty"`
	Phone      string `json:"phone_masked,omitempty"`
	Province   string `json:"province,omitempty"`
	City       string `json:"city,omitempty"`
	District   string `json:"district,omitempty"`
	Address    string `json:"address,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
}

type orderItemView struct {
	ID               string         `json:"id"`
	ProductID        string         `json:"product_id,omitempty"`
	SKUID            string         `json:"sku_id,omitempty"`
	ProductName      string         `json:"product_name"`
	SKUName          string         `json:"sku_name,omitempty"`
	SKUAttributes    map[string]any `json:"sku_attributes,omitempty"`
	Quantity         int64          `json:"quantity"`
	RefundedQuantity int64          `json:"refunded_quantity"`
	UnitPriceMinor   int64          `json:"unit_price_minor"`
	UnitPrice        string         `json:"unit_price_display"`
	PaidAmountMinor  int64          `json:"paid_amount_minor"`
	ItemStatus       string         `json:"item_status"`
}

type orderView struct {
	OrderNo              string          `json:"order_no"`
	Status               string          `json:"order_status"`
	Channel              string          `json:"order_channel"`
	CreatedAt            string          `json:"created_at"`
	PaidAt               *string         `json:"paid_at,omitempty"`
	ShippedAt            *string         `json:"shipped_at,omitempty"`
	DeliveredAt          *string         `json:"delivered_at,omitempty"`
	CancelledAt          *string         `json:"cancelled_at,omitempty"`
	Amounts              amountView      `json:"amounts"`
	Receiver             receiverView    `json:"receiver"`
	ShippingMethod       string          `json:"shipping_method"`
	RequestedDeliveryAt  string          `json:"requested_delivery_at,omitempty"`
	InvoiceTypeRequested string          `json:"invoice_type_requested,omitempty"`
	Remark               string          `json:"remark,omitempty"`
	IsPresale            bool            `json:"is_presale"`
	Items                []orderItemView `json:"items"`
}

func newOrderView(o *domain.Order) orderView {
	items := make([]orderItemView, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, orderItemView{
			ID:               it.ID,
			ProductID:        it.ProductID,
			SKUID:            it.SKUID,
			ProductName:      it.ProductNameSnapshot,
			SKUName:          it.SKUNameSnapshot,
			SKUAttributes:    it.SKUAttributes,
			Quantity:         it.Quantity,
			RefundedQuantity: it.RefundedQuantity,
			UnitPriceMinor:   it.UnitPriceMinor,
			UnitPrice:        domain.FormatAmount(it.UnitPriceMinor),
			PaidAmountMinor:  it.PaidAmountMinor,
			ItemStatus:       it.ItemStatus,
		})
	}
	return orderView{
		OrderNo:     o.OrderNo,
		Status:      o.OrderStatus,
		Channel:     o.OrderChannel,
		CreatedAt:   tstr(o.CreatedAt),
		PaidAt:      tptr(o.PaidAt),
		ShippedAt:   tptr(o.ShippedAt),
		DeliveredAt: tptr(o.DeliveredAt),
		CancelledAt: tptr(o.CancelledAt),
		Amounts: amountView{
			ItemAmountMinor:     o.ItemAmountMinor,
			DiscountAmountMinor: o.DiscountAmountMinor,
			ShippingFeeMinor:    o.ShippingFeeMinor,
			PaidAmountMinor:     o.PaidAmountMinor,
			RefundedAmountMinor: o.RefundedAmountMinor,
			Currency:            o.Currency,
			PaidDisplay:         domain.FormatAmount(o.PaidAmountMinor),
			RefundedDisplay:     domain.FormatAmount(o.RefundedAmountMinor),
		},
		Receiver: receiverView{
			Name:       o.ReceiverName,
			Phone:      o.ReceiverPhoneMasked,
			Province:   o.ShippingProvince,
			City:       o.ShippingCity,
			District:   o.ShippingDistrict,
			Address:    o.ShippingAddress,
			PostalCode: o.ShippingPostalCode,
		},
		ShippingMethod:       o.ShippingMethod,
		RequestedDeliveryAt:  o.RequestedDeliveryAt,
		InvoiceTypeRequested: o.InvoiceTypeRequested,
		Remark:               o.OrderRemark,
		IsPresale:            o.IsPresale,
		Items:                items,
	}
}

type skuView struct {
	ID             string         `json:"sku_id"`
	SKUCode        string         `json:"sku_code"`
	SKUName        string         `json:"sku_name"`
	Specifications map[string]any `json:"specifications,omitempty"`
	ListPriceMinor int64          `json:"list_price_minor"`
	ListPrice      string         `json:"list_price_display"`
	StockQuantity  int64          `json:"stock_quantity"`
	Status         string         `json:"status"`
}

type productView struct {
	ID          string         `json:"product_id"`
	ProductCode string         `json:"product_code"`
	Name        string         `json:"name"`
	Brand       string         `json:"brand,omitempty"`
	Category    string         `json:"category,omitempty"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	SKUs        []skuView      `json:"skus,omitempty"`
}

func newProductView(p *domain.Product) productView {
	skus := make([]skuView, 0, len(p.SKUs))
	for _, k := range p.SKUs {
		skus = append(skus, skuView{
			ID:             k.ID,
			SKUCode:        k.SKUCode,
			SKUName:        k.SKUName,
			Specifications: k.Specifications,
			ListPriceMinor: k.ListPriceMinor,
			ListPrice:      domain.FormatAmount(k.ListPriceMinor),
			StockQuantity:  k.StockQuantity,
			Status:         k.Status,
		})
	}
	return productView{
		ID:          p.ID,
		ProductCode: p.ProductCode,
		Name:        p.Name,
		Brand:       p.Brand,
		Category:    p.Category,
		Description: p.Description,
		Status:      p.Status,
		Attributes:  p.Attributes,
		SKUs:        skus,
	}
}

type shipmentEventView struct {
	Time        string `json:"time"`
	Description string `json:"description"`
	Location    string `json:"location,omitempty"`
	Source      string `json:"source"`
}

type shipmentView struct {
	ShipmentNo          string              `json:"shipment_no"`
	CarrierCode         string              `json:"carrier_code,omitempty"`
	CarrierName         string              `json:"carrier_name,omitempty"`
	TrackingNo          string              `json:"tracking_no,omitempty"`
	Status              string              `json:"status"`
	EstimatedDeliveryAt *string             `json:"estimated_delivery_at,omitempty"`
	LastEventAt         *string             `json:"last_event_at,omitempty"`
	Events              []shipmentEventView `json:"events"`
}

func newShipmentView(s *domain.Shipment) shipmentView {
	events := make([]shipmentEventView, 0, len(s.Events))
	for _, e := range s.Events {
		events = append(events, shipmentEventView{
			Time:        tstr(e.EventTime),
			Description: e.Description,
			Location:    e.Location,
			Source:      e.Source,
		})
	}
	return shipmentView{
		ShipmentNo:          s.ShipmentNo,
		CarrierCode:         s.CarrierCode,
		CarrierName:         s.CarrierName,
		TrackingNo:          s.TrackingNo,
		Status:              s.ShipmentStatus,
		EstimatedDeliveryAt: tptr(s.EstimatedDeliveryAt),
		LastEventAt:         tptr(s.LastEventAt),
		Events:              events,
	}
}

type serviceCaseView struct {
	CaseNo            string  `json:"case_no"`
	CaseType          string  `json:"case_type"`
	Status            string  `json:"case_status"`
	OrderNo           string  `json:"order_no,omitempty"`
	OrderItemID       string  `json:"order_item_id,omitempty"`
	RequestedQuantity int64   `json:"requested_quantity"`
	Reason            string  `json:"reason,omitempty"`
	Description       string  `json:"description,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	ClosedAt          *string `json:"closed_at,omitempty"`
}

func newServiceCaseView(c *domain.ServiceCase) serviceCaseView {
	return serviceCaseView{
		CaseNo:            c.CaseNo,
		CaseType:          c.CaseType,
		Status:            c.CaseStatus,
		OrderItemID:       c.OrderItemID,
		RequestedQuantity: c.RequestedQuantity,
		Reason:            c.ReasonText,
		Description:       c.Description,
		CreatedAt:         tstr(c.CreatedAt),
		UpdatedAt:         tstr(c.UpdatedAt),
		ClosedAt:          tptr(c.ClosedAt),
	}
}

type paymentView struct {
	PaymentNo     string  `json:"payment_no"`
	Method        string  `json:"payment_method"`
	Status        string  `json:"payment_status"`
	AmountMinor   int64   `json:"amount_minor"`
	Amount        string  `json:"amount_display"`
	RefundedMinor int64   `json:"refunded_amount_minor"`
	PaidAt        *string `json:"paid_at,omitempty"`
}

type refundView struct {
	RefundNo    string  `json:"refund_no"`
	Status      string  `json:"refund_status"`
	Channel     string  `json:"refund_channel,omitempty"`
	AmountMinor int64   `json:"amount_minor"`
	Amount      string  `json:"amount_display"`
	Reason      string  `json:"reason,omitempty"`
	RequestedAt string  `json:"requested_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

type invoiceView struct {
	InvoiceNo   string  `json:"invoice_no,omitempty"`
	Type        string  `json:"invoice_type"`
	Status      string  `json:"invoice_status"`
	Title       string  `json:"title"`
	AmountMinor int64   `json:"amount_minor"`
	Amount      string  `json:"amount_display"`
	RequestedAt string  `json:"requested_at"`
	IssuedAt    *string `json:"issued_at,omitempty"`
	FileURL     string  `json:"file_url,omitempty"`
}

type conversationView struct {
	ID             string              `json:"conversation_id"`
	ConversationNo string              `json:"conversation_no"`
	Status         string              `json:"status"`
	Channel        string              `json:"channel"`
	Summary        string              `json:"summary,omitempty"`
	ActiveTask     map[string]any      `json:"active_task,omitempty"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
	LastMessageAt  *string             `json:"last_message_at,omitempty"`
	PendingActions []pendingActionView `json:"pending_actions,omitempty"`
}

type pendingActionView struct {
	ActionID   string         `json:"action_id"`
	ActionNo   string         `json:"action_no"`
	ActionType string         `json:"action_type"`
	Status     string         `json:"status"`
	Summary    string         `json:"summary"`
	Preview    map[string]any `json:"preview"`
	ExpiresAt  string         `json:"expires_at"`
}

func tstr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func tptr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
