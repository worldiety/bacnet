package apdu

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/worldiety/bacnet/common/errors"
	"github.com/worldiety/bacnet/common/log"
	"github.com/worldiety/bacnet/common/netprim"
)

// DiscoverRequest configures a Who-Is discovery window.
type DiscoverRequest struct {
	Destination netprim.Address
	WhoIs       WhoIsRequest
	Window      time.Duration
}

// Discover sends Who-Is and collects I-Am indications until the window elapses
// or ctx is cancelled. Results are deduplicated by (device identifier, source).
func (c *clientImpl) Discover(ctx context.Context, req DiscoverRequest) ([]IAmIndication, error) {
	if req.Window <= 0 {
		return nil, errors.NewValidationError("window", req.Window, ErrInvalidASEConfig)
	}
	if c.iam == nil {
		return nil, errors.NewValidationError("i-am dispatcher", nil, ErrInvalidASEConfig)
	}

	subID, ch, err := c.iam.Subscribe()
	if err != nil {
		return nil, err
	}
	defer c.iam.Unsubscribe(subID)

	if err := c.WhoIs(ctx, req.Destination, req.WhoIs); err != nil {
		return nil, err
	}

	timer := time.NewTimer(req.Window)
	defer timer.Stop()

	seen := make(map[string]struct{})
	out := make([]IAmIndication, 0)

	for {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-timer.C:
			return out, nil
		case indication, ok := <-ch:
			if !ok {
				return out, nil
			}
			key := discoverDedupKey(indication)
			if _, exists := seen[key]; exists {
				log.Logger.Debug(
					"apdu discover duplicate i-am dropped",
					"device_identifier", indication.DeviceIdentifier,
					"src_network", indication.Source.Network,
					"src_mac_length", len(indication.Source.AddrPortBytes()),
				)

				continue
			}
			seen[key] = struct{}{}
			log.Logger.Debug(
				"apdu discover i-am accepted",
				"device_identifier", indication.DeviceIdentifier,
				"src_network", indication.Source.Network,
				"src_mac_length", len(indication.Source.AddrPortBytes()),
			)

			out = append(out, indication)
		}
	}
}

func discoverDedupKey(indication IAmIndication) string {
	return fmt.Sprintf("%d|%d|%x", indication.DeviceIdentifier, indication.Source.Network, indication.Source.AddrPort)
}

type iAmDispatcher struct {
	ue UserElement

	mu          sync.RWMutex
	registered  bool
	userHandler IAmHandler
	nextSubID   int
	subscribers map[int]chan IAmIndication
}

func newIAmDispatcher(ue UserElement) *iAmDispatcher {
	return &iAmDispatcher{
		ue:          ue,
		subscribers: make(map[int]chan IAmIndication),
	}
}

func (d *iAmDispatcher) RegisterHandler(handler IAmHandler) error {
	if handler == nil {
		return errors.NewValidationError("handler", nil, ErrHandlerNotFound)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.userHandler != nil {
		return ErrHandlerAlreadyRegistered
	}

	if err := d.ensureRegisteredLocked(); err != nil {
		return err
	}

	d.userHandler = handler
	return nil
}

func (d *iAmDispatcher) Subscribe() (int, <-chan IAmIndication, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.ensureRegisteredLocked(); err != nil {
		return 0, nil, err
	}

	d.nextSubID++
	id := d.nextSubID
	ch := make(chan IAmIndication, 16)
	d.subscribers[id] = ch
	return id, ch, nil
}

func (d *iAmDispatcher) Unsubscribe(id int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	ch, ok := d.subscribers[id]
	if !ok {
		return
	}
	delete(d.subscribers, id)
	close(ch)
}

func (d *iAmDispatcher) ensureRegisteredLocked() error {
	if d.registered {
		return nil
	}

	err := d.ue.HandleUnconfirmed(ServiceChoiceIAm, d.onInbound)
	if err != nil {
		return err
	}

	d.registered = true
	return nil
}

func (d *iAmDispatcher) onInbound(ctx context.Context, indication UnconfirmedIndicationICI) error {
	log.Logger.Debug(
		"apdu i-am dispatcher inbound",
		"src_network", indication.Source.Network,
		"src_mac_length", len(indication.Source.AddrPortBytes()),
		"payload_bytes", len(indication.ServiceRequest.Payload),
	)

	decoded, err := decodeIAmPayload(indication.ServiceRequest.Payload)
	if err != nil {
		return err
	}
	log.Logger.Debug(
		"apdu i-am dispatcher decode success",
		"device_identifier", decoded.DeviceIdentifier,
		"vendor_id", decoded.VendorID,
		"max_apdu_length", decoded.MaxAPDULengthAccepted,
		"segmentation_supported", decoded.SegmentationSupported,
	)

	typed := IAmIndication{
		Source:                indication.Source,
		DeviceIdentifier:      decoded.DeviceIdentifier,
		MaxAPDULengthAccepted: decoded.MaxAPDULengthAccepted,
		SegmentationSupported: decoded.SegmentationSupported,
		VendorID:              decoded.VendorID,
	}

	d.mu.RLock()
	user := d.userHandler
	subs := make([]chan IAmIndication, 0, len(d.subscribers))
	for _, ch := range d.subscribers {
		subs = append(subs, ch)
	}
	d.mu.RUnlock()

	if user != nil {
		if err := user(ctx, typed); err != nil {
			return err
		}
	}

	for _, ch := range subs {
		select {
		case ch <- typed:
		default:
			log.Logger.Debug(
				"apdu i-am dispatcher subscriber drop",
				"device_identifier", typed.DeviceIdentifier,
				"src_network", typed.Source.Network,
				"src_mac_length", len(typed.Source.AddrPortBytes()),
			)
		}
	}

	return nil
}
