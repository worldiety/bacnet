package apdu

import (
	"context"

	"go.wdy.de/bacnet"
)

// UserElement models a BACnet application-layer service user per clause 5.1 of
// ANSI/ASHRAE 135-2024.
//
// The BACnet application layer defines four service primitives exchanged between
// the Application Service Element (ASE) and a user element:
//
//	B-X.request    – user element initiates a service interaction
//	B-X.indication – ASE delivers an inbound service request to the user element
//	B-X.response   – user element returns a result for a confirmed service
//	B-X.confirm    – ASE delivers the remote result to the requesting user element
//
// Mapping to this interface:
//
//	B-X.request  (confirmed)    → InvokeConfirmed (blocks until B-X.confirm)
//	B-X.confirm                 → (returned from InvokeConfirmed)
//	B-X.request  (unconfirmed)  → SendUnconfirmed
//	B-X.indication (confirmed)  → handler registered via HandleConfirmed
//	B-X.response                → (returned by the ConfirmedHandler, forwarded by ASE)
//	B-X.indication (unconfirmed)→ handler registered via HandleUnconfirmed
type UserElement interface {
	// InvokeConfirmed executes the B-X.request → B-X.confirm exchange for a
	// confirmed service. It blocks until the peer's response arrives, the
	// configured invoke timeout elapses, or ctx is cancelled.
	InvokeConfirmed(ctx context.Context, dst bacnet.Address, req ConfirmedRequest) (ConfirmedAck, error)

	// SendUnconfirmed executes the B-X.request primitive for an unconfirmed
	// service. No response is expected or awaited.
	SendUnconfirmed(ctx context.Context, dst bacnet.Address, req UnconfirmedRequest) error

	// HandleConfirmed registers a ConfirmedHandler to receive the B-X.indication
	// primitive for the given confirmed service choice. The handler's return value
	// is forwarded to the originating peer as the B-X.response by the ASE.
	// Returns ErrHandlerAlreadyRegistered if a handler is already present for choice.
	HandleConfirmed(choice ServiceChoice, handler ConfirmedHandler) error

	// HandleUnconfirmed registers an UnconfirmedHandler to receive the
	// B-X.indication primitive for the given unconfirmed service choice.
	// Returns ErrHandlerAlreadyRegistered if a handler is already present for choice.
	HandleUnconfirmed(choice ServiceChoice, handler UnconfirmedHandler) error
}

// userElementImpl is the concrete UserElement implementation.
// It delegates all operations to the underlying ASE, which owns the wire transport
// and transaction lifecycle, so userElementImpl itself is stateless.
type userElementImpl struct {
	ase ASE
}

// NewUserElement constructs a UserElement backed by the given ASE.
// ase must not be nil.
func NewUserElement(ase ASE) (UserElement, error) {
	if ase == nil {
		return nil, ErrNilASE
	}
	return &userElementImpl{ase: ase}, nil
}

// InvokeConfirmed implements UserElement (B-X.request → B-X.confirm).
func (u *userElementImpl) InvokeConfirmed(ctx context.Context, dst bacnet.Address, req ConfirmedRequest) (ConfirmedAck, error) {
	return u.ase.InvokeConfirmed(ctx, dst, req)
}

// SendUnconfirmed implements UserElement (B-X.request, unconfirmed).
func (u *userElementImpl) SendUnconfirmed(ctx context.Context, dst bacnet.Address, req UnconfirmedRequest) error {
	return u.ase.SendUnconfirmed(ctx, dst, req)
}

// HandleConfirmed implements UserElement (B-X.indication → B-X.response registration).
func (u *userElementImpl) HandleConfirmed(choice ServiceChoice, handler ConfirmedHandler) error {
	return u.ase.RegisterConfirmed(choice, handler)
}

// HandleUnconfirmed implements UserElement (B-X.indication registration, unconfirmed).
func (u *userElementImpl) HandleUnconfirmed(choice ServiceChoice, handler UnconfirmedHandler) error {
	return u.ase.RegisterUnconfirmed(choice, handler)
}
