package apdu

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.wdy.de/bacnet"
)

// --- NewUserElement ---

func TestNewUserElementNilASE(t *testing.T) {
	_, err := NewUserElement(nil)
	if !errors.Is(err, ErrNilASE) {
		t.Fatalf("err = %v, want %v", err, ErrNilASE)
	}
}

func TestNewUserElementValid(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, newTestTransport())
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}
	_, err = NewUserElement(ase)
	if err != nil {
		t.Fatalf("NewUserElement: %v", err)
	}
}

// --- InvokeConfirmed (B-X.request → B-X.confirm) ---

func TestUserElementInvokeConfirmedComplexACK(t *testing.T) {
	transport := newTestTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	ue, _ := NewUserElement(ase)

	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})

	type result struct {
		ack ConfirmedAck
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ack, err := ue.InvokeConfirmed(context.Background(), dst, ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0xAA},
		})
		ch <- result{ack, err}
	}()

	// Inject the B-X.confirm from the peer.
	sent := <-transport.ch
	invokeID := sent.pdu[1]
	if err := ase.OnInbound(context.Background(), dst,
		[]byte{byte(PDUTypeComplexACK), invokeID, byte(ServiceChoiceReadProperty), 0xBB}); err != nil {
		t.Fatalf("OnInbound: %v", err)
	}

	r := <-ch
	if r.err != nil {
		t.Fatalf("InvokeConfirmed: %v", r.err)
	}
	if r.ack.Type != PDUTypeComplexACK {
		t.Errorf("ack type = %v, want %v", r.ack.Type, PDUTypeComplexACK)
	}
	if len(r.ack.Payload) != 1 || r.ack.Payload[0] != 0xBB {
		t.Errorf("ack payload = %v, want [0xBB]", r.ack.Payload)
	}
}

func TestUserElementInvokeConfirmedSimpleACK(t *testing.T) {
	transport := newTestTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	ue, _ := NewUserElement(ase)

	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})

	type result struct {
		ack ConfirmedAck
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ack, err := ue.InvokeConfirmed(context.Background(), dst, ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
		})
		ch <- result{ack, err}
	}()

	sent := <-transport.ch
	invokeID := sent.pdu[1]
	// SimpleACK carries no payload — 3-byte frame.
	if err := ase.OnInbound(context.Background(), dst,
		[]byte{byte(PDUTypeSimpleACK), invokeID, byte(ServiceChoiceReadProperty)}); err != nil {
		t.Fatalf("OnInbound: %v", err)
	}

	r := <-ch
	if r.err != nil {
		t.Fatalf("InvokeConfirmed: %v", r.err)
	}
	if r.ack.Type != PDUTypeSimpleACK {
		t.Errorf("ack type = %v, want %v", r.ack.Type, PDUTypeSimpleACK)
	}
}

func TestUserElementInvokeConfirmedTimeout(t *testing.T) {
	ase, _ := NewASE(ASEConfig{InvokeTimeout: 30 * time.Millisecond, MaxConcurrentInvokes: 4}, testCodec{}, newTestTransport())
	ue, _ := NewUserElement(ase)

	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})
	_, err := ue.InvokeConfirmed(context.Background(), dst, ConfirmedRequest{ServiceChoice: ServiceChoiceReadProperty})
	if !errors.Is(err, ErrAPDUTimeout) {
		t.Fatalf("err = %v, want %v", err, ErrAPDUTimeout)
	}
}

func TestUserElementInvokeConfirmedRemoteError(t *testing.T) {
	transport := newTestTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	ue, _ := NewUserElement(ase)

	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})
	type result struct {
		ack ConfirmedAck
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ack, err := ue.InvokeConfirmed(context.Background(), dst, ConfirmedRequest{ServiceChoice: ServiceChoiceReadProperty})
		ch <- result{ack, err}
	}()

	sent := <-transport.ch
	invokeID := sent.pdu[1]
	ase.OnInbound(context.Background(), dst, //nolint
		[]byte{byte(PDUTypeError), invokeID, byte(ServiceChoiceReadProperty)})

	r := <-ch
	if !errors.Is(r.err, ErrRemoteError) {
		t.Errorf("err = %v, want %v", r.err, ErrRemoteError)
	}
}

func TestUserElementInvokeConfirmedContextCancelled(t *testing.T) {
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, newTestTransport())
	ue, _ := NewUserElement(ase)

	ctx, cancel := context.WithCancel(context.Background())
	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})

	ch := make(chan error, 1)
	go func() {
		_, err := ue.InvokeConfirmed(ctx, dst, ConfirmedRequest{ServiceChoice: ServiceChoiceReadProperty})
		ch <- err
	}()

	cancel()
	err := <-ch
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// --- SendUnconfirmed (B-X.request, unconfirmed) ---

func TestUserElementSendUnconfirmed(t *testing.T) {
	transport := newTestTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	ue, _ := NewUserElement(ase)

	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})
	if err := ue.SendUnconfirmed(context.Background(), dst, UnconfirmedRequest{
		ServiceChoice: ServiceChoiceWhoIs,
		Payload:       []byte{0x01, 0x02},
	}); err != nil {
		t.Fatalf("SendUnconfirmed: %v", err)
	}

	sent := <-transport.ch
	if sent.pdu[0] != byte(PDUTypeUnconfirmedRequest) {
		t.Errorf("pdu type = %d, want %d", sent.pdu[0], byte(PDUTypeUnconfirmedRequest))
	}
	if sent.pdu[2] != byte(ServiceChoiceWhoIs) {
		t.Errorf("service choice = %d, want %d", sent.pdu[2], byte(ServiceChoiceWhoIs))
	}
}

