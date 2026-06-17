package apdu

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	bacneterrors "go.wdy.de/bacnet/common/errors"
	"go.wdy.de/bacnet/common/log"
	"go.wdy.de/bacnet/common/netprim"
	"go.wdy.de/bacnet/npdu"
)

// SegmentationSupport models local segmentation capability.
type SegmentationSupport byte

const (
	SegmentationSupportNo SegmentationSupport = iota
	SegmentationSupportTransmit
	SegmentationSupportReceive
	SegmentationSupportBoth
)

func (s SegmentationSupport) String() string {
	switch s {
	case SegmentationSupportNo:
		return "no-segmentation"
	case SegmentationSupportTransmit:
		return "segmented-transmit"
	case SegmentationSupportReceive:
		return "segmented-receive"
	case SegmentationSupportBoth:
		return "segmented-transmit-receive"
	default:
		return fmt.Sprintf("segmentation-support(%d)", s)
	}
}

// ASEConfig controls runtime behavior for transaction tracking and dispatch.
type ASEConfig struct {
	InvokeTimeout                    time.Duration         // timeout for invocation of a service by a client
	SegmentedRequestTimeout          time.Duration         // time between segments for a segmented service
	APDURetries                      uint8                 // max retries to send an APDU
	MaxSegmentDuplicates             uint8                 // maximum duplicate segmented-request PDUs tolerated before aborting the receive transaction
	MaxConcurrentInvokes             int                   // max concurrent requests by the client
	Segmentation                     SegmentationSupport   // segmentation support. One of: SegmentationSupportNo, SegmentationSupportTransmit, SegmentationSupportReceive, SegmentationSupportBoth
	PreferredWindowSize              uint8                 // preferred window size, defaults to 1
	MaxAPDUSizeAccepted              MaxApduLengthAccepted // max APDU size
	SegmentedTimedOutCollectorPeriod time.Duration         // time between calls to the cleanup of timed out segmented requests to the server
	TimeoutGracePeriod               time.Duration         // time a timed out request is stored after expiring before it is deleted
}

// DefaultASEConfig returns an ASEConfig with sensible defaults suitable for
// most BACnet/IP client applications:
//
//   - InvokeTimeout: 10 s — generous enough for slow field devices, per clause 5.4.
//   - APDURetries: 3 — standard recommendation.
//   - MaxConcurrentInvokes: 16 — balances throughput against controller load.
//   - MaxAPDUSizeAccepted: 1476 bytes — maximum that fits in a single Ethernet NPDU.
//   - Segmentation: SegmentationSupportNo — client-side segmented send is not yet
//     implemented; receiving segmented responses is handled transparently by the ASE.
//
// All other fields are left at their zero values; NewASE applies its own
// documented defaults for those (e.g. PreferredWindowSize → 1).
func DefaultASEConfig() ASEConfig {
	return ASEConfig{
		InvokeTimeout:        10 * time.Second,
		APDURetries:          3,
		MaxConcurrentInvokes: 16,
		MaxAPDUSizeAccepted:  1476,
		Segmentation:         SegmentationSupportNo,
	}
}

// NPDUTransport sends NPDUs to a BACnet peer.
type NPDUTransport interface {
	SendNPDU(ctx context.Context, dst netprim.Address, packet npdu.NetworkLayerProtocolDataUnit) error
}

// ConfirmedHandler processes confirmed requests and returns a response ICI.
type ConfirmedHandler func(ctx context.Context, indication ConfirmedIndicationICI) (ConfirmedResponseICI, error)

// UnconfirmedHandler processes unconfirmed requests as an unconfirmed indication.
type UnconfirmedHandler func(ctx context.Context, indication UnconfirmedIndicationICI) error

// ASE coordinates APDU dispatch and confirmed transaction lifecycle
type ASE interface {
	RegisterConfirmed(choice ServiceChoice, handler ConfirmedHandler) error
	RegisterUnconfirmed(choice ServiceChoice, handler UnconfirmedHandler) error
	BeginConfirmedServiceRequest(ctx context.Context, req ConfirmedRequestICI) (ConfirmICI, error)
	SendUnconfirmed(ctx context.Context, req UnconfirmedRequestICI) error
	OnInboundNPDU(ctx context.Context, src netprim.Address, packet npdu.NetworkLayerProtocolDataUnit) error
	Close() error
}

// aseImpl implements ASE.
type aseImpl struct {
	cfg       ASEConfig
	transport NPDUTransport
	mu        sync.Mutex
	closed    bool
	// stopCh is closed by Close() to signal background goroutines to stop.
	stopCh chan struct{}

	aseServerImpl
	aseClientImpl
}

type aseServerImpl struct {
	confirmedHandlers              map[ServiceChoice]ConfirmedHandler
	unconfirmedHandlers            map[ServiceChoice]UnconfirmedHandler
	inboundSegmentedServerEntries  map[segmentedServerKey]*segmentedServerEntry
	outboundSegmentedServerEntries map[segmentedServerKey]*segmentedServerResponseEntry
	// inFlightServerRequests tracks the set of (src, invokeID) pairs whose confirmed-service
	// handler is currently running. Duplicate requests are silently dropped (§5.4.4).
	inFlightServerRequests map[segmentedServerKey]struct{}
}

type aseClientImpl struct {
	// nextInvokeID is the next invoke ID to assign to an outbound confirmed request.
	nextInvokeID       InvokeID
	clientTransactions map[InvokeID]transaction
}

// segmentedServerEntry tracks an in-progress segmented confirmed-request receive
// on the server path; unlike transaction it does not model a client-side invoke lifecycle.
type segmentedServerEntry struct {
	machine      *confirmedServerMachine
	priority     netprim.NetworkPriority
	firstInbound inboundAPDU // first segment; carries invokeID, serviceChoice, maxAPDU, etc.
	expiresAt    time.Time
}

// segmentedServerResponseEntry tracks an in-progress segmented confirmed-server
// response while waiting for Segment-ACK PDUs from the requester.
type segmentedServerResponseEntry struct {
	machine   *confirmedServerMachine
	priority  netprim.NetworkPriority
	src       netprim.Address
	expiresAt time.Time
}

type segmentedServerKey string

// segmentedServerKeyFor returns the map key for an in-progress segmented
// server transaction identified by its source address and invoke ID.
func segmentedServerKeyFor(src netprim.Address, id InvokeID) segmentedServerKey {
	return segmentedServerKey(fmt.Sprintf("%d:%x:%d", id, src.AddrPortBytes(), src.Network))
}

