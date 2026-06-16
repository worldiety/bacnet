package apdu

import (
	"fmt"
	"time"
)

// machineInput carries the data associated with a state-machine event.
//
// Not every event requires data; callers that have no associated data should
// pass a zero-value machineInput{}.  Machines must guard against nil pointer
// fields when a given event does not provide them.
type machineInput struct {
	// ConfirmedRequest is populated for outbound confirmed-request send events.
	ConfirmedRequest *ConfirmedRequest

	// UnconfirmedRequest is populated for outbound unconfirmed-request send events.
	UnconfirmedRequest *UnconfirmedRequest

	// InboundAPDU is populated for inbound terminal-PDU events
	// (SimpleACK, ComplexACK, Error, Reject, Abort).
	InboundAPDU *inboundAPDU

	// HandlerResult is populated for response-ready events on the server path.
	HandlerResult *ServiceResult

	// HandlerResponseType is populated for confirmed-server response and handler
	// failure events. When nil, the server machine derives ACK type from payload
	// length (SimpleACK for empty, ComplexACK for non-empty).
	HandlerResponseType *ConfirmedResponseType

	// Cause carries an optional boundary error for failure events.
	Cause error
}

// machineOutput carries what the machine wants done after processing an event.
//
// The action field names the high-level intent; the optional pointer fields
// carry the pre-computed data needed to execute that intent so that the ASE
// does not need to repeat any decision logic already made by the machine.
type machineOutput struct {
	// action is the high-level intent determined by the state transition.
	action machineAction

	// Confirm is set by the confirmed-client machine for every deliver-*
	// action.  The ASE sends this result on the waiting transaction channel.
	Confirm *transactionResult

	// OutboundAPDU is set by the confirmed-server machine for every send-*
	// action.  The ASE encodes it and hands it to the network layer.
	OutboundAPDU *outboundAPDU

	// OutboundAPDUs is an optional batch of outbound APDUs emitted as a single
	// machine action (for example, a segmented ComplexACK transmit window).
	// When non-empty, ASE sends these APDUs in order and ignores OutboundAPDU.
	OutboundAPDUs []*outboundAPDU
}

// protocolStateMachine models a clause 5.4 application protocol state machine.
//
// The current scaffold covers unsegmented confirmed-service exchanges plus
// server-side segmented request receive and segmented ComplexACK response send.
// It is intentionally unexported so the public ASE API can remain stable while
// the implementation evolves.
type protocolStateMachine interface {
	Role() machineRole
	State() machineState
	// Handle advances the machine by one event.  in carries any data
	// associated with the event; unneeded fields may be nil.
	Handle(event machineEvent, in machineInput) (machineOutput, error)
}

type machineRole byte

const (
	machineRoleConfirmedClient machineRole = iota
	machineRoleConfirmedServer
	machineRoleUnconfirmedClient
	machineRoleUnconfirmedServer
)

func (r machineRole) String() string {
	switch r {
	case machineRoleConfirmedClient:
		return "confirmed-client"
	case machineRoleConfirmedServer:
		return "confirmed-server"
	case machineRoleUnconfirmedClient:
		return "unconfirmed-client"
	case machineRoleUnconfirmedServer:
		return "unconfirmed-server"
	default:
		return fmt.Sprintf("machine-role(%d)", r)
	}
}

type machineState byte

const (
	machineStateIdle machineState = iota
	machineStateSegmentedRequestReceiving
	machineStateAwaitResponse
	machineStateAwaitSegmentACK
	machineStateCompleted
	machineStateAborted
)

func (s machineState) String() string {
	switch s {
	case machineStateIdle:
		return "idle"
	case machineStateSegmentedRequestReceiving:
		return "segmented-request-receiving"
	case machineStateAwaitResponse:
		return "await-response"
	case machineStateAwaitSegmentACK:
		return "await-segment-ack"
	case machineStateCompleted:
		return "completed"
	case machineStateAborted:
		return "aborted"
	default:
		return fmt.Sprintf("machine-state(%d)", s)
	}
}

type machineEvent byte

const (
	machineEventSendConfirmedRequest machineEvent = iota
	machineEventInboundConfirmedRequest
	machineEventInboundSegment
	machineEventSegmentTimeout
	machineEventInboundSimpleACK
	machineEventInboundComplexACK
	machineEventInboundError
	machineEventInboundReject
	machineEventInboundAbort
	machineEventInboundSegmentACK
	machineEventResponseReady
	machineEventResponseRequiresSegmentation
	machineEventHandlerError
	machineEventHandlerDone
	machineEventSendUnconfirmedRequest
	machineEventInboundUnconfirmedRequest
	machineEventCannotSend
	machineEventUnexpectedPDU
	machineEventSecurityErrorReceived
	machineEventTimeout
	machineEventClose
)

