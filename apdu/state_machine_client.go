package apdu

// confirmedClientMachine scaffolds the clause 5.4 confirmed-request state
// machine for a requesting application entity.
//
// The initial implementation supports unsegmented transactions and makes
// segmentation-related transitions explicit by returning ErrSegmentationNotSupported.
type confirmedClientMachine struct {
	state machineState
}

func newConfirmedClientMachine() *confirmedClientMachine {
	return &confirmedClientMachine{state: machineStateIdle}
}

func (m *confirmedClientMachine) Role() machineRole {
	return machineRoleConfirmedClient
}

func (m *confirmedClientMachine) State() machineState {
	return m.state
}

func (m *confirmedClientMachine) Handle(event machineEvent) (machineAction, error) {
	switch m.state {
	case machineStateIdle:
		if event == machineEventSendConfirmedRequest {
			m.state = machineStateAwaitResponse
			return machineActionNone, nil
		}
	case machineStateAwaitResponse:
		switch event {
		case machineEventInboundSimpleACK:
			m.state = machineStateCompleted
			return machineActionDeliverSimpleACK, nil
		case machineEventInboundComplexACK:
			m.state = machineStateCompleted
			return machineActionDeliverComplexACK, nil
		case machineEventInboundError:
			m.state = machineStateAborted
			return machineActionDeliverError, nil
		case machineEventInboundReject:
			m.state = machineStateAborted
			return machineActionDeliverReject, nil
		case machineEventInboundAbort:
			m.state = machineStateAborted
			return machineActionDeliverAbort, nil
		case machineEventInboundSegmentACK:
			return machineActionNone, ErrSegmentationNotSupported
		case machineEventTimeout:
			m.state = machineStateAborted
			return machineActionFailTimeout, nil
		case machineEventClose:
			m.state = machineStateAborted
			return machineActionFailClosed, nil
		}
	case machineStateAwaitSegmentACK:
		switch event {
		case machineEventInboundSegmentACK:
			return machineActionNone, ErrSegmentationNotSupported
		case machineEventTimeout:
			m.state = machineStateAborted
			return machineActionFailTimeout, nil
		case machineEventClose:
			m.state = machineStateAborted
			return machineActionFailClosed, nil
		}
	}

	return machineActionNone, invalidStateTransition(m.Role(), m.state, event)
}