type transaction struct {
	done                  chan transactionResult
	stateMachine          protocolStateMachine
	expectedPeer          netprim.Address
	expectedServiceChoice ServiceChoice
}

type transactionResult struct {
	confirm ConfirmICI
	err     *TransactionError
}

type TransactionError struct {
	Err error
	// InboundApdu that caused the transaction error, may be nil
	InboundApdu *inboundAPDU

	//TODO maybe add more info as needed
}

func newTransactionError(err error, inboundApdu *inboundAPDU) *TransactionError {
	if err == nil {
		return nil
	}

	return &TransactionError{
		Err:         err,
		InboundApdu: inboundApdu,
	}
}

func (e *TransactionError) Unwrap() error {
	return e.Err
}

func (e *TransactionError) Error() string {
	return fmt.Sprintf("transaction error: %s\n", e.Err.Error())
}

func (e *TransactionError) String() string {
	s := fmt.Sprintf("transaction error: %s\n", e.Err)
	if e.InboundApdu != nil {
		s += fmt.Sprintf("inbound apdu: %v\n", e.InboundApdu)
	}

	return s
}

func inboundAPDUCarriesServiceContext(pduType PDUType) bool {
	switch pduType {
	case PDUTypeSimpleACK, PDUTypeComplexACK, PDUTypeError:
		return true
	default:
		return false
	}
}

func (t transaction) matchesInboundAPDU(src netprim.Address, in inboundAPDU) bool {
	if !t.expectedPeer.Equal(src) {
		return false
	}

	if inboundAPDUCarriesServiceContext(in.Type) && in.ServiceChoice != t.expectedServiceChoice {
		return false
	}

	return true
}

// NewASE validates configuration and creates an aseImpl instance.
func NewASE(cfg ASEConfig, transport NPDUTransport) (ASE, error) {
	if transport == nil {
		return nil, ErrNilTransport
	}

	if cfg.InvokeTimeout <= 0 {
		return nil, bacneterrors.NewValidationError("invoke timeout", cfg.InvokeTimeout, ErrInvalidASEConfig)
	}

	if cfg.MaxConcurrentInvokes <= 0 {
		return nil, bacneterrors.NewValidationError("max concurrent invokes", cfg.MaxConcurrentInvokes, ErrInvalidASEConfig)
	}

	if cfg.PreferredWindowSize > 127 {
		return nil, bacneterrors.NewValidationError("preferred window size", cfg.PreferredWindowSize, ErrInvalidASEConfig)
	}

	if cfg.SegmentedRequestTimeout < 0 {
		return nil, bacneterrors.NewValidationError("segmented request timeout", cfg.SegmentedRequestTimeout, ErrInvalidASEConfig)
	}

	// Default PreferredWindowSize to 1 (minimum value mandated by the standard).
	if cfg.PreferredWindowSize == 0 {
		cfg.PreferredWindowSize = 1
	}

	if cfg.MaxSegmentDuplicates == 0 {
		cfg.MaxSegmentDuplicates = uint8(defaultMaxDuplicateCount)
	}

	// Keep current behavior backwards-compatible when callers only set InvokeTimeout.
	if cfg.SegmentedRequestTimeout == 0 {
		cfg.SegmentedRequestTimeout = cfg.InvokeTimeout
	}

	// Default SegmentedTimedOutCollectorPeriod to InvokeTimeout when not set.
	if cfg.SegmentedTimedOutCollectorPeriod <= 0 {
		cfg.SegmentedTimedOutCollectorPeriod = cfg.InvokeTimeout
	}

	ase := &aseImpl{
		cfg:           cfg,
		transport:     transport,
		closed:        false,
		stopCh:        make(chan struct{}),
		aseClientImpl: aseClientImpl{clientTransactions: make(map[InvokeID]transaction)},
		aseServerImpl: aseServerImpl{
			confirmedHandlers:              make(map[ServiceChoice]ConfirmedHandler),
			unconfirmedHandlers:            make(map[ServiceChoice]UnconfirmedHandler),
			inboundSegmentedServerEntries:  make(map[segmentedServerKey]*segmentedServerEntry),
			outboundSegmentedServerEntries: make(map[segmentedServerKey]*segmentedServerResponseEntry),
			inFlightServerRequests:         make(map[segmentedServerKey]struct{}),
		},
	}

	go ase.timedOutCollector()

	return ase, nil
}

// Close stops the aseImpl and fails all pending transactions.
func (a *aseImpl) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	close(a.stopCh)
	pending := a.clientTransactions
	a.clientTransactions = make(map[InvokeID]transaction)
	inboundSegmented := a.inboundSegmentedServerEntries
	a.inboundSegmentedServerEntries = make(map[segmentedServerKey]*segmentedServerEntry)
	outboundSegmented := a.outboundSegmentedServerEntries
	a.outboundSegmentedServerEntries = make(map[segmentedServerKey]*segmentedServerResponseEntry)
	a.mu.Unlock()

	for _, tx := range pending {
		if tx.stateMachine != nil {
			_, _ = tx.stateMachine.Handle(machineEventClose, machineInput{})
		}
		tx.done <- transactionResult{
			err: newTransactionError(ErrASEClosed, nil),
		}
	}

	for _, entry := range inboundSegmented {
		if entry.machine != nil {
			_, _ = entry.machine.Handle(machineEventClose, machineInput{})
		}
	}

	for _, entry := range outboundSegmented {
		if entry.machine != nil {
			_, _ = entry.machine.Handle(machineEventClose, machineInput{})
		}
	}

	return nil
}