// segmentSequenceNumber is an APDU segment sequence number.
type segmentSequenceNumber uint8

// segmentWindowSize is the proposed/negotiated segment window size.
type segmentWindowSize uint8

// transactionRetryCount tracks APDU retry attempts.
type transactionRetryCount uint8

// segmentRetryCount tracks retries for segmented APDU exchanges.
type segmentRetryCount uint8

// segmentCount is the number of processed APDU segments in a transaction.
type segmentCount uint16

// duplicateCount tracks duplicate segment observations.
type duplicateCount uint8

// maxDuplicateCount is the maximum number of duplicate segments tolerated
// before aborting a segmented receive transaction.
type maxDuplicateCount uint8

const defaultMaxDuplicateCount maxDuplicateCount = 3

// receivedSegmentCount is the number of segments consumed in the active receive window.
type receivedSegmentCount uint8

// segmentedTransactionVariables stores clause 5.4.1 segmentation bookkeeping
// used by confirmed-service state machines.
type segmentedTransactionVariables struct {
	sequenceNumber        segmentSequenceNumber
	initialSequenceNumber segmentSequenceNumber
	lastSequenceNumber    segmentSequenceNumber
	proposedWindowSize    segmentWindowSize
	actualWindowSize      segmentWindowSize
	maxRetries            transactionRetryCount
	retryCount            transactionRetryCount
	segmentRetryCount     segmentRetryCount
	sentAllSegments       bool
	segmentCount          segmentCount
	segmentTimeout        time.Time
	duplicateCount        duplicateCount
	maxDuplicateCount     maxDuplicateCount
	bufferedPayloads      map[segmentSequenceNumber][]byte
	bufferedMoreFollows   map[segmentSequenceNumber]bool
}

func (s *segmentedTransactionVariables) InWindow(seqA, seqB segmentSequenceNumber) bool {
	return segmentWindowSize(seqA-seqB) < s.actualWindowSize
}

func (s *segmentedTransactionVariables) DuplicateInWindow(seqA segmentSequenceNumber, receivedCount receivedSegmentCount) bool {
	return receivedCount != 0 && receivedSegmentCount(seqA-s.initialSequenceNumber) <= receivedCount
}

func (s *segmentedTransactionVariables) resetDuplicateCount() {
	s.duplicateCount = 0
}

// observeDuplicate increments the duplicate count and returns whether the max duplicate count was exceeded
func (s *segmentedTransactionVariables) observeDuplicate() bool {
	if s.duplicateCount < duplicateCount(^maxDuplicateCount(0)) {
		s.duplicateCount++
	}

	return s.maxDuplicateCount != 0 && s.duplicateCount > duplicateCount(s.maxDuplicateCount)
}

func (s *segmentedTransactionVariables) ensureBufferedMaps() {
	if s.bufferedPayloads == nil {
		s.bufferedPayloads = make(map[segmentSequenceNumber][]byte)
	}

	if s.bufferedMoreFollows == nil {
		s.bufferedMoreFollows = make(map[segmentSequenceNumber]bool)
	}
}

func (s *segmentedTransactionVariables) bufferSegment(seq segmentSequenceNumber, payload []byte, moreFollows bool) {
	s.ensureBufferedMaps()
	s.bufferedPayloads[seq] = payload
	s.bufferedMoreFollows[seq] = moreFollows
}

func (s *segmentedTransactionVariables) takeBufferedSegment(seq segmentSequenceNumber) ([]byte, bool, bool) {
	if s.bufferedPayloads == nil {
		return nil, false, false
	}

	payload, okPayload := s.bufferedPayloads[seq]
	if !okPayload {
		return nil, false, false
	}

	moreFollows, okMF := s.bufferedMoreFollows[seq]
	if !okMF {
		moreFollows = true
	}

	delete(s.bufferedPayloads, seq)
	delete(s.bufferedMoreFollows, seq)
	return payload, moreFollows, true
}

// confirmedClientTransactionVariables stores the per-transaction state-machine
// variables used by the confirmed client path.
type confirmedClientTransactionVariables struct {
	invokeID              InvokeID
	segmentation          SegmentationSupport
	maxSegmentsAccepted   MaxSegmentsAccepted
	maxAPDUSizeAccepted   MaxApduLengthAccepted
	maxRetries            transactionRetryCount
	retryCount            transactionRetryCount
	requestServiceChoice  ServiceChoice
	requestPayload        []byte
	requestPayloadLength  int
	responsePayloadLength int
	responsePDUType       PDUType
	responsePDUTypeSet    bool
	confirmResult         ConfirmResult
	confirmResultSet      bool
	segmented             segmentedTransactionVariables
}

