package apdu

// confirmedClientMachine scaffolds the clause 5.4 confirmed-request state
// machine for a requesting application entity.
//
// The initial implementation supports unsegmented transactions and makes
// segmentation-related transitions explicit by returning ErrSegmentationNotSupported.
type confirmedClientMachine struct {
	state     machineState
	variables confirmedClientTransactionVariables
}

const confirmedRequestAPDUHeaderLength = 4

func newConfirmedClientMachine() *confirmedClientMachine {
	return newConfirmedClientMachineWithConfig(confirmedClientMachineConfig{})
}

func newConfirmedClientMachineWithConfig(cfg confirmedClientMachineConfig) *confirmedClientMachine {
	if cfg.requestPayloadLength < 0 {
		cfg.requestPayloadLength = 0
	}

	return &confirmedClientMachine{
		state: machineStateIdle,
		variables: confirmedClientTransactionVariables{
			invokeID:              cfg.invokeID,
			segmentation:          cfg.segmentation,
			maxSegmentsAccepted:   cfg.maxSegmentsAccepted,
			maxAPDUSizeAccepted:   cfg.maxAPDUSizeAccepted,
			maxRetries:            cfg.maxRetries,
			retryCount:            0,
			requestServiceChoice:  0,
			requestPayload:        nil,
			requestPayloadLength:  cfg.requestPayloadLength,
			responsePayloadLength: 0,
			segmented:             segmentedTransactionVariables{},
		},
	}
}

func (m *confirmedClientMachine) Role() machineRole {
	return machineRoleConfirmedClient
}

func (m *confirmedClientMachine) State() machineState {
	return m.state
}

func (m *confirmedClientMachine) segmentationRequired(payloadLen int) bool {
	if m.variables.maxAPDUSizeAccepted == 0 {
		return false
	}

	return confirmedRequestAPDUHeaderLength+payloadLen > int(m.variables.maxAPDUSizeAccepted)
}

func expectedInboundTerminalPDUTypeForEvent(event machineEvent) (PDUType, bool) {
	switch event {
	case machineEventInboundSimpleACK:
		return PDUTypeSimpleACK, true
	case machineEventInboundComplexACK:
		return PDUTypeComplexACK, true
	case machineEventInboundError:
		return PDUTypeError, true
	case machineEventInboundReject:
		return PDUTypeReject, true
	case machineEventInboundAbort:
		return PDUTypeAbort, true
	default:
		return 0, false
	}
}

func (m *confirmedClientMachine) buildFailureConfirm(result ConfirmResult, err error, apdu *inboundAPDU) transactionResult {
	if err == nil {
		err = ErrInvalidStateTransition
	}

	return transactionResult{
		confirm: ConfirmICI{InvokeID: m.variables.invokeID, Result: result},
		err:     newTransactionError(err, apdu),
	}
}

func (m *confirmedClientMachine) buildOutboundConfirmedRequest(in *ConfirmedRequest) *outboundAPDU {
	if in != nil {
		m.variables.requestServiceChoice = in.ServiceChoice
		m.variables.requestPayload = make([]byte, len(in.Payload))
		copy(m.variables.requestPayload, in.Payload)
		m.variables.requestPayloadLength = len(in.Payload)
	}

	payload := make([]byte, m.variables.requestPayloadLength)
	if len(m.variables.requestPayload) > 0 {
		copy(payload, m.variables.requestPayload)
	}

	return &outboundAPDU{
		Type:             PDUTypeConfirmedRequest,
		SegmentedMessage: false,
		MoreFollows:      false,
		SegmentedResponseAccepted: m.variables.maxSegmentsAccepted != MaxSegmentsUnspecified &&
			(m.variables.segmentation == SegmentationSupportReceive || m.variables.segmentation == SegmentationSupportBoth),
		MaxSegmentsAccepted:   m.variables.maxSegmentsAccepted,
		MaxAPDULengthAccepted: m.variables.maxAPDUSizeAccepted,
		InvokeID:              m.variables.invokeID,
		ServiceChoice:         m.variables.requestServiceChoice,
		Payload:               payload,
	}
}

// buildConfirm constructs the transactionResult for a terminal inbound PDU.
//
// The machine owns this mapping so that the ASE does not need to interpret
// PDU type semantics; it simply forwards the result on the transaction channel.
func (m *confirmedClientMachine) buildConfirm(in *inboundAPDU) transactionResult {
	if in == nil {
		return transactionResult{err: newTransactionError(ErrInvalidPDUType, nil)}
	}

	invokeID := m.variables.invokeID

	switch in.Type {
	case PDUTypeSimpleACK:
		return transactionResult{
			confirm: ConfirmICI{InvokeID: invokeID, Result: ConfirmResultPositiveAck},
		}
	case PDUTypeComplexACK:
		return transactionResult{
			confirm: ConfirmICI{
				InvokeID:        invokeID,
				Result:          ConfirmResultPositiveAck,
				ServiceResponse: &ServiceResult{Payload: in.Payload},
			},
		}
	case PDUTypeError:
		return transactionResult{
			confirm: ConfirmICI{InvokeID: invokeID, Result: ConfirmResultError},
			err:     newTransactionError(ErrRemoteError, in),
		}
	case PDUTypeReject:
		return transactionResult{
			confirm: ConfirmICI{InvokeID: invokeID, Result: ConfirmResultReject},
			err:     newTransactionError(ErrRemoteReject, in),
		}
	case PDUTypeAbort:
		return transactionResult{
			confirm: ConfirmICI{InvokeID: invokeID, Result: ConfirmResultAbort},
			err:     newTransactionError(ErrRemoteAbort, in),
		}
	default:
		return transactionResult{err: newTransactionError(ErrInvalidPDUType, in)}
	}
}

