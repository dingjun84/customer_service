package sqlite

import (
	"context"

	"github.com/example/ecommerce-support/internal/domain"
)

const shipmentColumns = `id, shipment_no, order_id, COALESCE(carrier_code,''), COALESCE(carrier_name,''),
	COALESCE(tracking_no,''), shipment_status, COALESCE(estimated_delivery_at,''), COALESCE(last_event_at,''),
	created_at, updated_at`

func scanShipment(scan func(dest ...any) error) (*domain.Shipment, error) {
	var sh domain.Shipment
	var created, updated, estimated, lastEvent string
	if err := scan(&sh.ID, &sh.ShipmentNo, &sh.OrderID, &sh.CarrierCode, &sh.CarrierName,
		&sh.TrackingNo, &sh.ShipmentStatus, &estimated, &lastEvent, &created, &updated); err != nil {
		return nil, translateErr(err)
	}
	sh.EstimatedDeliveryAt = parseTSPtr(estimated)
	sh.LastEventAt = parseTSPtr(lastEvent)
	sh.CreatedAt = parseTS(created)
	sh.UpdatedAt = parseTS(updated)
	return &sh, nil
}

// ListShipments returns all parcels of an order.
func (s *Store) ListShipments(ctx context.Context, orderID string) ([]domain.Shipment, error) {
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+shipmentColumns+` FROM shipments WHERE order_id = ? ORDER BY created_at, id`, orderID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.Shipment{}
	for rows.Next() {
		sh, err := scanShipment(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *sh)
	}
	return out, translateErr(rows.Err())
}

// ListShipmentEvents returns tracking nodes of one parcel, newest first.
func (s *Store) ListShipmentEvents(ctx context.Context, shipmentID string) ([]domain.ShipmentEvent, error) {
	rows, err := s.conn().QueryContext(ctx, `
		SELECT id, shipment_id, event_time, COALESCE(event_code,''), description, COALESCE(location,''), source, created_at
		  FROM shipment_events WHERE shipment_id = ? ORDER BY event_time DESC, id DESC`, shipmentID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.ShipmentEvent{}
	for rows.Next() {
		var e domain.ShipmentEvent
		var eventTime, created string
		if err := rows.Scan(&e.ID, &e.ShipmentID, &eventTime, &e.EventCode, &e.Description, &e.Location, &e.Source, &created); err != nil {
			return nil, translateErr(err)
		}
		e.EventTime = parseTS(eventTime)
		e.CreatedAt = parseTS(created)
		out = append(out, e)
	}
	return out, translateErr(rows.Err())
}

// GetShipmentByTrackingNo looks a parcel up by carrier tracking number.
func (s *Store) GetShipmentByTrackingNo(ctx context.Context, trackingNo string) (*domain.Shipment, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+shipmentColumns+` FROM shipments WHERE tracking_no = ?`, trackingNo)
	return scanShipment(row.Scan)
}