// confirmedServerTransactionVariables stores the per-transaction state-machine
// variables used by the confirmed server path.
type confirmedServerTransactionVariables struct {
	invokeID                     InvokeID
	serviceChoice                ServiceChoice
	requesterSegmentation        SegmentationSupport
	requesterMaxSegmentsAccepted MaxSegmentsAccepted
	requesterMaxAPDUSizeAccepted MaxApduLengthAccepted
	segmentation                 SegmentationSupport
	preferredWindowSize          segmentWindowSize
	maxAPDUSizeAccepted          MaxApduLengthAccepted
	requestPayloadLength         int
	responsePayload              []byte
	responsePayloadOffset        int
	responseNextOffset           int
	responsePayloadLength        int
	responsePDUType              PDUType
	responsePDUTypeSet           bool
	// segmentBuffer accumulates received segment payloads for segmented inbound requests.
	segmentBuffer []byte
	// receivedInCurrentWindow counts segments received in the current window.
	receivedInCurrentWindow receivedSegmentCount
	segmented               segmentedTransactionVariables
}

type confirmedClientMachineConfig struct {
	invokeID             InvokeID
	segmentation         SegmentationSupport
	maxSegmentsAccepted  MaxSegmentsAccepted
	maxAPDUSizeAccepted  MaxApduLengthAccepted
	maxRetries           transactionRetryCount
	requestPayloadLength int
}

type confirmedServerMachineConfig struct {
	invokeID                     InvokeID
	serviceChoice                ServiceChoice
	requesterSegmentation        SegmentationSupport
	requesterMaxSegmentsAccepted MaxSegmentsAccepted
	requesterMaxAPDUSizeAccepted MaxApduLengthAccepted
	segmentation                 SegmentationSupport
	preferredWindowSize          segmentWindowSize
	maxSegmentDuplicates         maxDuplicateCount
	maxRetries                   transactionRetryCount
	maxAPDUSizeAccepted          MaxApduLengthAccepted
	requestPayloadLength         int
}

type clientInboundNonSegmentedTransition struct {
	nextState machineState
	action    machineAction
}

type serverResponseNonSegmentedTransition struct {
	nextState machineState
	action    machineAction
}

var confirmedClientInboundNonSegmentedEvents = map[machineEvent]clientInboundNonSegmentedTransition{
	machineEventInboundSimpleACK: {
		nextState: machineStateCompleted,
		action:    machineActionDeliverSimpleACK,
	},
	machineEventInboundComplexACK: {
		nextState: machineStateCompleted,
		action:    machineActionDeliverComplexACK,
	},
	machineEventInboundError: {
		nextState: machineStateAborted,
		action:    machineActionDeliverError,
	},
	machineEventInboundReject: {
		nextState: machineStateAborted,
		action:    machineActionDeliverReject,
	},
	machineEventInboundAbort: {
		nextState: machineStateAborted,
		action:    machineActionDeliverAbort,
	},
}

// Segmented events are tracked explicitly for future clause 5.4 work.
var confirmedClientInboundSegmentedEvents = map[machineEvent]struct{}{
	machineEventInboundSegmentACK: {},
}

var confirmedServerResponseNonSegmentedTransitions = map[machineEvent]serverResponseNonSegmentedTransition{
	machineEventResponseReady: {
		nextState: machineStateCompleted,
		action:    machineActionNone,
	},
}

// Segmented events are tracked explicitly for future clause 5.4 work.
var confirmedServerResponseSegmentedEvents = map[machineEvent]struct{}{
	machineEventResponseRequiresSegmentation: {},
}

// Segmented events are tracked explicitly for future clause 5.4 work.
var confirmedServerInboundSegmentedEvents = map[machineEvent]struct{}{
	machineEventInboundSegmentACK: {},
}

var inboundTerminalPDUNonSegmentedEvents = map[PDUType]machineEvent{
	PDUTypeSimpleACK:  machineEventInboundSimpleACK,
	PDUTypeComplexACK: machineEventInboundComplexACK,
	PDUTypeError:      machineEventInboundError,
	PDUTypeReject:     machineEventInboundReject,
	PDUTypeAbort:      machineEventInboundAbort,
}

// Segmented terminal PDU mappings are declared for future support.
var inboundTerminalPDUSegmentedEvents = map[PDUType]machineEvent{
	PDUTypeSegmentACK: machineEventInboundSegmentACK,
}

