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

func (m *confirmedClientMachine) recordInboundTerminalPDU(pduType PDUType, payloadLen int) error {
	if payloadLen < 0 {
		payloadLen = 0
	}

	m.variables.responsePayloadLength = payloadLen
	m.variables.responsePDUType = pduType
	m.variables.responsePDUTypeSet = true

	switch pduType {
	case PDUTypeSimpleACK, PDUTypeComplexACK:
		m.variables.confirmResult = ConfirmResultPositiveAck
	case PDUTypeError:
		m.variables.confirmResult = ConfirmResultError
	case PDUTypeReject:
		m.variables.confirmResult = ConfirmResultReject
	case PDUTypeAbort:
		m.variables.confirmResult = ConfirmResultAbort
	default:
		return ErrInvalidPDUType
	}

	m.variables.confirmResultSet = true
	return nil
}

func (m *confirmedClientMachine) Handle(event machineEvent) (machineAction, error) {
	switch m.state {
	case machineStateIdle:
		switch event {
		case machineEventSendConfirmedRequest:
			m.state = machineStateAwaitResponse
			return machineActionNone, nil
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	case machineStateAwaitResponse:
		switch event {
		case machineEventInboundSimpleACK,
			machineEventInboundComplexACK,
			machineEventInboundError,
			machineEventInboundReject,
			machineEventInboundAbort:
			transition, _ := transitionForConfirmedClientInboundNonSegmentedEvent(event)
			m.state = transition.nextState
			return transition.action, nil
		case machineEventInboundSegmentACK:
			if _, ok := confirmedClientInboundSegmentedEvents[event]; !ok {
				//segmentation not supported yet
				return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
			}
			return machineActionNone, ErrSegmentationNotSupported
		case machineEventTimeout:
			m.state = machineStateAborted
			return machineActionFailTimeout, nil
		case machineEventClose:
			m.state = machineStateAborted
			return machineActionFailClosed, nil
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	case machineStateAwaitSegmentACK:
		switch event {
		case machineEventInboundSegmentACK:
			if _, ok := confirmedClientInboundSegmentedEvents[event]; !ok {
				return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
			}
			return machineActionNone, ErrSegmentationNotSupported
		case machineEventTimeout:
			m.state = machineStateAborted
			return machineActionFailTimeout, nil
		case machineEventClose:
			m.state = machineStateAborted
			return machineActionFailClosed, nil
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	case machineStateCompleted:
		switch event {
		case machineEventClose:
			return machineActionNone, nil
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	case machineStateAborted:
		switch event {
		case machineEventClose:
			return machineActionNone, nil
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	case machineStateAwaitConfirm:
		switch event {
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	}

	return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
}
