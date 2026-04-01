package apdu

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.wdy.de/bacnet"
)

type testCodec struct{}

func (testCodec) Encode(apdu OutboundAPDU) ([]byte, error) {
	out := make([]byte, 3+len(apdu.Payload))
	out[0] = byte(apdu.Type)
	out[1] = byte(apdu.InvokeID)
	out[2] = byte(apdu.ServiceChoice)
	copy(out[3:], apdu.Payload)
	return out, nil
}

func (testCodec) Decode(raw []byte) (InboundAPDU, error) {
	if len(raw) < 3 {
		return InboundAPDU{}, ErrDecodeFailure
	}
	return InboundAPDU{
		Type:          PDUType(raw[0]),
		InvokeID:      InvokeID(raw[1]),
		ServiceChoice: ServiceChoice(raw[2]),
		Payload:       cloneBytes(raw[3:]),
	}, nil
}

type sentFrame struct {
	dst bacnet.Address
	pdu []byte
}

type testTransport struct {
	mu   sync.Mutex
	ch   chan sentFrame
	sent []sentFrame
}

func newTestTransport() *testTransport {
	return &testTransport{ch: make(chan sentFrame, 8)}
}

func (t *testTransport) SendAPDU(_ context.Context, dst bacnet.Address, apdu []byte) error {
	frame := sentFrame{dst: dst, pdu: cloneBytes(apdu)}
	t.mu.Lock()
	t.sent = append(t.sent, frame)
	t.mu.Unlock()
	t.ch <- frame
	return nil
}

func TestNewASEValidation(t *testing.T) {
	transport := newTestTransport()
	cfg := ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 8}

	tests := []struct {
		name      string
		cfg       ASEConfig
		codec     Codec
		transport Transport
		wantErr   error
	}{
		{name: "valid", cfg: cfg, codec: testCodec{}, transport: transport},
		{name: "nil codec", cfg: cfg, codec: nil, transport: transport, wantErr: ErrNilCodec},
		{name: "nil transport", cfg: cfg, codec: testCodec{}, transport: nil, wantErr: ErrNilTransport},
		{name: "zero timeout", cfg: ASEConfig{InvokeTimeout: 0, MaxConcurrentInvokes: 8}, codec: testCodec{}, transport: transport, wantErr: ErrInvalidASEConfig},
		{name: "zero max invokes", cfg: ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 0}, codec: testCodec{}, transport: transport, wantErr: ErrInvalidASEConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewASE(tt.cfg, tt.codec, tt.transport)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("NewASE error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewASE error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegisterHandlerDuplicate(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, newTestTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	h := func(context.Context, ConfirmedRequest) (ServiceResult, error) { return ServiceResult{}, nil }
	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, h); err != nil {
		t.Fatalf("first RegisterConfirmed failed: %v", err)
	}
	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, h); !errors.Is(err, ErrHandlerAlreadyRegistered) {
		t.Fatalf("RegisterConfirmed duplicate error = %v, want %v", err, ErrHandlerAlreadyRegistered)
	}
}

func TestInvokeConfirmedRoundTrip(t *testing.T) {
	transport := newTestTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	type callResult struct {
		ack ConfirmedAck
		err error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		ack, err := ase.InvokeConfirmed(context.Background(), dst, ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0xAA},
		})
		resultCh <- callResult{ack: ack, err: err}
	}()

	sent := <-transport.ch
	invokeID := sent.pdu[1]
	if err := ase.OnInbound(context.Background(), dst, []byte{byte(PDUTypeComplexACK), invokeID, byte(ServiceChoiceReadProperty), 0xBB}); err != nil {
		t.Fatalf("OnInbound returned error: %v", err)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("InvokeConfirmed returned error: %v", result.err)
	}
	if result.ack.Type != PDUTypeComplexACK {
		t.Fatalf("ack type = %v, want %v", result.ack.Type, PDUTypeComplexACK)
	}
	if got := result.ack.Payload; len(got) != 1 || got[0] != 0xBB {
		t.Fatalf("ack payload = %v, want [187]", got)
	}
}

func TestInvokeConfirmedTimeout(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: 30 * time.Millisecond, MaxConcurrentInvokes: 4}, testCodec{}, newTestTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x02})
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	_, err = ase.InvokeConfirmed(context.Background(), dst, ConfirmedRequest{ServiceChoice: ServiceChoiceReadProperty})
	if !errors.Is(err, ErrAPDUTimeout) {
		t.Fatalf("InvokeConfirmed error = %v, want %v", err, ErrAPDUTimeout)
	}
}

func TestInboundConfirmedDispatch(t *testing.T) {
	transport := newTestTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, testCodec{}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, req ConfirmedRequest) (ServiceResult, error) {
		if got := req.Payload; len(got) != 1 || got[0] != 0x10 {
			t.Fatalf("handler payload = %v, want [16]", got)
		}
		return ServiceResult{Payload: []byte{0x20}}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed returned error: %v", err)
	}

	src, err := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x03})
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	if err := ase.OnInbound(context.Background(), src, []byte{byte(PDUTypeConfirmedRequest), 7, byte(ServiceChoiceReadProperty), 0x10}); err != nil {
		t.Fatalf("OnInbound returned error: %v", err)
	}

	response := <-transport.ch
	if response.pdu[0] != byte(PDUTypeComplexACK) {
		t.Fatalf("response pdu type = %d, want %d", response.pdu[0], byte(PDUTypeComplexACK))
	}
	if response.pdu[1] != 7 {
		t.Fatalf("response invoke ID = %d, want 7", response.pdu[1])
	}
	if response.pdu[2] != byte(ServiceChoiceReadProperty) {
		t.Fatalf("response service choice = %d, want %d", response.pdu[2], byte(ServiceChoiceReadProperty))
	}
	if len(response.pdu) != 4 || response.pdu[3] != 0x20 {
		t.Fatalf("response payload = %v, want trailing 0x20", response.pdu)
	}
}

