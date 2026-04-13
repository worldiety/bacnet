package apdu

// confirmedServerMachine scaffolds the clause 5.4 confirmed-service state
// machine for a responding application entity.
//
// It currently models the request -> handler -> ACK flow for unsegmented
// responses and leaves segmented-response handling as an explicit future path.
type confirmedServerMachine struct {
	state machineState
}

func newConfirmedServerMachine() *confirmedServerMachine {
	return &confirmedServerMachine{state: machineStateIdle}
}

func (m *confirmedServerMachine) Role() machineRole {
	return machineRoleConfirmedServer
}

func (m *confirmedServerMachine) State() machineState {
	return m.state
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
			action, _ := confirmedServerResponseNonSegmentedEvents[event]
			m.state = machineStateCompleted
			return action, nil
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
			if _, ok := confirmedClientInboundSegmentedEvents[event]; !ok {
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
	default:
		return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
	}
}
