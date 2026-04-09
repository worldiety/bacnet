package apdu

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.wdy.de/bacnet"
)

// SegmentationSupport models local segmentation capability.
type SegmentationSupport byte

const (
	SegmentationNo SegmentationSupport = iota
	SegmentationTransmit
	SegmentationReceive
	SegmentationBoth
)

func (s SegmentationSupport) String() string {
	switch s {
	case SegmentationNo:
		return "no-segmentation"
	case SegmentationTransmit:
		return "segmented-transmit"
	case SegmentationReceive:
		return "segmented-receive"
	case SegmentationBoth:
		return "segmented-both"
	default:
		return fmt.Sprintf("segmentation-support(%d)", s)
	}
}

// ASEConfig controls runtime behavior for transaction tracking and dispatch.
type ASEConfig struct {
	InvokeTimeout        time.Duration
	MaxConcurrentInvokes int
	Segmentation         SegmentationSupport
	MaxAPDUSizeAccepted  uint16
}

// Codec translates raw APDU bytes into normalized envelope structs.
type Codec interface {
	Decode(raw []byte) (InboundAPDU, error)
	Encode(apdu OutboundAPDU) ([]byte, error)
}

// Transport sends encoded APDUs to a BACnet peer.
type Transport interface {
	SendAPDU(ctx context.Context, dst bacnet.Address, apdu []byte) error
}

// ConfirmedHandler processes confirmed requests and returns a response payload.
type ConfirmedHandler func(ctx context.Context, req ConfirmedRequest) (ServiceResult, error)

// UnconfirmedHandler processes unconfirmed requests.
type UnconfirmedHandler func(ctx context.Context, req UnconfirmedRequest) error

// ASE coordinates APDU dispatch and confirmed transaction lifecycle
type ASE interface {
	RegisterConfirmed(choice ServiceChoice, handler ConfirmedHandler) error
	RegisterUnconfirmed(choice ServiceChoice, handler UnconfirmedHandler) error
	InvokeConfirmed(ctx context.Context, dst bacnet.Address, req ConfirmedRequest) (ConfirmedAck, error)
	SendUnconfirmed(ctx context.Context, address bacnet.Address, req UnconfirmedRequest) error
	OnInbound(ctx context.Context, src bacnet.Address, raw []byte) error
	Close() error
}

// aseImpl implements ASE.
type aseImpl struct {
	cfg       ASEConfig
	codec     Codec
	transport Transport

	mu                  sync.Mutex
	closed              bool
	nextInvokeID        InvokeID
	transactions        map[InvokeID]transaction
	confirmedHandlers   map[ServiceChoice]ConfirmedHandler
	unconfirmedHandlers map[ServiceChoice]UnconfirmedHandler
}

type transaction struct {
	done chan transactionResult
}

type transactionResult struct {
	ack ConfirmedAck
	err error
}

// NewASE validates configuration and creates an aseImpl instance.
func NewASE(cfg ASEConfig, codec Codec, transport Transport) (ASE, error) {
	if codec == nil {
		return nil, ErrNilCodec
	}
	if transport == nil {
		return nil, ErrNilTransport
	}
	if cfg.InvokeTimeout <= 0 {
		return nil, bacnet.NewValidationError("invoke timeout", cfg.InvokeTimeout, ErrInvalidASEConfig)
	}
	if cfg.MaxConcurrentInvokes <= 0 {
		return nil, bacnet.NewValidationError("max concurrent invokes", cfg.MaxConcurrentInvokes, ErrInvalidASEConfig)
	}

	return &aseImpl{
		cfg:                 cfg,
		codec:               codec,
		transport:           transport,
		transactions:        make(map[InvokeID]transaction),
		confirmedHandlers:   make(map[ServiceChoice]ConfirmedHandler),
		unconfirmedHandlers: make(map[ServiceChoice]UnconfirmedHandler),
	}, nil
}

// Close stops the aseImpl and fails all pending transactions.
func (a *aseImpl) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	pending := a.transactions
	a.transactions = make(map[InvokeID]transaction)
	a.mu.Unlock()

	for _, tx := range pending {
		tx.done <- transactionResult{err: ErrASEClosed}
	}

	return nil
}

