package apdu

import (
	"errors"
	"testing"
)

func TestConfirmedClientMachineTransitions(t *testing.T) {
	tests := []struct {
		name       string
		events     []machineEvent
		wantState  machineState
		wantAction machineAction
		wantErr    error
	}{
		{
			name:       "request then simple ack completes",
			events:     []machineEvent{machineEventSendConfirmedRequest, machineEventInboundSimpleACK},
			wantState:  machineStateCompleted,
			wantAction: machineActionDeliverSimpleACK,
		},
		{
			name:       "request then complex ack completes",
			events:     []machineEvent{machineEventSendConfirmedRequest, machineEventInboundComplexACK},
			wantState:  machineStateCompleted,
			wantAction: machineActionDeliverComplexACK,
		},
		{
			name:       "request then reject aborts",
			events:     []machineEvent{machineEventSendConfirmedRequest, machineEventInboundReject},
			wantState:  machineStateAborted,
			wantAction: machineActionDeliverReject,
		},
		{
			name:       "request then timeout aborts",
			events:     []machineEvent{machineEventSendConfirmedRequest, machineEventTimeout},
			wantState:  machineStateAborted,
			wantAction: machineActionFailTimeout,
		},
		{
			name:       "close in completed state is noop",
			events:     []machineEvent{machineEventSendConfirmedRequest, machineEventInboundSimpleACK, machineEventClose},
			wantState:  machineStateCompleted,
			wantAction: machineActionNone,
		},
		{
			name:       "close in aborted state is noop",
			events:     []machineEvent{machineEventSendConfirmedRequest, machineEventTimeout, machineEventClose},
			wantState:  machineStateAborted,
			wantAction: machineActionNone,
		},
		{
			name:       "segment ack is scaffolded but unsupported",
			events:     []machineEvent{machineEventSendConfirmedRequest, machineEventInboundSegmentACK},
			wantState:  machineStateAwaitResponse,
			wantAction: machineActionNone,
			wantErr:    ErrSegmentationNotSupported,
		},
		{
			name:       "terminal APDU before request is invalid",
			events:     []machineEvent{machineEventInboundSimpleACK},
			wantState:  machineStateIdle,
			wantAction: machineActionNone,
			wantErr:    ErrInvalidStateTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := newConfirmedClientMachine()
			var gotAction machineAction
			var err error

			for _, event := range tt.events {
				gotAction, err = machine.Handle(event)
				if err != nil {
					break
				}
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Handle() error = %v, want %v", err, tt.wantErr)
			}
			if machine.State() != tt.wantState {
				t.Fatalf("State() = %v, want %v", machine.State(), tt.wantState)
			}
			if gotAction != tt.wantAction {
				t.Fatalf("action = %v, want %v", gotAction, tt.wantAction)
			}
		})
	}
}

