package apdu

// unconfirmedClientTransactionVariables stores the per-transaction state-machine
// variables used by the unconfirmed client path.
type unconfirmedClientTransactionVariables struct {
	requestPayloadLength int
}

// unconfirmedClientMachineConfig holds the constructor arguments for an
// unconfirmed client machine.
type unconfirmedClientMachineConfig struct {
	requestPayloadLength int
}

// unconfirmedClientMachine scaffolds the clause 5.4 unconfirmed-request state
// machine for a sending application entity.
//
// The machine models the fire-and-forget nature of unconfirmed services:
// it transitions directly from IDLE to COMPLETED on a single send event and
// never waits for a peer response.
type unconfirmedClientMachine struct {
	state     machineState
	variables unconfirmedClientTransactionVariables
}

// newUnconfirmedClientMachine creates an unconfirmed client machine with default config.
func newUnconfirmedClientMachine() *unconfirmedClientMachine {
	return newUnconfirmedClientMachineWithConfig(unconfirmedClientMachineConfig{})
}

// newUnconfirmedClientMachineWithConfig creates an unconfirmed client machine
// initialized with the supplied configuration.
func newUnconfirmedClientMachineWithConfig(cfg unconfirmedClientMachineConfig) *unconfirmedClientMachine {
	if cfg.requestPayloadLength < 0 {
		cfg.requestPayloadLength = 0
	}
	return &unconfirmedClientMachine{
		state: machineStateIdle,
		variables: unconfirmedClientTransactionVariables{
			requestPayloadLength: cfg.requestPayloadLength,
		},
	}
}

// Role returns machineRoleUnconfirmedClient.
func (m *unconfirmedClientMachine) Role() machineRole {
	return machineRoleUnconfirmedClient
}

// State returns the current machine state.
func (m *unconfirmedClientMachine) State() machineState {
	return m.state
}

// Handle processes the given event and returns the resulting output.
//
// Valid transitions:
//
//	IDLE        + machineEventSendUnconfirmedRequest → COMPLETED / machineActionSendUnconfirmedRequest
//	COMPLETED   + machineEventClose                  → COMPLETED / machineActionNone  (no-op)
//	ABORTED     + machineEventClose                  → ABORTED   / machineActionNone  (no-op)
func (m *unconfirmedClientMachine) Handle(event machineEvent, in machineInput) (machineOutput, error) {
	switch m.state {
	case machineStateIdle:
		return m.handleInIdleState(event, in)
	case machineStateCompleted:
		return m.handleInCompletedState(event)
	case machineStateAborted:
		return m.handleInAbortedState(event)
	default:
		panic(invalidStateTransition(m.Role(), m.state, event))
	}
}

func (m *unconfirmedClientMachine) handleInIdleState(event machineEvent, in machineInput) (machineOutput, error) {
	switch event {
	case machineEventSendUnconfirmedRequest:
		if in.UnconfirmedRequest == nil {
			return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
		}
		m.state = machineStateCompleted
		return machineOutput{
			action: machineActionSendUnconfirmedRequest,
			OutboundAPDU: &outboundAPDU{
				Type:          PDUTypeUnconfirmedRequest,
				ServiceChoice: in.UnconfirmedRequest.ServiceChoice,
				Payload:       in.UnconfirmedRequest.Payload,
			},
		}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *unconfirmedClientMachine) handleInCompletedState(event machineEvent) (machineOutput, error) {
	switch event {
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}

func (m *unconfirmedClientMachine) handleInAbortedState(event machineEvent) (machineOutput, error) {
	switch event {
	case machineEventClose:
		return machineOutput{action: machineActionNone}, nil
	default:
		return machineOutput{}, invalidStateTransition(m.Role(), m.state, event)
	}
}