// RegisterConfirmed registers a confirmed request handler for a service choice.
func (a *aseImpl) RegisterConfirmed(choice ServiceChoice, handler ConfirmedHandler) error {
	if handler == nil {
		return bacneterrors.NewValidationError("handler", nil, ErrHandlerNotFound)
	}

	if !IsConfirmedServiceChoice(choice) {
		return bacneterrors.NewValidationError("service choice", choice, ErrInvalidServiceChoice)
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
		return bacneterrors.NewValidationError("handler", nil, ErrHandlerNotFound)
	}

	if !IsUnconfirmedServiceChoice(choice) {
		return bacneterrors.NewValidationError("service choice", choice, ErrInvalidServiceChoice)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.unconfirmedHandlers[choice]; exists {
		return ErrHandlerAlreadyRegistered
	}
	a.unconfirmedHandlers[choice] = handler
	return nil
}

// BeginConfirmedServiceRequest sends a confirmed request and waits for B-X.confirm.
func (a *aseImpl) BeginConfirmedServiceRequest(ctx context.Context, req ConfirmedRequestICI) (ConfirmICI, error) {
	if !req.Priority.Valid() {
		return ConfirmICI{}, bacneterrors.NewValidationError("priority", req.Priority, ErrInvalidASEConfig)
	}

	if !IsConfirmedServiceChoice(req.ServiceRequest.ServiceChoice) {
		return ConfirmICI{}, bacneterrors.NewValidationError("service choice", req.ServiceRequest.ServiceChoice, ErrInvalidServiceChoice)
	}

	if a.segmentationRequiredConfirmed(len(req.ServiceRequest.Payload)) {
		return ConfirmICI{}, ErrSegmentationNotSupported
	}

	txID, tx, err := a.startTransaction(
		req.Destination,
		req.ServiceRequest.ServiceChoice,
		len(req.ServiceRequest.Payload),
		req.SegmentationSupported,
		req.MaxSegmentsAccepted,
		req.MaxAPDULengthAccepted,
	)
	if err != nil {
		return ConfirmICI{}, err
	}

	output, err := tx.stateMachine.Handle(machineEventSendConfirmedRequest, machineInput{ConfirmedRequest: &req.ServiceRequest})
	if err != nil {
		log.Logger.Error("apdu begin confirmed state-machine handle", "error", err)
		a.finishClientTransaction(txID)
		return ConfirmICI{}, err
	}

	if output.OutboundAPDU == nil {
		a.finishClientTransaction(txID)
		return ConfirmICI{}, invalidStateTransition(tx.stateMachine.Role(), tx.stateMachine.State(), machineEventSendConfirmedRequest)
	}

	for {
		packet, err := buildOutboundNPDU(req.Destination, req.Priority, true, *output.OutboundAPDU)
		if err != nil {
			log.Logger.Error("apdu begin confirmed build outbound npdu", "error", err)
			a.finishClientTransaction(txID)
			return ConfirmICI{}, err
		}

		if err := a.transport.SendNPDU(ctx, req.Destination, packet); err != nil {
			log.Logger.Error("apdu begin confirmed send npdu", "error", err)
			wrapped := fmt.Errorf("%w: %v", ErrTransportFailure, err)
			if tx.stateMachine != nil {
				if out, smErr := tx.stateMachine.Handle(machineEventCannotSend, machineInput{Cause: wrapped}); smErr == nil && out.Confirm != nil {
					a.finishClientTransaction(txID)
					return out.Confirm.confirm, out.Confirm.err
				}
			}
			a.finishClientTransaction(txID)
			return ConfirmICI{}, wrapped
		}

		timer := time.NewTimer(a.cfg.InvokeTimeout)
		select {
		case <-ctx.Done():
			timer.Stop()
			if tx.stateMachine != nil {
				if _, closeErr := tx.stateMachine.Handle(machineEventClose, machineInput{}); closeErr != nil {
					log.Logger.Warn("apdu begin confirmed close state-machine ignored", "error", closeErr, "invoke_id", txID)
				}
			}
			a.finishClientTransaction(txID)
			return ConfirmICI{}, ctx.Err()
		case <-timer.C:
			if tx.stateMachine == nil {
				a.finishClientTransaction(txID)
				return ConfirmICI{}, ErrAPDUTimeout
			}

			timeoutOut, smErr := tx.stateMachine.Handle(machineEventTimeout, machineInput{})
			if smErr != nil {
				log.Logger.Error("apdu begin confirmed timeout state-machine handle", "error", smErr)
				a.finishClientTransaction(txID)
				return ConfirmICI{}, smErr
			}

			switch timeoutOut.action {
			case machineActionResendConfirmedRequest:
				if timeoutOut.OutboundAPDU == nil {
					a.finishClientTransaction(txID)
					return ConfirmICI{}, invalidStateTransition(tx.stateMachine.Role(), tx.stateMachine.State(), machineEventTimeout)
				}
				output = timeoutOut
				continue
			case machineActionFailTimeout:
				a.finishClientTransaction(txID)
				return ConfirmICI{}, ErrAPDUTimeout
			default:
				a.finishClientTransaction(txID)
				return ConfirmICI{}, invalidStateTransition(tx.stateMachine.Role(), tx.stateMachine.State(), machineEventTimeout)
			}
		case result := <-tx.done:
			timer.Stop()
			if result.err != nil {
				return result.confirm, result.err
			}
			if result.confirm.ServiceResponse != nil {
				result.confirm.ServiceResponse = &ServiceResult{Payload: slices.Clone(result.confirm.ServiceResponse.Payload)}
			}
			return result.confirm, nil
		}
	}
}

// SendUnconfirmed sends an unconfirmed request.
func (a *aseImpl) SendUnconfirmed(ctx context.Context, req UnconfirmedRequestICI) error {
	if !req.Priority.Valid() {
		return bacneterrors.NewValidationError("priority", req.Priority, ErrInvalidASEConfig)
	}

	if !IsUnconfirmedServiceChoice(req.ServiceRequest.ServiceChoice) {
		return bacneterrors.NewValidationError("service choice", req.ServiceRequest.ServiceChoice, ErrInvalidServiceChoice)
	}
	if a.segmentationRequiredUnconfirmed(len(req.ServiceRequest.Payload)) {
		return ErrSegmentationNotSupported
	}

	machine := newUnconfirmedClientMachineWithConfig(unconfirmedClientMachineConfig{
		requestPayloadLength: len(req.ServiceRequest.Payload),
	})
	output, err := machine.Handle(machineEventSendUnconfirmedRequest, machineInput{UnconfirmedRequest: &req.ServiceRequest})
	if err != nil {
		log.Logger.Error("apdu send unconfirmed state-machine handle", "error", err)
		return err
	}

	if output.OutboundAPDU == nil {
		return invalidStateTransition(machine.Role(), machine.State(), machineEventSendUnconfirmedRequest)
	}

	packet, err := buildOutboundNPDU(req.Destination, req.Priority, false, *output.OutboundAPDU)
	if err != nil {
		log.Logger.Error("apdu send unconfirmed build outbound npdu", "error", err)
		return err
	}
	if err := a.transport.SendNPDU(ctx, req.Destination, packet); err != nil {
		log.Logger.Error("apdu send unconfirmed transport send", "error", err)
		return fmt.Errorf("%w: %v", ErrTransportFailure, err)
	}
	return nil
}

// OnInboundNPDU decodes the APDU in an inbound NPDU and dispatches it.
//
// Ownership note: packet.APDUBytes() is the NPDU->APDU ingress boundary and
// already returns a defensive copy. The APDU package then passes that slice
// through internal state-machine and dispatch paths without re-cloning.
func (a *aseImpl) OnInboundNPDU(ctx context.Context, src netprim.Address, packet npdu.NetworkLayerProtocolDataUnit) error {
	if !packet.Valid() {
		return bacneterrors.NewValidationError("npdu", packet, ErrDecodeFailure)
	}
	if packet.IsNetworkLayerMessage() {
		return ErrInvalidPDUType
	}

	apduBytes := packet.APDUBytes()
	log.Logger.Debug(
		"apdu inbound npdu",
		"src_network", src.Network,
		"src_mac_length", len(src.AddrPortBytes()),
		"priority", packet.Priority(),
		"payload_bytes", len(apduBytes),
	)

	decoded, err := decodeAPDU(apduBytes)
	if err != nil {
		log.Logger.Error("apdu inbound decode apdu", "error", err)
		return fmt.Errorf("%w: %v", ErrDecodeFailure, err)
	}

	log.Logger.Debug(
		"apdu inbound decode success",
		"pdu_type", decoded.Type,
		"invoke_id", decoded.InvokeID,
		"service_choice", decoded.ServiceChoice,
		"segmented_message", decoded.SegmentedMessage,
		"more_follows", decoded.MoreFollows,
		"payload_bytes", len(decoded.Payload),
	)

	switch decoded.Type {
	case PDUTypeConfirmedRequest:
		return a.handleConfirmedRequest(ctx, src, packet.Priority(), decoded)
	case PDUTypeUnconfirmedRequest:
		return a.handleUnconfirmedRequest(ctx, src, packet.Priority(), decoded)
	case PDUTypeSegmentACK:
		if handled, err := a.handleSegmentACK(ctx, src, decoded); handled || err != nil {
			return err
		}

		return ErrUnexpectedPDU
	case PDUTypeSimpleACK, PDUTypeComplexACK, PDUTypeError, PDUTypeReject, PDUTypeAbort:
		return a.completeTransaction(ctx, src, packet.Priority(), decoded)
	default:
		return bacneterrors.NewValidationError("pdu type", decoded.Type, ErrInvalidPDUType)
	}
}

func (a *aseImpl) startTransaction(
	expectedPeer netprim.Address,
	expectedServiceChoice ServiceChoice,
	requestPayloadLength int,
	segmentation SegmentationSupport,
	maxSegmentsAccepted MaxSegmentsAccepted,
	maxAPDUSizeAccepted MaxApduLengthAccepted,
) (InvokeID, transaction, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return 0, transaction{}, ErrASEClosed
	}

	if len(a.clientTransactions) >= a.cfg.MaxConcurrentInvokes {
		return 0, transaction{}, ErrNoInvokeIDAvailable
	}

	if requestPayloadLength < 0 {
		requestPayloadLength = 0
	}

	for range 256 {
		id := a.nextInvokeID
		a.nextInvokeID++
		if _, exists := a.clientTransactions[id]; exists {
			continue
		}

		tx := transaction{
			done: make(chan transactionResult, 1),
			stateMachine: newConfirmedClientMachineWithConfig(confirmedClientMachineConfig{
				invokeID:             id,
				segmentation:         segmentation,
				maxSegmentsAccepted:  maxSegmentsAccepted,
				maxAPDUSizeAccepted:  maxAPDUSizeAccepted,
				maxRetries:           transactionRetryCount(a.cfg.APDURetries),
				requestPayloadLength: requestPayloadLength,
			}),
			expectedPeer: netprim.Address{
				Network:  expectedPeer.Network,
				AddrPort: expectedPeer.AddrPort,
			},
			expectedServiceChoice: expectedServiceChoice,
		}

		a.clientTransactions[id] = tx

		return id, tx, nil
	}

	return 0, transaction{}, ErrNoInvokeIDAvailable
}

