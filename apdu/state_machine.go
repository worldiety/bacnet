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
	switch pduType {
	case PDUTypeSimpleACK:
		return machineEventInboundSimpleACK, nil
	case PDUTypeComplexACK:
		return machineEventInboundComplexACK, nil
	case PDUTypeError:
		return machineEventInboundError, nil
	case PDUTypeReject:
		return machineEventInboundReject, nil
	case PDUTypeAbort:
		return machineEventInboundAbort, nil
	default:
		return 0, ErrInvalidPDUType
	}
}

func invalidStateTransition(role machineRole, state machineState, event machineEvent) error {
	return fmt.Errorf("%w: %s machine in %s cannot handle %s", ErrInvalidStateTransition, role, state, event)
}