// --- HandleConfirmed (B-X.indication → B-X.response registration) ---

func TestUserElementHandleConfirmedDispatch(t *testing.T) {
	transport := newTestTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	ue, _ := NewUserElement(ase)

	var gotPayload []byte
	if err := ue.HandleConfirmed(ServiceChoiceReadProperty, func(_ context.Context, req ConfirmedRequest) (ServiceResult, error) {
		gotPayload = req.Payload
		return ServiceResult{Payload: []byte{0x42}}, nil
	}); err != nil {
		t.Fatalf("HandleConfirmed: %v", err)
	}

	src, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x02})
	// Simulate B-X.indication: inbound confirmed request.
	if err := ase.OnInbound(context.Background(), src,
		[]byte{byte(PDUTypeConfirmedRequest), 5, byte(ServiceChoiceReadProperty), 0x10}); err != nil {
		t.Fatalf("OnInbound: %v", err)
	}

	// The ASE encodes and sends the B-X.response; wait for it.
	resp := <-transport.ch
	if len(gotPayload) != 1 || gotPayload[0] != 0x10 {
		t.Errorf("handler received payload = %v, want [0x10]", gotPayload)
	}
	if resp.pdu[0] != byte(PDUTypeComplexACK) {
		t.Errorf("response pdu type = %d, want ComplexACK (%d)", resp.pdu[0], byte(PDUTypeComplexACK))
	}
	if resp.pdu[3] != 0x42 {
		t.Errorf("response payload byte = 0x%02X, want 0x42", resp.pdu[3])
	}
}

func TestUserElementHandleConfirmedSimpleACKWhenNoPayload(t *testing.T) {
	transport := newTestTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	ue, _ := NewUserElement(ase)

	ue.HandleConfirmed(ServiceChoiceReadProperty, func(_ context.Context, _ ConfirmedRequest) (ServiceResult, error) { //nolint
		return ServiceResult{}, nil // empty payload → SimpleACK
	})

	src, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x02})
	ase.OnInbound(context.Background(), src, //nolint
		[]byte{byte(PDUTypeConfirmedRequest), 3, byte(ServiceChoiceReadProperty)})

	resp := <-transport.ch
	if resp.pdu[0] != byte(PDUTypeSimpleACK) {
		t.Errorf("response pdu type = %d, want SimpleACK (%d)", resp.pdu[0], byte(PDUTypeSimpleACK))
	}
}

func TestUserElementHandleConfirmedDuplicate(t *testing.T) {
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, newTestTransport())
	ue, _ := NewUserElement(ase)

	h := func(context.Context, ConfirmedRequest) (ServiceResult, error) { return ServiceResult{}, nil }
	if err := ue.HandleConfirmed(ServiceChoiceReadProperty, h); err != nil {
		t.Fatalf("first HandleConfirmed: %v", err)
	}
	if err := ue.HandleConfirmed(ServiceChoiceReadProperty, h); !errors.Is(err, ErrHandlerAlreadyRegistered) {
		t.Errorf("duplicate HandleConfirmed err = %v, want %v", err, ErrHandlerAlreadyRegistered)
	}
}

// --- HandleUnconfirmed (B-X.indication registration, unconfirmed) ---

func TestUserElementHandleUnconfirmedDispatch(t *testing.T) {
	transport := newTestTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	ue, _ := NewUserElement(ase)

	var gotPayload []byte
	if err := ue.HandleUnconfirmed(ServiceChoiceWhoIs, func(_ context.Context, req UnconfirmedRequest) error {
		gotPayload = req.Payload
		return nil
	}); err != nil {
		t.Fatalf("HandleUnconfirmed: %v", err)
	}

	src, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x03})
	// Simulate B-X.indication: inbound unconfirmed request. OnInbound is synchronous.
	if err := ase.OnInbound(context.Background(), src,
		[]byte{byte(PDUTypeUnconfirmedRequest), 0, byte(ServiceChoiceWhoIs), 0x20}); err != nil {
		t.Fatalf("OnInbound: %v", err)
	}

	if len(gotPayload) != 1 || gotPayload[0] != 0x20 {
		t.Errorf("handler received payload = %v, want [0x20]", gotPayload)
	}
}

func TestUserElementHandleUnconfirmedDuplicate(t *testing.T) {
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, newTestTransport())
	ue, _ := NewUserElement(ase)

	h := func(context.Context, UnconfirmedRequest) error { return nil }
	if err := ue.HandleUnconfirmed(ServiceChoiceWhoIs, h); err != nil {
		t.Fatalf("first HandleUnconfirmed: %v", err)
	}
	if err := ue.HandleUnconfirmed(ServiceChoiceWhoIs, h); !errors.Is(err, ErrHandlerAlreadyRegistered) {
		t.Errorf("duplicate HandleUnconfirmed err = %v, want %v", err, ErrHandlerAlreadyRegistered)
	}
}