func (a *aseImpl) finishClientTransaction(id InvokeID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.clientTransactions, id)
}

func (a *aseImpl) segmentExchangeTimeout() time.Duration {
	if a.cfg.SegmentedRequestTimeout > 0 {
		return a.cfg.SegmentedRequestTimeout
	}

	return a.cfg.InvokeTimeout
}

func (a *aseImpl) syncSegmentedServerResponseEntry(src netprim.Address, priority netprim.NetworkPriority, machine *confirmedServerMachine) {
	key := segmentedServerKeyFor(src, machine.variables.invokeID)

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		delete(a.outboundSegmentedServerEntries, key)
		return
	}

	if machine.State() != machineStateAwaitSegmentACK {
		delete(a.outboundSegmentedServerEntries, key)
		return
	}

	expiresAt := time.Now().Add(a.segmentExchangeTimeout())
	machine.SetSegmentTimeout(expiresAt)

	a.outboundSegmentedServerEntries[key] = &segmentedServerResponseEntry{
		machine:  machine,
		priority: priority,
		src: netprim.Address{
			Network:  src.Network,
			AddrPort: src.AddrPort,
		},
		expiresAt: expiresAt,
	}
}

func (a *aseImpl) handleSegmentACK(ctx context.Context, src netprim.Address, in inboundAPDU) (bool, error) {
	a.mu.Lock()
	_, clientOK := a.clientTransactions[in.InvokeID]
	_, responseOK := a.outboundSegmentedServerEntries[segmentedServerKeyFor(src, in.InvokeID)]
	a.mu.Unlock()

	if clientOK {
		if err := a.failTransaction(src, in, machineEventUnexpectedPDU, ErrSegmentationNotSupported); err != nil {
			return true, err
		}
		return true, ErrSegmentationNotSupported
	}

	if !responseOK {
		return false, nil
	}

	return true, a.handleSegmentedServerResponseACK(ctx, src, in)
}

