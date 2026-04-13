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
		if event == machineEventInboundConfirmedRequest {
			m.state = machineStateAwaitResponse
			return machineActionNone, nil
		}
	case machineStateAwaitResponse:
		switch event {
		case machineEventResponseReadySimpleACK:
			m.state = machineStateCompleted
			return machineActionSendSimpleACK, nil
		case machineEventResponseReadyComplexACK:
			m.state = machineStateCompleted
			return machineActionSendComplexACK, nil
		case machineEventResponseRequiresSegmentation:
			return machineActionNone, ErrSegmentationNotSupported
		case machineEventHandlerError:
			m.state = machineStateAborted
			return machineActionNone, nil
		case machineEventClose:
			m.state = machineStateAborted
			return machineActionFailClosed, nil
		}
	case machineStateAwaitSegmentACK:
		switch event {
		case machineEventInboundSegmentACK:
			return machineActionNone, ErrSegmentationNotSupported
		case machineEventClose:
			m.state = machineStateAborted
			return machineActionFailClosed, nil
		}
	}

	return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
}
