package apdu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/npdu"
)

// --- NewUserElement ---

func TestNewUserElementNilASE(t *testing.T) {
	_, err := NewUserElement(nil)
	if !errors.Is(err, ErrNilASE) {
		t.Fatalf("err = %v, want %v", err, ErrNilASE)
	}
}

func TestNewUserElementValid(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
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
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	ue, _ := NewUserElement(ase)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})

	type result struct {
		confirm ConfirmICI
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		confirm, err := ue.InvokeConfirmed(context.Background(), ConfirmedRequestICI{
			Destination: dst,
			Priority:    netprim.NetworkPriorityNormal,
			ServiceRequest: ConfirmedRequest{
				ServiceChoice: ServiceChoiceReadProperty,
				Payload:       []byte{0xAA},
			},
		})
		ch <- result{confirm: confirm, err: err}
	}()

	sent := <-transport.ch
	outbound, _ := decodeAPDU(sent.packet.APDUBytes())
	ackBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeComplexACK, InvokeID: outbound.InvokeID, ServiceChoice: ServiceChoiceReadProperty, Payload: []byte{0xBB}})
	if err != nil {
		t.Fatalf("encodeAPDU: %v", err)
	}
	ack, _ := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, ackBytes)
	if err := ase.OnInboundNPDU(context.Background(), dst, *ack); err != nil {
		t.Fatalf("OnInboundNPDU: %v", err)
	}

	r := <-ch
	if r.err != nil {
		t.Fatalf("InvokeConfirmed: %v", r.err)
	}
	if r.confirm.Result != ConfirmResultPositiveAck {
		t.Fatalf("confirm result = %v, want %v", r.confirm.Result, ConfirmResultPositiveAck)
	}
	if r.confirm.ServiceResponse == nil || len(r.confirm.ServiceResponse.Payload) != 1 || r.confirm.ServiceResponse.Payload[0] != 0xBB {
		t.Fatalf("confirm payload = %v, want [0xBB]", r.confirm.ServiceResponse)
	}
}

func TestUserElementInvokeConfirmedTimeout(t *testing.T) {
	ase, _ := NewASE(ASEConfig{InvokeTimeout: 30 * time.Millisecond, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
	ue, _ := NewUserElement(ase)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})
	_, err := ue.InvokeConfirmed(context.Background(), ConfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
		},
	})
	if !errors.Is(err, ErrAPDUTimeout) {
		t.Fatalf("err = %v, want %v", err, ErrAPDUTimeout)
	}
}

// --- SendUnconfirmed (B-X.request, unconfirmed) ---

func TestUserElementSendUnconfirmed(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	ue, _ := NewUserElement(ase)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})
	if err := ue.SendUnconfirmed(context.Background(), UnconfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: ServiceChoiceWhoIs,
			Payload:       []byte{0x01, 0x02},
		},
	}); err != nil {
		t.Fatalf("SendUnconfirmed: %v", err)
	}

	sent := <-transport.ch
	apdu, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}
	if apdu.Type != PDUTypeUnconfirmedRequest {
		t.Fatalf("pdu type = %v, want %v", apdu.Type, PDUTypeUnconfirmedRequest)
	}
}

// --- HandleConfirmed (B-X.indication → B-X.response registration) ---

func TestUserElementHandleConfirmedDispatch(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	ue, _ := NewUserElement(ase)

	if err := ue.HandleConfirmed(ServiceChoiceReadProperty, func(_ context.Context, indication ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{
			Destination:     indication.Source,
			InvokeID:        indication.InvokeID,
			ServiceResponse: ServiceResult{Payload: []byte{0x99}},
		}, nil
	}); err != nil {
		t.Fatalf("HandleConfirmed: %v", err)
	}

	src, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x02})
	inboundBytes, err := encodeAPDU(outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  7,
		ServiceChoice:             ServiceChoiceReadProperty,
		MaxSegmentsAccepted:       MaxSegmentsUnspecified,
		MaxAPDULengthAccepted:     1476,
		SegmentedResponseAccepted: false,
		Payload:                   []byte{0x01},
	})
	if err != nil {
		t.Fatalf("encodeAPDU: %v", err)
	}
	inbound, _ := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, inboundBytes)
	if err := ase.OnInboundNPDU(context.Background(), src, *inbound); err != nil {
		t.Fatalf("OnInboundNPDU: %v", err)
	}

	sent := <-transport.ch
	apdu, _ := decodeAPDU(sent.packet.APDUBytes())
	if apdu.Type != PDUTypeComplexACK {
		t.Fatalf("response type = %v, want %v", apdu.Type, PDUTypeComplexACK)
	}
}