func (a *aseImpl) handleSegmentedServerResponseACK(ctx context.Context, src netprim.Address, in inboundAPDU) error {
	key := segmentedServerKeyFor(src, in.InvokeID)

	a.mu.Lock()
	entry, ok := a.outboundSegmentedServerEntries[key]
	a.mu.Unlock()
	if !ok {
		return ErrUnexpectedPDU
	}

	if in.Server {
		return ErrUnexpectedPDU
	}

	if entry.machine.SegmentTimeoutExpired(time.Now()) {
		output, err := entry.machine.Handle(machineEventTimeout, machineInput{Cause: ErrAPDUTimeout})
		if err != nil {
			return err
		}
		if output.OutboundAPDU != nil {
			if err := a.sendConfirmedServerOutput(ctx, src, entry.priority, entry.machine, output, machineEventTimeout); err != nil {
				return err
			}
		}
		if entry.machine.State() != machineStateAwaitSegmentACK {
			a.mu.Lock()
			delete(a.outboundSegmentedServerEntries, key)
			a.mu.Unlock()
		}
		return ErrAPDUTimeout
	}

	output, err := entry.machine.Handle(machineEventInboundSegmentACK, machineInput{InboundAPDU: &in})
	if err != nil {
		return err
	}

	if output.OutboundAPDU != nil {
		if err := a.sendConfirmedServerOutput(ctx, src, entry.priority, entry.machine, output, machineEventInboundSegmentACK); err != nil {
			return err
		}
	} else {
		a.syncSegmentedServerResponseEntry(src, entry.priority, entry.machine)
	}

	return nil
}

func (a *aseImpl) handleConfirmedRequest(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, in inboundAPDU) error {
	if in.SegmentedMessage {
		return a.handleSegmentedConfirmedRequest(ctx, src, priority, in)
	}
	return a.handleUnsegmentedConfirmedRequest(ctx, src, priority, in)
}

// handleUnsegmentedConfirmedRequest dispatches a non-segmented inbound confirmed
// request through a fresh server machine to the registered handler.
//
// Duplicate requests (same src + invokeID) that arrive while the handler is still
// running are silently dropped per §5.4.4 (ignorance policy).
func (a *aseImpl) handleUnsegmentedConfirmedRequest(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, in inboundAPDU) error {
	key := segmentedServerKeyFor(src, in.InvokeID)

	a.mu.Lock()
	if _, inFlight := a.inFlightServerRequests[key]; inFlight {
		a.mu.Unlock()
		log.Logger.Debug(
			"apdu confirmed duplicate request dropped",
			"invoke_id", in.InvokeID,
			"service_choice", in.ServiceChoice,
			"src_network", src.Network,
			"src_mac_length", len(src.AddrPortBytes()),
		)
		return nil // duplicate while handler is running; drop silently (§5.4.4)
	}
	a.inFlightServerRequests[key] = struct{}{}
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.inFlightServerRequests, key)
		a.mu.Unlock()
	}()

	machine := newConfirmedServerMachineWithConfig(confirmedServerMachineConfig{
		invokeID:                     in.InvokeID,
		serviceChoice:                in.ServiceChoice,
		requesterSegmentation:        segmentationSupportFromInboundConfirmedRequest(in),
		requesterMaxSegmentsAccepted: in.MaxSegmentsAccepted,
		requesterMaxAPDUSizeAccepted: in.MaxAPDULengthAccepted,
		segmentation:                 a.cfg.Segmentation,
		preferredWindowSize:          segmentWindowSize(a.cfg.PreferredWindowSize),
		maxRetries:                   transactionRetryCount(a.cfg.APDURetries),
		maxAPDUSizeAccepted:          a.cfg.MaxAPDUSizeAccepted,
		requestPayloadLength:         len(in.Payload),
		maxSegmentDuplicates:         maxDuplicateCount(a.cfg.MaxSegmentDuplicates),
	})

	if _, err := machine.Handle(machineEventInboundConfirmedRequest, machineInput{InboundAPDU: &in}); err != nil {
		return err
	}
	return a.dispatchToHandler(ctx, src, priority, machine, in)
}

// handleSegmentedConfirmedRequest routes the first segment of a segmented request
// to startSegmentedReceive and continuation segments to continueSegmentedReceive.
func (a *aseImpl) handleSegmentedConfirmedRequest(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, in inboundAPDU) error {
	if in.SequenceNumber == 0 {
		return a.startSegmentedReceive(ctx, src, priority, in)
	}
	return a.continueSegmentedReceive(ctx, src, priority, in)
}

// startSegmentedReceive handles the first segment (SequenceNumber==0) of a new
// segmented confirmed request.
//
// If the local ASE does not support receiving segmented requests an Abort PDU is
// sent immediately.  Otherwise, a server machine is created, the first segment is
// buffered, a Segment-ACK is sent if required, and the entry is stored for
// subsequent segments.
func (a *aseImpl) startSegmentedReceive(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, in inboundAPDU) error {
	key := segmentedServerKeyFor(src, in.InvokeID)

	a.mu.Lock()
	machineEntry, entryExists := a.inboundSegmentedServerEntries[key]
	a.mu.Unlock()

	var machine *confirmedServerMachine

	if entryExists {
		if machineEntry.machine.SegmentTimeoutExpired(time.Now()) {
			output, err := machineEntry.machine.Handle(machineEventSegmentTimeout, machineInput{Cause: ErrAPDUTimeout})
			if err != nil {
				return err
			}

			if output.OutboundAPDU != nil {
				if err := a.sendConfirmedServerOutput(ctx, src, priority, machineEntry.machine, output, machineEventSegmentTimeout); err != nil {
					return err
				}
			}

			a.mu.Lock()
			delete(a.inboundSegmentedServerEntries, key)
			a.mu.Unlock()

			return ErrAPDUTimeout
		}

		machine = machineEntry.machine
	} else {
		machine = newConfirmedServerMachineWithConfig(confirmedServerMachineConfig{
			invokeID:                     in.InvokeID,
			serviceChoice:                in.ServiceChoice,
			requesterSegmentation:        segmentationSupportFromInboundConfirmedRequest(in),
			requesterMaxSegmentsAccepted: in.MaxSegmentsAccepted,
			requesterMaxAPDUSizeAccepted: in.MaxAPDULengthAccepted,
			segmentation:                 a.cfg.Segmentation,
			preferredWindowSize:          segmentWindowSize(a.cfg.PreferredWindowSize),
			maxRetries:                   transactionRetryCount(a.cfg.APDURetries),
			maxAPDUSizeAccepted:          a.cfg.MaxAPDUSizeAccepted,
			requestPayloadLength:         len(in.Payload),
			maxSegmentDuplicates:         maxDuplicateCount(a.cfg.MaxSegmentDuplicates),
		})
	}

	output, err := machine.Handle(machineEventInboundConfirmedRequest, machineInput{InboundAPDU: &in})
	if err != nil {
		return err
	}

	if output.OutboundAPDU != nil {
		if err := a.sendConfirmedServerOutput(ctx, src, priority, machine, output, machineEventInboundConfirmedRequest); err != nil {
			return err
		}
	}

	// check if this was an existing machine => duplicate apdu, so no need to check machine state or store machine
	if entryExists {
		if machine.State() == machineStateAborted {
			a.mu.Lock()
			delete(a.inboundSegmentedServerEntries, key)
			a.mu.Unlock()
			if output.action == machineActionSendAbort {
				return ErrSegmentationNotSupported
			}
		}
		machineEntry.expiresAt = time.Now().Add(a.cfg.SegmentedRequestTimeout)
		machineEntry.machine.SetSegmentTimeout(machineEntry.expiresAt)
		return nil
	}

	if output.action == machineActionSendAbort {
		return ErrSegmentationNotSupported
	}

	// If the machine already reached AwaitResponse (single-segment "segmented" request),
	// dispatch directly without storing an entry.
	if machine.State() == machineStateAwaitResponse {
		assembled, err := a.assembleInboundAPDU(in, machine)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrTransportFailure, err)
		}

		return a.dispatchToHandler(ctx, src, priority, machine, assembled)
	}

	// Store the entry for future continuation segments.
	expiryTime := time.Now().Add(a.cfg.SegmentedRequestTimeout)
	entry := &segmentedServerEntry{
		machine:      machine,
		priority:     priority,
		firstInbound: in,
		expiresAt:    expiryTime,
	}
	entry.machine.SetSegmentTimeout(expiryTime)
	a.mu.Lock()
	a.inboundSegmentedServerEntries[key] = entry

	a.mu.Unlock()

	return nil
}