// RegisterConfirmed registers a confirmed request handler for a service choice.
func (a *aseImpl) RegisterConfirmed(choice ServiceChoice, handler ConfirmedHandler) error {
	if handler == nil {
		return bacnet.NewValidationError("handler", nil, ErrHandlerNotFound)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.confirmedHandlers[choice]; exists {
		return ErrHandlerAlreadyRegistered
	}
	a.confirmedHandlers[choice] = handler
	return nil
}

// RegisterUnconfirmed registers an unconfirmed request handler for a service choice.
func (a *aseImpl) RegisterUnconfirmed(choice ServiceChoice, handler UnconfirmedHandler) error {
	if handler == nil {
		return bacnet.NewValidationError("handler", nil, ErrHandlerNotFound)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.unconfirmedHandlers[choice]; exists {
		return ErrHandlerAlreadyRegistered
	}
	a.unconfirmedHandlers[choice] = handler
	return nil
}

// InvokeConfirmed sends a confirmed request and waits for a terminal response APDU.
func (a *aseImpl) InvokeConfirmed(ctx context.Context, dst bacnet.Address, req ConfirmedRequest) (ConfirmedAck, error) {
	txID, tx, err := a.startTransaction()
	if err != nil {
		return ConfirmedAck{}, err
	}

	raw, err := a.codec.Encode(OutboundAPDU{
		Type:          PDUTypeConfirmedRequest,
		InvokeID:      txID,
		ServiceChoice: req.ServiceChoice,
		Payload:       cloneBytes(req.Payload),
	})
	if err != nil {
		a.finishTransaction(txID)
		return ConfirmedAck{}, fmt.Errorf("%w: %v", ErrEncodeFailure, err)
	}

	if err := a.transport.SendAPDU(ctx, dst, raw); err != nil {
		a.finishTransaction(txID)
		return ConfirmedAck{}, fmt.Errorf("%w: %v", ErrTransportFailure, err)
	}

	timer := time.NewTimer(a.cfg.InvokeTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		a.finishTransaction(txID)
		return ConfirmedAck{}, ctx.Err()
	case <-timer.C:
		a.finishTransaction(txID)
		return ConfirmedAck{}, ErrAPDUTimeout
	case result := <-tx.done:
		if result.err != nil {
			return ConfirmedAck{}, result.err
		}
		result.ack.Payload = cloneBytes(result.ack.Payload)
		return result.ack, nil
	}
}

// SendUnconfirmed sends an unconfirmed request APDU without creating a transaction.
func (a *aseImpl) SendUnconfirmed(ctx context.Context, dst bacnet.Address, req UnconfirmedRequest) error {
	raw, err := a.codec.Encode(OutboundAPDU{
		Type:          PDUTypeUnconfirmedRequest,
		ServiceChoice: req.ServiceChoice,
		Payload:       cloneBytes(req.Payload),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEncodeFailure, err)
	}
	if err := a.transport.SendAPDU(ctx, dst, raw); err != nil {
		return fmt.Errorf("%w: %v", ErrTransportFailure, err)
	}
	return nil
}

// OnInbound decodes and dispatches an inbound APDU.
func (a *aseImpl) OnInbound(ctx context.Context, src bacnet.Address, raw []byte) error {
	decoded, err := a.codec.Decode(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDecodeFailure, err)
	}

	switch decoded.Type {
	case PDUTypeConfirmedRequest:
		return a.handleConfirmedRequest(ctx, src, decoded)
	case PDUTypeUnconfirmedRequest:
		return a.handleUnconfirmedRequest(ctx, decoded)
	case PDUTypeSimpleACK, PDUTypeComplexACK, PDUTypeError, PDUTypeReject, PDUTypeAbort:
		return a.completeTransaction(decoded)
	default:
		return bacnet.NewValidationError("pdu type", decoded.Type, ErrInvalidPDUType)
	}
}

func (a *aseImpl) startTransaction() (InvokeID, transaction, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return 0, transaction{}, ErrASEClosed
	}
	if len(a.transactions) >= a.cfg.MaxConcurrentInvokes {
		return 0, transaction{}, ErrNoInvokeIDAvailable
	}

	for range 256 {
		id := a.nextInvokeID
		a.nextInvokeID++
		if _, exists := a.transactions[id]; exists {
			continue
		}
		tx := transaction{done: make(chan transactionResult, 1)}
		a.transactions[id] = tx
		return id, tx, nil
	}

	return 0, transaction{}, ErrNoInvokeIDAvailable
}

func (a *aseImpl) finishTransaction(id InvokeID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.transactions, id)
}

func (a *aseImpl) handleConfirmedRequest(ctx context.Context, src bacnet.Address, in InboundAPDU) error {
	a.mu.Lock()
	handler, ok := a.confirmedHandlers[in.ServiceChoice]
	a.mu.Unlock()
	if !ok {
		return ErrHandlerNotFound
	}

	result, err := handler(ctx, ConfirmedRequest{
		ServiceChoice: in.ServiceChoice,
		Payload:       cloneBytes(in.Payload),
	})
	if err != nil {
		return err
	}

	responseType := PDUTypeSimpleACK
	if len(result.Payload) > 0 {
		responseType = PDUTypeComplexACK
	}

	raw, err := a.codec.Encode(OutboundAPDU{
		Type:          responseType,
		InvokeID:      in.InvokeID,
		ServiceChoice: in.ServiceChoice,
		Payload:       cloneBytes(result.Payload),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEncodeFailure, err)
	}

	if err := a.transport.SendAPDU(ctx, src, raw); err != nil {
		return fmt.Errorf("%w: %v", ErrTransportFailure, err)
	}
	return nil
}

func (a *aseImpl) handleUnconfirmedRequest(ctx context.Context, in InboundAPDU) error {
	a.mu.Lock()
	handler, ok := a.unconfirmedHandlers[in.ServiceChoice]
	a.mu.Unlock()
	if !ok {
		return ErrHandlerNotFound
	}
	return handler(ctx, UnconfirmedRequest{ServiceChoice: in.ServiceChoice, Payload: cloneBytes(in.Payload)})
}

func (a *aseImpl) completeTransaction(in InboundAPDU) error {
	a.mu.Lock()
	tx, ok := a.transactions[in.InvokeID]
	if ok {
		delete(a.transactions, in.InvokeID)
	}
	a.mu.Unlock()
	if !ok {
		return ErrTransactionNotFound
	}

	result := transactionResult{}
	switch in.Type {
	case PDUTypeSimpleACK, PDUTypeComplexACK:
		result.ack = ConfirmedAck{
			Type:          in.Type,
			InvokeID:      in.InvokeID,
			ServiceChoice: in.ServiceChoice,
			Payload:       cloneBytes(in.Payload),
		}
	case PDUTypeError:
		result.err = ErrRemoteError
	case PDUTypeReject:
		result.err = ErrRemoteReject
	case PDUTypeAbort:
		result.err = ErrRemoteAbort
	default:
		result.err = bacnet.NewValidationError("pdu type", in.Type, ErrInvalidPDUType)
	}

	tx.done <- result
	return nil
}