func TestConfirmedServerMachineTransitions(t *testing.T) {
	tests := []struct {
		name       string
		events     []machineEvent
		wantState  machineState
		wantAction machineAction
		wantErr    error
	}{
		{
			name:       "request then simple ack response completes",
			events:     []machineEvent{machineEventInboundConfirmedRequest, machineEventResponseReadySimpleACK},
			wantState:  machineStateCompleted,
			wantAction: machineActionSendSimpleACK,
		},
		{
			name:       "request then complex ack response completes",
			events:     []machineEvent{machineEventInboundConfirmedRequest, machineEventResponseReadyComplexACK},
			wantState:  machineStateCompleted,
			wantAction: machineActionSendComplexACK,
		},
		{
			name:       "segmented response path is explicit but unsupported",
			events:     []machineEvent{machineEventInboundConfirmedRequest, machineEventResponseRequiresSegmentation},
			wantState:  machineStateAwaitResponse,
			wantAction: machineActionNone,
			wantErr:    ErrSegmentationNotSupported,
		},
		{
			name:       "handler error aborts",
			events:     []machineEvent{machineEventInboundConfirmedRequest, machineEventHandlerError},
			wantState:  machineStateAborted,
			wantAction: machineActionNone,
		},
		{
			name:       "close in completed state is noop",
			events:     []machineEvent{machineEventInboundConfirmedRequest, machineEventResponseReadySimpleACK, machineEventClose},
			wantState:  machineStateCompleted,
			wantAction: machineActionNone,
		},
		{
			name:       "close in aborted state is noop",
			events:     []machineEvent{machineEventInboundConfirmedRequest, machineEventHandlerError, machineEventClose},
			wantState:  machineStateAborted,
			wantAction: machineActionNone,
		},
		{
			name:       "response before request is invalid",
			events:     []machineEvent{machineEventResponseReadySimpleACK},
			wantState:  machineStateIdle,
			wantAction: machineActionNone,
			wantErr:    ErrInvalidStateTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := newConfirmedServerMachine()
			var gotAction machineAction
			var err error

			for _, event := range tt.events {
				gotAction, err = machine.Handle(event)
				if err != nil {
					break
				}
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Handle() error = %v, want %v", err, tt.wantErr)
			}
			if machine.State() != tt.wantState {
				t.Fatalf("State() = %v, want %v", machine.State(), tt.wantState)
			}
			if gotAction != tt.wantAction {
				t.Fatalf("action = %v, want %v", gotAction, tt.wantAction)
			}
		})
	}
}