// continueSegmentedReceive feeds a continuation segment (SequenceNumber>0) into
// the existing server machine for that (src, invokeID) pair.
//
// Returns ErrUnexpectedPDU when no matching segmented transaction is found.
func (a *aseImpl) continueSegmentedReceive(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, in inboundAPDU) error {
	key := segmentedServerKeyFor(src, in.InvokeID)

	a.mu.Lock()
	entry, ok := a.inboundSegmentedServerEntries[key]
	a.mu.Unlock()

	if !ok {
		return ErrUnexpectedPDU
	}

	if entry.machine.SegmentTimeoutExpired(time.Now()) {
		output, err := entry.machine.Handle(machineEventSegmentTimeout, machineInput{Cause: ErrAPDUTimeout})
		if err != nil {
			return err
		}

		if output.OutboundAPDU != nil {
			if err := a.sendConfirmedServerOutput(ctx, src, priority, entry.machine, output, machineEventSegmentTimeout); err != nil {
				return err
			}
		}

		// lazy delete when APDU comes in for expired entry
		a.mu.Lock()
		delete(a.inboundSegmentedServerEntries, key)
		a.mu.Unlock()

		return ErrAPDUTimeout
	}

	output, err := entry.machine.Handle(machineEventInboundSegment, machineInput{InboundAPDU: &in})
	if err != nil {
		return err
	}

	if output.OutboundAPDU != nil {
		if err := a.sendConfirmedServerOutput(ctx, src, priority, entry.machine, output, machineEventInboundSegment); err != nil {
			return err
		}
	}

	switch entry.machine.State() {
	case machineStateAwaitResponse:
		// All segments received; clean up and dispatch to handler.
		a.mu.Lock()
		delete(a.inboundSegmentedServerEntries, key)
		a.mu.Unlock()
		assembled, err := a.assembleInboundAPDU(entry.firstInbound, entry.machine)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrTransportFailure, err)
		}

		return a.dispatchToHandler(ctx, src, entry.priority, entry.machine, assembled)

	case machineStateAborted:
		a.mu.Lock()
		delete(a.inboundSegmentedServerEntries, key)
		a.mu.Unlock()
		if output.action == machineActionSendSegmentACK {
			return ErrSegmentationNotSupported
		}
		if output.action == machineActionSendAbort {
			return ErrSegmentationNotSupported
		}
		return ErrUnexpectedPDU

	default:
		// Still receiving segments.
		entry.expiresAt = time.Now().Add(a.cfg.SegmentedRequestTimeout)
		entry.machine.SetSegmentTimeout(entry.expiresAt)
		return nil
	}
}

// assembleInboundAPDU constructs a synthetic non-segmented inboundAPDU from the
// metadata of the first segment and the fully accumulated payload held by machine.
func (a *aseImpl) assembleInboundAPDU(firstSeg inboundAPDU, machine *confirmedServerMachine) (inboundAPDU, error) {
	payload := machine.AssembledPayload()
	if payload == nil {
		return inboundAPDU{}, ErrTransactionNotReady
	}

	return inboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		SegmentedResponseAccepted: firstSeg.SegmentedResponseAccepted,
		MaxSegmentsAccepted:       firstSeg.MaxSegmentsAccepted,
		MaxAPDULengthAccepted:     firstSeg.MaxAPDULengthAccepted,
		InvokeID:                  firstSeg.InvokeID,
		ServiceChoice:             firstSeg.ServiceChoice,
		Payload:                   payload,
	}, nil
}

