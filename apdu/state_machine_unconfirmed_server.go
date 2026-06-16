package apdu

// unconfirmedServerTransactionVariables stores the per-transaction state-machine
// variables used by the unconfirmed server path.
type unconfirmedServerTransactionVariables struct {
	requestPayloadLength int
}

// unconfirmedServerMachineConfig holds the constructor arguments for an
// unconfirmed server machine.
type unconfirmedServerMachineConfig struct {
	requestPayloadLength int
}

// unconfirmedServerMachine scaffolds the clause 5.4 unconfirmed-request state
// machine for a receiving application entity.
//
// When an unconfirmed request arrives the machine moves from IDLE to
// AWAIT_RESPONSE (the application handler is executing). Once the handler
// finishes it signals either machineEventHandlerDone (→ COMPLETED) or
// machineEventHandlerError (→ ABORTED). No reply is ever sent to the peer.
type unconfirmedServerMachine struct {
	state     machineState
	variables unconfirmedServerTransactionVariables
}

// newUnconfirmedServerMachine creates an unconfirmed server machine with default config.
func newUnconfirmedServerMachine() *unconfirmedServerMachine {
	return newUnconfirmedServerMachineWithConfig(unconfirmedServerMachineConfig{})
}

// newUnconfirmedServerMachineWithConfig creates an unconfirmed server machine
// initialized with the supplied configuration.
func newUnconfirmedServerMachineWithConfig(cfg unconfirmedServerMachineConfig) *unconfirmedServerMachine {
	if cfg.requestPayloadLength < 0 {
		cfg.requestPayloadLength = 0
	}
	return &unconfirmedServerMachine{
		state: machineStateIdle,
		variables: unconfirmedServerTransactionVariables{
			requestPayloadLength: cfg.requestPayloadLength,
		},
	}
}

// Role returns machineRoleUnconfirmedServer.
func (m *unconfirmedServerMachine) Role() machineRole {
	return machineRoleUnconfirmedServer
}

// State returns the current machine state.
func (m *unconfirmedServerMachine) State() machineState {
	return m.state
}

// Handle processes the given event and returns the resulting action.
//
// Valid transitions:
//
//	IDLE          + machineEventInboundUnconfirmedRequest → AWAIT_RESPONSE / machineActionNone
//	AWAIT_RESPONSE + machineEventHandlerDone              → COMPLETED      / machineActionNone
//	AWAIT_RESPONSE + machineEventHandlerError             → ABORTED        / machineActionNone
//	AWAIT_RESPONSE + machineEventClose                   → ABORTED        / machineActionFailClosed
//	COMPLETED      + machineEventClose                   → COMPLETED      / machineActionNone  (no-op)
//	ABORTED        + machineEventClose                   → ABORTED        / machineActionNone  (no-op)
func (m *unconfirmedServerMachine) Handle(event machineEvent, _ machineInput) (machineOutput, error) {
	switch m.state {
	case machineStateIdle:
		return m.handleInIdleState(event)
	case machineStateAwaitResponse:
		return m.handleInAwaitResponseState(event)
	case machineStateCompleted:
		return m.handleInCompletedState(event)
	case machineStateAborted:
		return m.handleInAbortedState(event)
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *unconfirmedServerMachine) handleInIdleState(event machineEvent) (machineOutput, error) {
	switch event {
	case machineEventInboundUnconfirmedRequest:
		m.state = machineStateAwaitResponse
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *unconfirmedServerMachine) handleInAwaitResponseState(event machineEvent) (machineOutput, error) {
	switch event {
	case machineEventHandlerDone:
		m.state = machineStateCompleted
		return machineOutput{action: machineActionNone}, nil
	case machineEventHandlerError:
		m.state = machineStateAborted
		return machineOutput{action: machineActionNone}, nil
	case machineEventClose:
		m.state = machineStateAborted
		return machineOutput{action: machineActionFailClosed}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *unconfirmedServerMachine) handleInCompletedState(event machineEvent) (machineOutput, error) {
	switch event {
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *unconfirmedServerMachine) handleInAbortedState(event machineEvent) (machineOutput, error) {
	switch event {
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}