func TestMachineEventForInboundTerminalPDU(t *testing.T) {
	tests := []struct {
		name    string
		pduType PDUType
		want    machineEvent
		wantErr error
	}{
		{name: "simple ack", pduType: PDUTypeSimpleACK, want: machineEventInboundSimpleACK},
		{name: "complex ack", pduType: PDUTypeComplexACK, want: machineEventInboundComplexACK},
		{name: "error", pduType: PDUTypeError, want: machineEventInboundError},
		{name: "reject", pduType: PDUTypeReject, want: machineEventInboundReject},
		{name: "abort", pduType: PDUTypeAbort, want: machineEventInboundAbort},
		{name: "invalid", pduType: PDUTypeConfirmedRequest, wantErr: ErrInvalidPDUType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := machineEventForInboundTerminalPDU(tt.pduType)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("machineEventForInboundTerminalPDU() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("machineEventForInboundTerminalPDU() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransitionForConfirmedClientInboundNonSegmentedEvent(t *testing.T) {
	tests := []struct {
		name       string
		event      machineEvent
		wantState  machineState
		wantAction machineAction
		wantOK     bool
	}{
		{
			name:       "simple ack",
			event:      machineEventInboundSimpleACK,
			wantState:  machineStateCompleted,
			wantAction: machineActionDeliverSimpleACK,
			wantOK:     true,
		},
		{
			name:       "error",
			event:      machineEventInboundError,
			wantState:  machineStateAborted,
			wantAction: machineActionDeliverError,
			wantOK:     true,
		},
		{
			name:   "unsupported event",
			event:  machineEventTimeout,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := transitionForConfirmedClientInboundNonSegmentedEvent(tt.event)
			if ok != tt.wantOK {
				t.Fatalf("transitionForConfirmedClientInboundNonSegmentedEvent() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.nextState != tt.wantState {
				t.Fatalf("transition nextState = %v, want %v", got.nextState, tt.wantState)
			}
			if got.action != tt.wantAction {
				t.Fatalf("transition action = %v, want %v", got.action, tt.wantAction)
			}
		})
	}
}

func TestTransitionForConfirmedServerResponseNonSegmentedEvent(t *testing.T) {
	tests := []struct {
		name       string
		event      machineEvent
		wantState  machineState
		wantAction machineAction
		wantOK     bool
	}{
		{
			name:       "simple ack response",
			event:      machineEventResponseReadySimpleACK,
			wantState:  machineStateCompleted,
			wantAction: machineActionSendSimpleACK,
			wantOK:     true,
		},
		{
			name:       "complex ack response",
			event:      machineEventResponseReadyComplexACK,
			wantState:  machineStateCompleted,
			wantAction: machineActionSendComplexACK,
			wantOK:     true,
		},
		{
			name:   "unsupported event",
			event:  machineEventHandlerError,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := transitionForConfirmedServerResponseNonSegmentedEvent(tt.event)
			if ok != tt.wantOK {
				t.Fatalf("transitionForConfirmedServerResponseNonSegmentedEvent() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.nextState != tt.wantState {
				t.Fatalf("transition nextState = %v, want %v", got.nextState, tt.wantState)
			}
			if got.action != tt.wantAction {
				t.Fatalf("transition action = %v, want %v", got.action, tt.wantAction)
			}
		})
	}
}

func TestNewConfirmedClientMachineWithConfigInitializesVariables(t *testing.T) {
	machine := newConfirmedClientMachineWithConfig(confirmedClientMachineConfig{
		invokeID:             7,
		segmentation:         SegmentationTransmit,
		maxSegmentsAccepted:  MaxSegments16,
		maxAPDUSizeAccepted:  480,
		requestPayloadLength: 12,
	})

	if machine.State() != machineStateIdle {
		t.Fatalf("State() = %v, want %v", machine.State(), machineStateIdle)
	}
	if machine.variables.invokeID != 7 {
		t.Fatalf("invokeID = %d, want 7", machine.variables.invokeID)
	}
	if machine.variables.segmentation != SegmentationTransmit {
		t.Fatalf("segmentation = %v, want %v", machine.variables.segmentation, SegmentationTransmit)
	}
	if machine.variables.maxSegmentsAccepted != MaxSegments16 {
		t.Fatalf("maxSegmentsAccepted = %v, want %v", machine.variables.maxSegmentsAccepted, MaxSegments16)
	}
	if machine.variables.maxAPDUSizeAccepted != 480 {
		t.Fatalf("maxAPDUSizeAccepted = %d, want 480", machine.variables.maxAPDUSizeAccepted)
	}
	if machine.variables.requestPayloadLength != 12 {
		t.Fatalf("requestPayloadLength = %d, want 12", machine.variables.requestPayloadLength)
	}
	if machine.variables.responsePDUTypeSet {
		t.Fatalf("responsePDUTypeSet = true, want false")
	}
	if machine.variables.confirmResultSet {
		t.Fatalf("confirmResultSet = true, want false")
	}
	if machine.variables.segmented != (segmentedTransactionVariables{}) {
		t.Fatalf("segmented = %#v, want zero value", machine.variables.segmented)
	}
}

func TestConfirmedClientMachineRecordInboundTerminalPDU(t *testing.T) {
	tests := []struct {
		name       string
		pduType    PDUType
		payloadLen int
		wantResult ConfirmResult
		wantErr    error
	}{
		{name: "simple ack", pduType: PDUTypeSimpleACK, payloadLen: 0, wantResult: ConfirmResultPositiveAck},
		{name: "complex ack", pduType: PDUTypeComplexACK, payloadLen: 4, wantResult: ConfirmResultPositiveAck},
		{name: "error", pduType: PDUTypeError, payloadLen: 2, wantResult: ConfirmResultError},
		{name: "reject", pduType: PDUTypeReject, payloadLen: 1, wantResult: ConfirmResultReject},
		{name: "abort", pduType: PDUTypeAbort, payloadLen: 3, wantResult: ConfirmResultAbort},
		{name: "invalid", pduType: PDUTypeConfirmedRequest, wantErr: ErrInvalidPDUType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := newConfirmedClientMachine()
			err := machine.recordInboundTerminalPDU(tt.pduType, tt.payloadLen)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("recordInboundTerminalPDU() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !machine.variables.responsePDUTypeSet {
				t.Fatalf("responsePDUTypeSet = false, want true")
			}
			if machine.variables.responsePDUType != tt.pduType {
				t.Fatalf("responsePDUType = %v, want %v", machine.variables.responsePDUType, tt.pduType)
			}
			if machine.variables.responsePayloadLength != tt.payloadLen {
				t.Fatalf("responsePayloadLength = %d, want %d", machine.variables.responsePayloadLength, tt.payloadLen)
			}
			if !machine.variables.confirmResultSet {
				t.Fatalf("confirmResultSet = false, want true")
			}
			if machine.variables.confirmResult != tt.wantResult {
				t.Fatalf("confirmResult = %v, want %v", machine.variables.confirmResult, tt.wantResult)
			}
		})
	}
}

func TestNewConfirmedServerMachineWithConfigInitializesVariables(t *testing.T) {
	machine := newConfirmedServerMachineWithConfig(confirmedServerMachineConfig{
		invokeID:                     9,
		requesterSegmentation:        SegmentationReceive,
		requesterMaxSegmentsAccepted: MaxSegments8,
		requesterMaxAPDUSizeAccepted: 1024,
		segmentation:                 SegmentationBoth,
		maxAPDUSizeAccepted:          1476,
		requestPayloadLength:         6,
	})

	if machine.State() != machineStateIdle {
		t.Fatalf("State() = %v, want %v", machine.State(), machineStateIdle)
	}
	if machine.variables.invokeID != 9 {
		t.Fatalf("invokeID = %d, want 9", machine.variables.invokeID)
	}
	if machine.variables.requesterSegmentation != SegmentationReceive {
		t.Fatalf("requesterSegmentation = %v, want %v", machine.variables.requesterSegmentation, SegmentationReceive)
	}
	if machine.variables.requesterMaxSegmentsAccepted != MaxSegments8 {
		t.Fatalf("requesterMaxSegmentsAccepted = %v, want %v", machine.variables.requesterMaxSegmentsAccepted, MaxSegments8)
	}
	if machine.variables.requesterMaxAPDUSizeAccepted != 1024 {
		t.Fatalf("requesterMaxAPDUSizeAccepted = %d, want 1024", machine.variables.requesterMaxAPDUSizeAccepted)
	}
	if machine.variables.segmentation != SegmentationBoth {
		t.Fatalf("segmentation = %v, want %v", machine.variables.segmentation, SegmentationBoth)
	}
	if machine.variables.maxAPDUSizeAccepted != 1476 {
		t.Fatalf("maxAPDUSizeAccepted = %d, want 1476", machine.variables.maxAPDUSizeAccepted)
	}
	if machine.variables.requestPayloadLength != 6 {
		t.Fatalf("requestPayloadLength = %d, want 6", machine.variables.requestPayloadLength)
	}
	if machine.variables.responsePDUTypeSet {
		t.Fatalf("responsePDUTypeSet = true, want false")
	}
	if machine.variables.segmented != (segmentedTransactionVariables{}) {
		t.Fatalf("segmented = %#v, want zero value", machine.variables.segmented)
	}
}

func TestConfirmedServerMachineRecordResponsePDU(t *testing.T) {
	tests := []struct {
		name       string
		pduType    PDUType
		payloadLen int
		wantErr    error
	}{
		{name: "simple ack", pduType: PDUTypeSimpleACK, payloadLen: 0},
		{name: "complex ack", pduType: PDUTypeComplexACK, payloadLen: 5},
		{name: "invalid", pduType: PDUTypeError, wantErr: ErrInvalidPDUType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := newConfirmedServerMachine()
			err := machine.recordResponsePDU(tt.pduType, tt.payloadLen)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("recordResponsePDU() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !machine.variables.responsePDUTypeSet {
				t.Fatalf("responsePDUTypeSet = false, want true")
			}
			if machine.variables.responsePDUType != tt.pduType {
				t.Fatalf("responsePDUType = %v, want %v", machine.variables.responsePDUType, tt.pduType)
			}
			if machine.variables.responsePayloadLength != tt.payloadLen {
				t.Fatalf("responsePayloadLength = %d, want %d", machine.variables.responsePayloadLength, tt.payloadLen)
			}
		})
	}
}

func TestSegmentedEventVariables(t *testing.T) {
	if _, ok := confirmedClientInboundSegmentedEvents[machineEventInboundSegmentACK]; !ok {
		t.Fatalf("confirmedClientInboundSegmentedEvents missing %v", machineEventInboundSegmentACK)
	}
	if _, ok := confirmedServerInboundSegmentedEvents[machineEventInboundSegmentACK]; !ok {
		t.Fatalf("confirmedServerInboundSegmentedEvents missing %v", machineEventInboundSegmentACK)
	}
	if _, ok := confirmedServerResponseSegmentedEvents[machineEventResponseRequiresSegmentation]; !ok {
		t.Fatalf("confirmedServerResponseSegmentedEvents missing %v", machineEventResponseRequiresSegmentation)
	}
	if _, ok := inboundTerminalPDUSegmentedEvents[PDUTypeSegmentACK]; !ok {
		t.Fatalf("inboundTerminalPDUSegmentedEvents missing %v", PDUTypeSegmentACK)
	}
}

func TestUnconfirmedClientMachineTransitions(t *testing.T) {
	tests := []struct {
		name       string
		events     []machineEvent
		wantState  machineState
		wantAction machineAction
		wantErr    error
	}{
		{
			name:       "send completes immediately",
			events:     []machineEvent{machineEventSendUnconfirmedRequest},
			wantState:  machineStateCompleted,
			wantAction: machineActionNone,
		},
		{
			name:       "close in completed state is noop",
			events:     []machineEvent{machineEventSendUnconfirmedRequest, machineEventClose},
			wantState:  machineStateCompleted,
			wantAction: machineActionNone,
		},
		{
			name:       "terminal event before send is invalid",
			events:     []machineEvent{machineEventInboundSimpleACK},
			wantState:  machineStateIdle,
			wantAction: machineActionNone,
			wantErr:    ErrInvalidStateTransition,
		},
		{
			name:       "confirmed request event in idle is invalid",
			events:     []machineEvent{machineEventSendConfirmedRequest},
			wantState:  machineStateIdle,
			wantAction: machineActionNone,
			wantErr:    ErrInvalidStateTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := newUnconfirmedClientMachine()
			var gotAction machineAction
			var err error

			for _, event := range tt.events {
				gotAction, err = machine.Handle(event)
				if err != nil {
					break
				}
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Handle() error = %v, want %v", err, tt.wantErr)
			}
			if machine.State() != tt.wantState {
				t.Fatalf("State() = %v, want %v", machine.State(), tt.wantState)
			}
			if gotAction != tt.wantAction {
				t.Fatalf("action = %v, want %v", gotAction, tt.wantAction)
			}
		})
	}
}

func TestUnconfirmedServerMachineTransitions(t *testing.T) {
	tests := []struct {
		name       string
		events     []machineEvent
		wantState  machineState
		wantAction machineAction
		wantErr    error
	}{
		{
			name:       "receive then handler done completes",
			events:     []machineEvent{machineEventInboundUnconfirmedRequest, machineEventHandlerDone},
			wantState:  machineStateCompleted,
			wantAction: machineActionNone,
		},
		{
			name:       "receive then handler error aborts",
			events:     []machineEvent{machineEventInboundUnconfirmedRequest, machineEventHandlerError},
			wantState:  machineStateAborted,
			wantAction: machineActionNone,
		},
		{
			name:       "receive then close aborts with fail-closed action",
			events:     []machineEvent{machineEventInboundUnconfirmedRequest, machineEventClose},
			wantState:  machineStateAborted,
			wantAction: machineActionFailClosed,
		},
		{
			name:       "close in completed state is noop",
			events:     []machineEvent{machineEventInboundUnconfirmedRequest, machineEventHandlerDone, machineEventClose},
			wantState:  machineStateCompleted,
			wantAction: machineActionNone,
		},
		{
			name:       "close in aborted state is noop",
			events:     []machineEvent{machineEventInboundUnconfirmedRequest, machineEventHandlerError, machineEventClose},
			wantState:  machineStateAborted,
			wantAction: machineActionNone,
		},
		{
			name:       "handler event before receive is invalid",
			events:     []machineEvent{machineEventHandlerDone},
			wantState:  machineStateIdle,
			wantAction: machineActionNone,
			wantErr:    ErrInvalidStateTransition,
		},
		{
			name:       "confirmed request event in idle is invalid",
			events:     []machineEvent{machineEventInboundConfirmedRequest},
			wantState:  machineStateIdle,
			wantAction: machineActionNone,
			wantErr:    ErrInvalidStateTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machine := newUnconfirmedServerMachine()
			var gotAction machineAction
			var err error

			for _, event := range tt.events {
				gotAction, err = machine.Handle(event)
				if err != nil {
					break
				}
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Handle() error = %v, want %v", err, tt.wantErr)
			}
			if machine.State() != tt.wantState {
				t.Fatalf("State() = %v, want %v", machine.State(), tt.wantState)
			}
			if gotAction != tt.wantAction {
				t.Fatalf("action = %v, want %v", gotAction, tt.wantAction)
			}
		})
	}
}

