package apdu

// confirmedServerMachine scaffolds the clause 5.4 confirmed-service state
// machine for a responding application entity.
//
// It currently models the request -> handler -> ACK flow for unsegmented
// responses and leaves segmented-response handling as an explicit future path.
type confirmedServerMachine struct {
	state     machineState
	variables confirmedServerTransactionVariables
}

func newConfirmedServerMachine() *confirmedServerMachine {
	return newConfirmedServerMachineWithConfig(confirmedServerMachineConfig{})
}

func newConfirmedServerMachineWithConfig(cfg confirmedServerMachineConfig) *confirmedServerMachine {
	if cfg.requestPayloadLength < 0 {
		cfg.requestPayloadLength = 0
	}

	return &confirmedServerMachine{
		state: machineStateIdle,
		variables: confirmedServerTransactionVariables{
			invokeID:                     cfg.invokeID,
			requesterSegmentation:        cfg.requesterSegmentation,
			requesterMaxSegmentsAccepted: cfg.requesterMaxSegmentsAccepted,
			requesterMaxAPDUSizeAccepted: cfg.requesterMaxAPDUSizeAccepted,
			segmentation:                 cfg.segmentation,
			maxAPDUSizeAccepted:          cfg.maxAPDUSizeAccepted,
			requestPayloadLength:         cfg.requestPayloadLength,
			responsePayloadLength:        0,
			segmented:                    segmentedTransactionVariables{},
		},
	}
}

func (m *confirmedServerMachine) Role() machineRole {
	return machineRoleConfirmedServer
}

func (m *confirmedServerMachine) State() machineState {
	return m.state
}

func (m *confirmedServerMachine) recordResponsePDU(pduType PDUType, payloadLen int) error {
	if payloadLen < 0 {
		payloadLen = 0
	}

	switch pduType {
	case PDUTypeSimpleACK, PDUTypeComplexACK:
		m.variables.responsePayloadLength = payloadLen
		m.variables.responsePDUType = pduType
		m.variables.responsePDUTypeSet = true
		return nil
	default:
		return ErrInvalidPDUType
	}
}

func (m *confirmedServerMachine) Handle(event machineEvent) (machineAction, error) {
	switch m.state {
	case machineStateIdle:
		switch event {
		case machineEventInboundConfirmedRequest:
			m.state = machineStateAwaitResponse
			return machineActionNone, nil
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	case machineStateAwaitResponse:
		switch event {
		case machineEventResponseReadySimpleACK,
			machineEventResponseReadyComplexACK:
			transition, _ := transitionForConfirmedServerResponseNonSegmentedEvent(event)
			m.state = transition.nextState
			return transition.action, nil
		case machineEventResponseRequiresSegmentation:
			if _, ok := confirmedServerResponseSegmentedEvents[event]; !ok {
				return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
			}
			return machineActionNone, ErrSegmentationNotSupported
		case machineEventHandlerError:
			m.state = machineStateAborted
			return machineActionNone, nil
		case machineEventClose:
			m.state = machineStateAborted
			return machineActionFailClosed, nil
		default:
			return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
		}
	case machineStateAwaitSegmentACK:
		switch event {
		case machineEventInboundSegmentACK:
			if _, ok := confirmedServerInboundSegmentedEvents[event]; !ok {
				return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
			}
			return machineActionNone, ErrSegmentationNotSupported
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
	default:
		return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
	}
}