func (e machineEvent) String() string {
	switch e {
	case machineEventSendConfirmedRequest:
		return "send-confirmed-request"
	case machineEventInboundConfirmedRequest:
		return "inbound-confirmed-request"
	case machineEventInboundSegment:
		return "inbound-segment"
	case machineEventSegmentTimeout:
		return "segment-timeout"
	case machineEventInboundSimpleACK:
		return "inbound-simple-ack"
	case machineEventInboundComplexACK:
		return "inbound-complex-ack"
	case machineEventInboundError:
		return "inbound-error"
	case machineEventInboundReject:
		return "inbound-reject"
	case machineEventInboundAbort:
		return "inbound-abort"
	case machineEventInboundSegmentACK:
		return "inbound-segment-ack"
	case machineEventResponseReady:
		return "response-ready"
	case machineEventResponseRequiresSegmentation:
		return "response-requires-segmentation"
	case machineEventHandlerError:
		return "handler-error"
	case machineEventHandlerDone:
		return "handler-done"
	case machineEventSendUnconfirmedRequest:
		return "send-unconfirmed-request"
	case machineEventInboundUnconfirmedRequest:
		return "inbound-unconfirmed-request"
	case machineEventCannotSend:
		return "cannot-send"
	case machineEventUnexpectedPDU:
		return "unexpected-pdu"
	case machineEventSecurityErrorReceived:
		return "security-error-received"
	case machineEventTimeout:
		return "timeout"
	case machineEventClose:
		return "close"
	default:
		return fmt.Sprintf("machine-event(%d)", e)
	}
}

func transitionForConfirmedClientInboundNonSegmentedEvent(event machineEvent) (clientInboundNonSegmentedTransition, bool) {
	transition, ok := confirmedClientInboundNonSegmentedEvents[event]
	return transition, ok
}

func transitionForConfirmedServerResponseNonSegmentedEvent(event machineEvent) (serverResponseNonSegmentedTransition, bool) {
	transition, ok := confirmedServerResponseNonSegmentedTransitions[event]
	return transition, ok
}

type machineAction byte

const (
	machineActionNone machineAction = iota
	machineActionDeliverSimpleACK
	machineActionDeliverComplexACK
	machineActionDeliverError
	machineActionDeliverReject
	machineActionDeliverAbort
	machineActionSendAbort
	machineActionFailTimeout
	machineActionFailClosed
	machineActionSendConfirmedRequest
	machineActionResendConfirmedRequest
	machineActionSendUnconfirmedRequest
	machineActionSendSimpleACK
	machineActionSendComplexACK
	machineActionSendError
	machineActionSendReject
	machineActionSendSegmentACK
	machineActionDeliverCannotSend
	machineActionDeliverUnexpectedPDU
	machineActionDeliverSecurityError
)

func (a machineAction) String() string {
	switch a {
	case machineActionNone:
		return "none"
	case machineActionDeliverSimpleACK:
		return "deliver-simple-ack"
	case machineActionDeliverComplexACK:
		return "deliver-complex-ack"
	case machineActionDeliverError:
		return "deliver-error"
	case machineActionDeliverReject:
		return "deliver-reject"
	case machineActionDeliverAbort:
		return "deliver-abort"
	case machineActionSendAbort:
		return "send-abort"
	case machineActionFailTimeout:
		return "fail-timeout"
	case machineActionFailClosed:
		return "fail-closed"
	case machineActionSendConfirmedRequest:
		return "send-confirmed-request"
	case machineActionResendConfirmedRequest:
		return "resend-confirmed-request"
	case machineActionSendUnconfirmedRequest:
		return "send-unconfirmed-request"
	case machineActionSendSimpleACK:
		return "send-simple-ack"
	case machineActionSendComplexACK:
		return "send-complex-ack"
	case machineActionSendError:
		return "send-error"
	case machineActionSendReject:
		return "send-reject"
	case machineActionSendSegmentACK:
		return "send-segment-ack"
	case machineActionDeliverCannotSend:
		return "deliver-cannot-send"
	case machineActionDeliverUnexpectedPDU:
		return "deliver-unexpected-pdu"
	case machineActionDeliverSecurityError:
		return "deliver-security-error"
	default:
		return fmt.Sprintf("machine-action(%d)", a)
	}
}

func machineEventForInboundTerminalPDU(pduType PDUType) (machineEvent, error) {
	event, ok := inboundTerminalPDUNonSegmentedEvents[pduType]
	if !ok {
		return 0, ErrInvalidPDUType
	}

	return event, nil
}

func invalidStateTransition(role machineRole, state machineState, event machineEvent) error {
	return fmt.Errorf("%w: %s machine in %s cannot handle %s", ErrInvalidStateTransition, role, state, event)
}