func TestNewUnconfirmedClientMachineWithConfigInitializesVariables(t *testing.T) {
	machine := newUnconfirmedClientMachineWithConfig(unconfirmedClientMachineConfig{
		requestPayloadLength: 20,
	})

	if machine.State() != machineStateIdle {
		t.Fatalf("State() = %v, want %v", machine.State(), machineStateIdle)
	}
	if machine.Role() != machineRoleUnconfirmedClient {
		t.Fatalf("Role() = %v, want %v", machine.Role(), machineRoleUnconfirmedClient)
	}
	if machine.variables.requestPayloadLength != 20 {
		t.Fatalf("requestPayloadLength = %d, want 20", machine.variables.requestPayloadLength)
	}
}

func TestNewUnconfirmedServerMachineWithConfigInitializesVariables(t *testing.T) {
	machine := newUnconfirmedServerMachineWithConfig(unconfirmedServerMachineConfig{
		requestPayloadLength: 42,
	})

	if machine.State() != machineStateIdle {
		t.Fatalf("State() = %v, want %v", machine.State(), machineStateIdle)
	}
	if machine.Role() != machineRoleUnconfirmedServer {
		t.Fatalf("Role() = %v, want %v", machine.Role(), machineRoleUnconfirmedServer)
	}
	if machine.variables.requestPayloadLength != 42 {
		t.Fatalf("requestPayloadLength = %d, want 42", machine.variables.requestPayloadLength)
	}
}

func TestConfirmedClientMachineHandlePanicsOnUnknownState(t *testing.T) {
	machine := newConfirmedClientMachine()
	machine.state = machineState(255)

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Handle() did not panic for unknown state")
		}
	}()

	_, _ = machine.Handle(machineEventSendConfirmedRequest)
}

func TestUnconfirmedClientMachineHandlePanicsOnUnknownState(t *testing.T) {
	machine := newUnconfirmedClientMachine()
	machine.state = machineState(255)

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Handle() did not panic for unknown state")
		}
	}()

	_, _ = machine.Handle(machineEventSendUnconfirmedRequest)
}

func TestUnconfirmedServerMachineHandleReturnsErrorOnUnknownState(t *testing.T) {
	machine := newUnconfirmedServerMachine()
	machine.state = machineState(255)

	_, err := machine.Handle(machineEventInboundUnconfirmedRequest)
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("Handle() error = %v, want %v", err, ErrInvalidStateTransition)
	}
}

//TODO State Machine kontrollieren, und wo nötig anpassen/erweitern
