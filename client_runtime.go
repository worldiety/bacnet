package bacnet

import (
	"context"
	"errors"
	"net/netip"

	"go.wdy.de/bacnet/apdu"
	"go.wdy.de/bacnet/bip"
	"go.wdy.de/bacnet/common/log"
)

// ClientRuntimeConfig configures the high-level BACnet/IP client runtime.
//
// It combines transport, ASE, and typed APDU client settings into one place so
// callers can bootstrap an end-to-end client workflow with a single constructor.
type ClientRuntimeConfig struct {
	// MaxDatagramSize bounds the UDP datagram size used by the BVLC transport.
	// Zero defaults to DefaultMaxDatagramSize.
	MaxDatagramSize int

	// ASE contains APDU ASE transaction and timeout settings.
	ASE apdu.ASEConfig

	// Client contains typed APDU client defaults.
	Client apdu.ClientConfig
}

// ClientRuntime wires DatagramConn -> Transport -> Stack -> ASE -> typed Client.
//
// Run starts the inbound receive loop. Close stops the ASE and transport.
type ClientRuntime struct {
	transport *bip.Transport
	stack     *bip.Stack
	ase       apdu.ASE
	client    apdu.Client
}

// NewClientRuntime creates a client runtime bound to localAddr.
func NewClientRuntime(localAddr netip.Addr, cfg ClientRuntimeConfig) (*ClientRuntime, error) {
	conn, err := bip.NewDatagramConn(localAddr)
	if err != nil {
		log.Logger.Error("bip client runtime create datagram conn", "error", err, "addr", localAddr)
		return nil, err
	}

	runtime, err := NewClientRuntimeWithConn(conn, cfg)
	if err != nil {
		log.Logger.Error("bip client runtime build with conn", "error", err)
		_ = conn.Close()
		return nil, err
	}

	return runtime, nil
}

// NewClientRuntimeWithConn creates a client runtime from an existing DatagramConn.
//
// This is useful for tests and advanced integrations that provide custom network
// adapters. conn must not be nil.
func NewClientRuntimeWithConn(conn bip.DatagramConn, cfg ClientRuntimeConfig) (*ClientRuntime, error) {
	if conn == nil {
		return nil, bip.ErrNilDatagramConn
	}

	if cfg.MaxDatagramSize == 0 {
		cfg.MaxDatagramSize = bip.DefaultMaxDatagramSize
	}

	transport, err := bip.NewTransport(conn, cfg.MaxDatagramSize)
	if err != nil {
		log.Logger.Error("bip client runtime create transport", "error", err)
		return nil, err
	}

	stack, err := bip.NewStack(transport)
	if err != nil {
		log.Logger.Error("bip client runtime create stack", "error", err)
		return nil, err
	}

	ase, err := apdu.NewASE(cfg.ASE, stack)
	if err != nil {
		log.Logger.Error("bip client runtime create ase", "error", err)
		return nil, err
	}

	client, err := apdu.NewClient(ase, cfg.Client)
	if err != nil {
		log.Logger.Error("bip client runtime create client", "error", err)
		_ = ase.Close()
		return nil, err
	}

	return &ClientRuntime{transport: transport, stack: stack, ase: ase, client: client}, nil
}

// Client returns the typed APDU client facade.
func (r *ClientRuntime) Client() apdu.Client {
	return r.client
}

// ASE returns the underlying APDU application service element.
func (r *ClientRuntime) ASE() apdu.ASE {
	return r.ase
}

// Run starts the inbound receive loop and dispatches inbound frames to the ASE.
func (r *ClientRuntime) Run(ctx context.Context) error {
	return r.stack.Run(ctx, r.ase)
}

// Close shuts down ASE state and closes the underlying datagram transport.
func (r *ClientRuntime) Close() error {
	if r == nil {
		return nil
	}

	var closeErr error

	if r.ase != nil {
		if err := r.ase.Close(); err != nil {
			closeErr = err
		}
	}

	if r.transport != nil {
		if err := r.transport.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}

	return closeErr
}