// Handle advances the machine by one event and returns the resulting output.
//
// For machineEventSendConfirmedRequest the caller must supply
// in.ConfirmedRequest; the machine emits OutboundAPDU for transport.
//
// For inbound terminal-PDU events the caller must supply in.InboundAPDU so
// that the machine can construct the ConfirmICI without the ASE duplicating
// PDU-type decision logic.
func (m *confirmedClientMachine) Handle(event machineEvent, in machineInput) (machineOutput, error) {
	switch m.state {
	case machineStateIdle:
		return m.handleInIdleState(event, in)
	case machineStateAwaitResponse:
		return m.handleInAwaitResponseState(event, in)
	case machineStateAwaitSegmentACK:
		return m.handleInAwaitSegmentACKState(event, in)
	case machineStateCompleted:
		return m.handleInCompletedState(event, in)
	case machineStateAborted:
		return m.handleInAbortedState(event, in)
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedClientMachine) handleInIdleState(event machineEvent, in machineInput) (machineOutput, error) {
	switch event {
	case machineEventSendConfirmedRequest:
		if in.ConfirmedRequest == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		if m.segmentationRequired(len(in.ConfirmedRequest.Payload)) {
			return machineOutput{}, ErrSegmentationNotSupported
		}

		m.state = machineStateAwaitResponse
		m.variables.retryCount = 0
		out := m.buildOutboundConfirmedRequest(in.ConfirmedRequest)
		return machineOutput{
			action:       machineActionSendConfirmedRequest,
			OutboundAPDU: out,
		}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedClientMachine) handleInAwaitResponseState(event machineEvent, in machineInput) (machineOutput, error) {
	switch event {
	case machineEventInboundSimpleACK,
		machineEventInboundComplexACK,
		machineEventInboundError,
		machineEventInboundReject,
		machineEventInboundAbort:
		if in.InboundAPDU == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		expectedType, ok := expectedInboundTerminalPDUTypeForEvent(event)
		if !ok || in.InboundAPDU.Type != expectedType {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		transition, _ := transitionForConfirmedClientInboundNonSegmentedEvent(event)
		m.state = transition.nextState
		confirm := m.buildConfirm(in.InboundAPDU)
		return machineOutput{action: transition.action, Confirm: &confirm}, nil
	case machineEventInboundSegmentACK:
		if _, ok := confirmedClientInboundSegmentedEvents[event]; !ok {
			// segmentation not supported yet
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		return machineOutput{}, ErrSegmentationNotSupported
	case machineEventCannotSend:
		m.state = machineStateAborted
		confirm := m.buildFailureConfirm(ConfirmResultCannotSend, in.Cause, in.InboundAPDU)

		return machineOutput{action: machineActionDeliverCannotSend, Confirm: &confirm}, nil
	case machineEventUnexpectedPDU:
		m.state = machineStateAborted
		cause := in.Cause
		if cause == nil {
			cause = ErrUnexpectedPDU
		}
		confirm := m.buildFailureConfirm(ConfirmResultUnexpectedPDU, cause, in.InboundAPDU)
		return machineOutput{action: machineActionDeliverUnexpectedPDU, Confirm: &confirm}, nil
	case machineEventSecurityErrorReceived:
		m.state = machineStateAborted
		cause := in.Cause
		if cause == nil {
			cause = ErrSecurityError
		}
		confirm := m.buildFailureConfirm(ConfirmResultSecurityError, cause, in.InboundAPDU)
		return machineOutput{action: machineActionDeliverSecurityError, Confirm: &confirm}, nil
	case machineEventTimeout:
		if m.variables.retryCount < m.variables.maxRetries {
			m.variables.retryCount++
			out := m.buildOutboundConfirmedRequest(nil)
			return machineOutput{action: machineActionResendConfirmedRequest, OutboundAPDU: out}, nil
		}
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailTimeout}, nil
	case machineEventClose:
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailClosed}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedClientMachine) handleInAwaitSegmentACKState(event machineEvent, _ machineInput) (machineOutput, error) {
	switch event {
	case machineEventInboundSegmentACK:
		if _, ok := confirmedClientInboundSegmentedEvents[event]; !ok {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		return machineOutput{}, ErrSegmentationNotSupported
	case machineEventTimeout:
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailTimeout}, nil
	case machineEventClose:
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailClosed}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedClientMachine) handleInCompletedState(event machineEvent, _ machineInput) (machineOutput, error) {
	switch event {
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *confirmedClientMachine) handleInAbortedState(event machineEvent, _ machineInput) (machineOutput, error) {
	switch event {
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}
