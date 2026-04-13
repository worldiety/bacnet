package apdu

import "fmt"

// protocolStateMachine models a clause 5.4 application protocol state machine.
//
// The current scaffold focuses on unsegmented confirmed-service exchanges while
// reserving explicit states and events for later segmentation work.
// It is intentionally unexported so the public ASE API can remain stable while
// the implementation evolves.
type protocolStateMachine interface {
	Role() machineRole
	State() machineState
	Handle(event machineEvent) (machineAction, error)
}

type machineRole byte

const (
	machineRoleConfirmedClient machineRole = iota
	machineRoleConfirmedServer
)

func (r machineRole) String() string {
	switch r {
	case machineRoleConfirmedClient:
		return "confirmed-client"
	case machineRoleConfirmedServer:
		return "confirmed-server"
	default:
		return fmt.Sprintf("machine-role(%d)", r)
	}
}

type machineState byte

const (
	machineStateIdle machineState = iota
	machineStateAwaitConfirm
	machineStateAwaitResponse
	machineStateAwaitSegmentACK
	machineStateCompleted
	machineStateAborted
)

func (s machineState) String() string {
	switch s {
	case machineStateIdle:
		return "idle"
	case machineStateAwaitConfirm:
		return "await-confirm"
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
	machineEventInboundSimpleACK
	machineEventInboundComplexACK
	machineEventInboundError
	machineEventInboundReject
	machineEventInboundAbort
	machineEventInboundSegmentACK
	machineEventResponseReadySimpleACK
	machineEventResponseReadyComplexACK
	machineEventResponseRequiresSegmentation
	machineEventHandlerError
	machineEventTimeout
	machineEventClose
)

// segmentedTransactionVariables reserves the clause 5.4.1 bookkeeping needed
// for segmented confirmed-service exchanges.
//
// The current implementation does not execute segmented transitions yet, so
// these variables remain zero-value placeholders until segmentation support is
// implemented.
type segmentedTransactionVariables struct {
	sequenceNumber        uint8
	initialSequenceNumber uint8
	lastSequenceNumber    uint8
	proposedWindowSize    uint8
	actualWindowSize      uint8
	retryCount            uint8
	segmentRetryCount     uint8
	sentAllSegments       bool
	segmentCount          uint16
}

// confirmedClientTransactionVariables stores the per-transaction state-machine
// variables used by the confirmed client path.
type confirmedClientTransactionVariables struct {
	invokeID              InvokeID
	segmentation          SegmentationSupport
	maxSegmentsAccepted   MaxSegmentsAccepted
	maxAPDUSizeAccepted   uint16
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
	requesterSegmentation        SegmentationSupport
	requesterMaxSegmentsAccepted MaxSegmentsAccepted
	requesterMaxAPDUSizeAccepted uint16
	segmentation                 SegmentationSupport
	maxAPDUSizeAccepted          uint16
	requestPayloadLength         int
	responsePayloadLength        int
	responsePDUType              PDUType
	responsePDUTypeSet           bool
	segmented                    segmentedTransactionVariables
}

type confirmedClientMachineConfig struct {
	invokeID             InvokeID
	segmentation         SegmentationSupport
	maxSegmentsAccepted  MaxSegmentsAccepted
	maxAPDUSizeAccepted  uint16
	requestPayloadLength int
}

type confirmedServerMachineConfig struct {
	invokeID                     InvokeID
	requesterSegmentation        SegmentationSupport
	requesterMaxSegmentsAccepted MaxSegmentsAccepted
	requesterMaxAPDUSizeAccepted uint16
	segmentation                 SegmentationSupport
	maxAPDUSizeAccepted          uint16
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
	machineEventResponseReadySimpleACK: {
		nextState: machineStateCompleted,
		action:    machineActionSendSimpleACK,
	},
	machineEventResponseReadyComplexACK: {
		nextState: machineStateCompleted,
		action:    machineActionSendComplexACK,
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
	case machineEventResponseReadySimpleACK:
		return "response-ready-simple-ack"
	case machineEventResponseReadyComplexACK:
		return "response-ready-complex-ack"
	case machineEventResponseRequiresSegmentation:
		return "response-requires-segmentation"
	case machineEventHandlerError:
		return "handler-error"
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
	machineActionFailTimeout
	machineActionFailClosed
	machineActionSendSimpleACK
	machineActionSendComplexACK
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
	case machineActionFailTimeout:
		return "fail-timeout"
	case machineActionFailClosed:
		return "fail-closed"
	case machineActionSendSimpleACK:
		return "send-simple-ack"
	case machineActionSendComplexACK:
		return "send-complex-ack"
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