// dispatchToHandler invokes the registered ConfirmedHandler for the assembled
// request and sends the terminal response APDU back to the peer.
//
// The machine must already be in machineStateAwaitResponse when this is called.
func (a *aseImpl) dispatchToHandler(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, machine *confirmedServerMachine, in inboundAPDU) error {
	a.mu.Lock()
	handler, ok := a.confirmedHandlers[in.ServiceChoice]
	a.mu.Unlock()

	if !ok {
		return a.handleConfirmedServerFailure(ctx, src, priority, machine, ErrHandlerNotFound, ConfirmedResponseTypeReject)
	}

	result, err := handler(ctx, ConfirmedIndicationICI{
		Source:                src,
		InvokeID:              in.InvokeID,
		MaxAPDULengthAccepted: in.MaxAPDULengthAccepted,
		SegmentationSupported: segmentationSupportFromInboundConfirmedRequest(in),
		MaxSegmentsAccepted:   in.MaxSegmentsAccepted,
		Priority:              priority,
		DataExpectingReply:    true,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: in.ServiceChoice,
			Payload:       in.Payload,
		},
	})
	if err != nil {
		return a.handleConfirmedServerFailure(ctx, src, priority, machine, err, ConfirmedResponseTypeError)
	}

	if result.InvokeID != in.InvokeID {
		return a.handleConfirmedServerFailure(
			ctx, src, priority, machine,
			bacneterrors.NewValidationError("invoke id", result.InvokeID, ErrInvalidASEConfig),
			ConfirmedResponseTypeReject,
		)
	}
	if !result.Destination.Equal(src) {
		return a.handleConfirmedServerFailure(
			ctx, src, priority, machine,
			bacneterrors.NewValidationError("destination", result.Destination, ErrInvalidASEConfig),
			ConfirmedResponseTypeReject,
		)
	}

	if machine.maxAPDUSizeExceeded(len(result.ServiceResponse.Payload)) {
		responseType := result.ResponseType
		output, err := machine.Handle(machineEventResponseRequiresSegmentation, machineInput{
			Cause:               ErrSegmentationNotSupported,
			HandlerResult:       &result.ServiceResponse,
			HandlerResponseType: &responseType,
		})
		if err != nil {
			return err
		}
		if err := a.sendConfirmedServerOutput(ctx, src, priority, machine, output, machineEventResponseRequiresSegmentation); err != nil {
			return err
		}
		if machine.State() == machineStateAwaitSegmentACK {
			return nil
		}
		return ErrSegmentationNotSupported
	}

	responseType := result.ResponseType
	output, err := machine.Handle(machineEventResponseReady, machineInput{HandlerResult: &result.ServiceResponse, HandlerResponseType: &responseType})
	if err != nil {
		return err
	}
	return a.sendConfirmedServerOutput(ctx, src, priority, machine, output, machineEventResponseReady)
}

func (a *aseImpl) handleConfirmedServerFailure(
	ctx context.Context,
	src netprim.Address,
	priority netprim.NetworkPriority,
	machine protocolStateMachine,
	cause error,
	responseType ConfirmedResponseType,
) error {
	output, err := machine.Handle(machineEventHandlerError, machineInput{Cause: cause, HandlerResponseType: &responseType})
	if err != nil {
		log.Logger.Error("apdu confirmed server failure state-machine handle", "error", err)
		return err
	}

	if err := a.sendConfirmedServerOutput(ctx, src, priority, machine, output, machineEventHandlerError); err != nil {
		return err
	}

	return cause
}

func (a *aseImpl) sendConfirmedServerOutput(
	ctx context.Context,
	src netprim.Address,
	priority netprim.NetworkPriority,
	machine protocolStateMachine,
	output machineOutput,
	event machineEvent,
) error {
	if output.action == machineActionNone {
		return nil
	}

	var outbound []*outboundAPDU
	if len(output.OutboundAPDUs) > 0 {
		outbound = output.OutboundAPDUs
	} else if output.OutboundAPDU != nil {
		outbound = []*outboundAPDU{output.OutboundAPDU}
	}

	if len(outbound) == 0 {
		return invalidStateTransition(machine.Role(), machine.State(), event)
	}

	for _, out := range outbound {
		if out == nil {
			return invalidStateTransition(machine.Role(), machine.State(), event)
		}

		packet, err := buildOutboundNPDU(src, priority, false, *out)
		if err != nil {
			log.Logger.Error("apdu confirmed server output build outbound npdu", "error", err)
			return err
		}

		if err := a.transport.SendNPDU(ctx, src, packet); err != nil {
			log.Logger.Error("apdu confirmed server output send npdu", "error", err)
			wrapped := fmt.Errorf("%w: %v", ErrTransportFailure, err)
			_, _ = machine.Handle(machineEventCannotSend, machineInput{Cause: wrapped})
			return wrapped
		}
	}

	if serverMachine, ok := machine.(*confirmedServerMachine); ok {
		a.syncSegmentedServerResponseEntry(src, priority, serverMachine)
	}

	return nil
}

const unconfirmedRequestAPDUHeaderLength = 2

func (a *aseImpl) segmentationRequiredConfirmed(payloadLen int) bool {
	if a.cfg.MaxAPDUSizeAccepted == 0 {
		return false
	}

	return confirmedRequestAPDUHeaderLength+payloadLen > int(a.cfg.MaxAPDUSizeAccepted)
}

func (a *aseImpl) segmentationRequiredUnconfirmed(payloadLen int) bool {
	if a.cfg.MaxAPDUSizeAccepted == 0 {
		return false
	}

	return unconfirmedRequestAPDUHeaderLength+payloadLen > int(a.cfg.MaxAPDUSizeAccepted)
}

func segmentationSupportFromInboundConfirmedRequest(in inboundAPDU) SegmentationSupport {
	if in.SegmentedMessage {
		if in.SegmentedResponseAccepted {
			return SegmentationSupportBoth
		}
		return SegmentationSupportTransmit
	}

	if in.SegmentedResponseAccepted {
		return SegmentationSupportReceive
	}

	return SegmentationSupportNo
}

func (a *aseImpl) handleUnconfirmedRequest(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, in inboundAPDU) error {
	machine := newUnconfirmedServerMachineWithConfig(unconfirmedServerMachineConfig{
		requestPayloadLength: len(in.Payload),
	})
	if _, err := machine.Handle(machineEventInboundUnconfirmedRequest, machineInput{}); err != nil {
		return err
	}

	a.mu.Lock()
	handler, ok := a.unconfirmedHandlers[in.ServiceChoice]
	a.mu.Unlock()
	if !ok {
		_, _ = machine.Handle(machineEventHandlerError, machineInput{})
		return ErrHandlerNotFound
	}
	if err := handler(ctx, UnconfirmedIndicationICI{
		Source:   src,
		Priority: priority,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: in.ServiceChoice,
			Payload:       in.Payload,
		},
	}); err != nil {
		_, _ = machine.Handle(machineEventHandlerError, machineInput{})
		return err
	}
	_, _ = machine.Handle(machineEventHandlerDone, machineInput{})
	return nil
}

func (a *aseImpl) completeTransaction(ctx context.Context, src netprim.Address, priority netprim.NetworkPriority, in inboundAPDU) error {
	a.mu.Lock()
	tx, ok := a.clientTransactions[in.InvokeID]
	a.mu.Unlock()
	if !ok {
		return ErrUnexpectedPDU
	}

	if !tx.matchesInboundAPDU(src, in) {
		return ErrUnexpectedPDU
	}

	// Client-side segmented terminal responses are not supported in v1.
	if in.Type == PDUTypeComplexACK && in.SegmentedMessage {
		abortErr := a.sendClientAbortForUnsupportedSegmentation(ctx, src, priority, in.InvokeID)
		if err := a.failTransaction(src, in, machineEventUnexpectedPDU, ErrSegmentationNotSupported); err != nil {
			return err
		}
		if abortErr != nil {
			return abortErr
		}
		return ErrSegmentationNotSupported
	}

	event, err := machineEventForInboundTerminalPDU(in.Type)
	if err != nil {
		return bacneterrors.NewValidationError("pdu type", in.Type, ErrInvalidPDUType)
	}

	// The machine builds the transactionResult from the inbound APDU so
	// that PDU-type decision logic stays inside the machine.
	output, err := tx.stateMachine.Handle(event, machineInput{InboundAPDU: &in})
	if err != nil {
		return err
	}
	if output.Confirm == nil {
		return invalidStateTransition(tx.stateMachine.Role(), tx.stateMachine.State(), event)
	}

	a.finishClientTransaction(in.InvokeID)
	tx.done <- *output.Confirm
	return nil
}

func (a *aseImpl) sendClientAbortForUnsupportedSegmentation(ctx context.Context, dst netprim.Address, priority netprim.NetworkPriority, invokeID InvokeID) error {
	packet, err := buildOutboundNPDU(dst, priority, false, outboundAPDU{
		Type:     PDUTypeAbort,
		InvokeID: invokeID,
		Payload:  []byte{byte(AbortReasonSegmentationNotSupported)},
	})
	if err != nil {
		return err
	}

	if err := a.transport.SendNPDU(ctx, dst, packet); err != nil {
		return fmt.Errorf("%w: %v", ErrTransportFailure, err)
	}

	return nil
}

func (a *aseImpl) failTransaction(src netprim.Address, in inboundAPDU, event machineEvent, cause error) error {
	a.mu.Lock()
	tx, ok := a.clientTransactions[in.InvokeID]
	a.mu.Unlock()
	if !ok {
		return ErrUnexpectedPDU
	}

	if !tx.matchesInboundAPDU(src, in) {
		return ErrUnexpectedPDU
	}

	output, err := tx.stateMachine.Handle(event, machineInput{Cause: cause})
	if err != nil {
		return err
	}
	if output.Confirm == nil {
		return invalidStateTransition(tx.stateMachine.Role(), tx.stateMachine.State(), event)
	}

	a.finishClientTransaction(in.InvokeID)
	tx.done <- *output.Confirm
	return nil
}

// timedOutCollector periodically removes expired segmented-server entries and
// sends Abort PDUs for expired outbound segmented responses.
//
// It exits promptly when Close() is called via the stopCh signal, rather than
// waiting for a full sleep period to elapse.
func (a *aseImpl) timedOutCollector() {
	ticker := time.NewTicker(a.cfg.SegmentedTimedOutCollectorPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
		}

		now := time.Now()
		latestTimedOut := now.Add(-a.cfg.TimeoutGracePeriod)
		var expiredResponses []*segmentedServerResponseEntry

		a.mu.Lock()

		for key, entry := range a.inboundSegmentedServerEntries {
			if entry.expiresAt.Before(latestTimedOut) {
				delete(a.inboundSegmentedServerEntries, key)
			}
		}

		for key, entry := range a.outboundSegmentedServerEntries {
			if entry.expiresAt.Before(now) {
				expiredResponses = append(expiredResponses, entry)
				delete(a.outboundSegmentedServerEntries, key)
			}
		}

		a.mu.Unlock()

		for _, entry := range expiredResponses {
			output, err := entry.machine.Handle(machineEventTimeout, machineInput{Cause: ErrAPDUTimeout})
			if err != nil {
				log.Logger.Warn(
					"apdu timed out collector machine timeout handle ignored",
					"error", err,
					"invoke_id", entry.machine.variables.invokeID,
					"src_network", entry.src.Network,
					"src_mac_length", len(entry.src.AddrPortBytes()),
				)

				continue
			}
			if output.OutboundAPDU != nil {
				if sendErr := a.sendConfirmedServerOutput(context.Background(), entry.src, entry.priority, entry.machine, output, machineEventTimeout); sendErr != nil {
					log.Logger.Warn(
						"apdu timed out collector send output ignored",
						"error", sendErr,
						"invoke_id", entry.machine.variables.invokeID,
						"src_network", entry.src.Network,
						"src_mac_length", len(entry.src.AddrPortBytes()),
					)
				}
			}
		}

	}
}

func buildOutboundNPDU(dst netprim.Address, priority netprim.NetworkPriority, expectingReply bool, apdu outboundAPDU) (npdu.NetworkLayerProtocolDataUnit, error) {
	if !priority.Valid() {
		return npdu.NetworkLayerProtocolDataUnit{}, bacneterrors.NewValidationError("priority", priority, ErrInvalidASEConfig)
	}

	apduBytes, err := encodeAPDU(apdu)
	if err != nil {
		log.Logger.Error("apdu build outbound encode apdu", "error", err)
		return npdu.NetworkLayerProtocolDataUnit{}, fmt.Errorf("%w: %v", ErrEncodeFailure, err)
	}

	if dst.Network.IsLocal() {
		packet, err := npdu.NewLocalAPDU(priority, expectingReply, apduBytes)
		if err != nil {
			log.Logger.Error("apdu build outbound local npdu", "error", err)
			if errors.Is(err, npdu.ErrInvalidLength) {
				return npdu.NetworkLayerProtocolDataUnit{}, fmt.Errorf("%w: %v", ErrEncodeFailure, err)
			}

			return npdu.NetworkLayerProtocolDataUnit{}, err
		}

		return *packet, nil
	}

	packet, err := npdu.NewRoutedAPDU(
		npdu.UltimateDestinationNetworkNumber(dst.Network),
		npdu.UltimateDestinationMacLayerAddress(dst.AddrPortBytes()),
		255,
		priority,
		expectingReply,
		apduBytes,
	)

	if err != nil {
		log.Logger.Error("apdu build outbound routed npdu", "error", err)
		if errors.Is(err, npdu.ErrInvalidLength) || errors.Is(err, npdu.ErrInvalidPriority) {
			return npdu.NetworkLayerProtocolDataUnit{}, fmt.Errorf("%w: %v", ErrEncodeFailure, err)
		}
		return npdu.NetworkLayerProtocolDataUnit{}, err
	}
	return *packet, nil
}
