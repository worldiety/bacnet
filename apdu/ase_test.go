package apdu

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/npdu"
)

type sentPacket struct {
	dst    netprim.Address
	packet npdu.NetworkLayerProtocolDataUnit
}

type testNPDUTransport struct {
	mu   sync.Mutex
	ch   chan sentPacket
	sent []sentPacket
	err  error
}

func newTestNPDUTransport() *testNPDUTransport {
	return &testNPDUTransport{ch: make(chan sentPacket, 8)}
}

func (t *testNPDUTransport) SendNPDU(_ context.Context, dst netprim.Address, packet npdu.NetworkLayerProtocolDataUnit) error {
	if t.err != nil {
		return t.err
	}
	frame := sentPacket{dst: dst, packet: packet}
	t.mu.Lock()
	t.sent = append(t.sent, frame)
	t.mu.Unlock()
	t.ch <- frame
	return nil
}

func TestInvokeConfirmedCannotSend(t *testing.T) {
	transport := newTestNPDUTransport()
	transport.err = errors.New("write failed") //todo the test presets the error, instead of relying on the state machine to recognize the failure and return the error

	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	confirm, err := ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0x01},
		},
	})
	if !errors.Is(err, ErrTransportFailure) {
		t.Fatalf("InvokeConfirmed error = %v, want %v", err, ErrTransportFailure)
	}
	if confirm.Result != ConfirmResultCannotSend {
		t.Fatalf("Confirm.Result = %v, want %v", confirm.Result, ConfirmResultCannotSend)
	}
}

func TestInvokeConfirmedInboundSegmentAckFailsFast(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	type callResult struct {
		confirm ConfirmICI
		err     error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		confirm, err := ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
			Destination: dst,
			Priority:    netprim.NetworkPriorityNormal,
			ServiceRequest: ConfirmedRequest{
				ServiceChoice: ServiceChoiceReadProperty,
				Payload:       []byte{0xAA},
			},
		})
		resultCh <- callResult{confirm: confirm, err: err}
	}()

	sent := <-transport.ch
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}

	segAck, err := npdu.NewLocalAPDU(
		netprim.NetworkPriorityNormal,
		false,
		// Segment-ACK: 4 bytes — type|flags, invokeID, seqNum, windowSize.
		[]byte{byte(PDUTypeSegmentACK << 4), byte(decoded.InvokeID), 0x00, 0x01},
	)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), dst, *segAck)
	if !errors.Is(err, ErrSegmentationNotSupported) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, ErrSegmentationNotSupported)
	}

	result := <-resultCh
	if !errors.Is(result.err, ErrSegmentationNotSupported) {
		t.Fatalf("InvokeConfirmed error = %v, want %v", result.err, ErrSegmentationNotSupported)
	}
	if result.confirm.Result != ConfirmResultUnexpectedPDU {
		t.Fatalf("Confirm.Result = %v, want %v", result.confirm.Result, ConfirmResultUnexpectedPDU)
	}
}

func TestInvokeConfirmedInboundSegmentedComplexACKFailsFast(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	type callResult struct {
		confirm ConfirmICI
		err     error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		confirm, err := ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
			Destination: dst,
			Priority:    netprim.NetworkPriorityNormal,
			ServiceRequest: ConfirmedRequest{
				ServiceChoice: ServiceChoiceReadProperty,
				Payload:       []byte{0xAB},
			},
		})
		resultCh <- callResult{confirm: confirm, err: err}
	}()

	sent := <-transport.ch
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}

	segmentedAckBytes, err := encodeAPDU(outboundAPDU{
		Type:             PDUTypeComplexACK,
		SegmentedMessage: true,
		MoreFollows:      true,
		InvokeID:         decoded.InvokeID,
		SequenceNumber:   0,
		ActualWindowSize: 1,
		ServiceChoice:    ServiceChoiceReadProperty,
		Payload:          []byte{0xBB},
	})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}

	segmentedAck, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, segmentedAckBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), dst, *segmentedAck)
	if !errors.Is(err, ErrSegmentationNotSupported) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, ErrSegmentationNotSupported)
	}

	abortSent := <-transport.ch
	abortDecoded, err := decodeAPDU(abortSent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(abort) returned error: %v", err)
	}
	if abortDecoded.Type != PDUTypeAbort {
		t.Fatalf("abort pdu type = %v, want %v", abortDecoded.Type, PDUTypeAbort)
	}
	if abortDecoded.InvokeID != decoded.InvokeID {
		t.Fatalf("abort invoke ID = %d, want %d", abortDecoded.InvokeID, decoded.InvokeID)
	}
	if len(abortDecoded.Payload) != 1 || abortDecoded.Payload[0] != byte(AbortReasonSegmentationNotSupported) {
		t.Fatalf("abort payload = %v, want [%d]", abortDecoded.Payload, byte(AbortReasonSegmentationNotSupported))
	}
	if !abortSent.dst.Equal(dst) {
		t.Fatalf("abort destination = %v, want %v", abortSent.dst, dst)
	}

	result := <-resultCh
	if !errors.Is(result.err, ErrSegmentationNotSupported) {
		t.Fatalf("BeginConfirmedServiceRequest error = %v, want %v", result.err, ErrSegmentationNotSupported)
	}
	if result.confirm.Result != ConfirmResultUnexpectedPDU {
		t.Fatalf("Confirm.Result = %v, want %v", result.confirm.Result, ConfirmResultUnexpectedPDU)
	}

	impl, ok := ase.(*aseImpl)
	if !ok {
		t.Fatal("expected concrete aseImpl for test assertions")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if len(impl.clientTransactions) != 0 {
		t.Fatalf("clientTransactions = %d, want 0", len(impl.clientTransactions))
	}
}

func TestNewASEValidation(t *testing.T) {
	transport := newTestNPDUTransport()
	cfg := ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 8}

	tests := []struct {
		name      string
		cfg       ASEConfig
		transport NPDUTransport
		wantErr   error
	}{
		{name: "valid", cfg: cfg, transport: transport},
		{name: "nil transport", cfg: cfg, transport: nil, wantErr: ErrNilTransport},
		{name: "zero timeout", cfg: ASEConfig{InvokeTimeout: 0, MaxConcurrentInvokes: 8}, transport: transport, wantErr: ErrInvalidASEConfig},
		{name: "zero max invokes", cfg: ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 0}, transport: transport, wantErr: ErrInvalidASEConfig},
		{name: "negative segmented timeout", cfg: ASEConfig{InvokeTimeout: time.Second, SegmentedRequestTimeout: -time.Second, MaxConcurrentInvokes: 8}, transport: transport, wantErr: ErrInvalidASEConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewASE(tt.cfg, tt.transport)
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

func TestInvokeConfirmedRoundTrip(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	type callResult struct {
		confirm ConfirmICI
		err     error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		confirm, err := ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
			Destination: dst,
			Priority:    netprim.NetworkPriorityNormal,
			ServiceRequest: ConfirmedRequest{
				ServiceChoice: ServiceChoiceReadProperty,
				Payload:       []byte{0xAA},
			},
		})
		resultCh <- callResult{confirm: confirm, err: err}
	}()

	sent := <-transport.ch
	apduBytes := sent.packet.APDUBytes()
	decoded, err := decodeAPDU(apduBytes)
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}

	ackBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeComplexACK, InvokeID: decoded.InvokeID, ServiceChoice: ServiceChoiceReadProperty, Payload: []byte{0xBB}})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	ack, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, ackBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}
	if err := ase.OnInboundNPDU(context.Background(), dst, *ack); err != nil {
		t.Fatalf("OnInboundNPDU returned error: %v", err)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("InvokeConfirmed returned error: %v", result.err)
	}
	if result.confirm.Result != ConfirmResultPositiveAck {
		t.Fatalf("confirm result = %v, want %v", result.confirm.Result, ConfirmResultPositiveAck)
	}
	if result.confirm.ServiceResponse == nil || len(result.confirm.ServiceResponse.Payload) != 1 || result.confirm.ServiceResponse.Payload[0] != 0xBB {
		t.Fatalf("confirm payload = %v, want [187]", result.confirm.ServiceResponse)
	}
}

func TestInvokeConfirmedUnexpectedPeerDoesNotCompleteTransaction(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	otherPeer, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 243}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	type callResult struct {
		confirm ConfirmICI
		err     error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		confirm, err := ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
			Destination: dst,
			Priority:    netprim.NetworkPriorityNormal,
			ServiceRequest: ConfirmedRequest{
				ServiceChoice: ServiceChoiceReadProperty,
				Payload:       []byte{0xAA},
			},
		})
		resultCh <- callResult{confirm: confirm, err: err}
	}()

	sent := <-transport.ch
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}

	ackBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeComplexACK, InvokeID: decoded.InvokeID, ServiceChoice: ServiceChoiceReadProperty, Payload: []byte{0xBB}})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	ack, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, ackBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), otherPeer, *ack)
	if !errors.Is(err, ErrUnexpectedPDU) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, ErrUnexpectedPDU)
	}

	select {
	case result := <-resultCh:
		t.Fatalf("BeginConfirmedServiceRequest completed early: %#v", result)
	case <-time.After(20 * time.Millisecond):
	}

	if err := ase.OnInboundNPDU(context.Background(), dst, *ack); err != nil {
		t.Fatalf("OnInboundNPDU returned error: %v", err)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("BeginConfirmedServiceRequest returned error: %v", result.err)
	}
	if result.confirm.Result != ConfirmResultPositiveAck {
		t.Fatalf("Confirm.Result = %v, want %v", result.confirm.Result, ConfirmResultPositiveAck)
	}
}

func TestInvokeConfirmedUnexpectedServiceChoiceDoesNotCompleteTransaction(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	type callResult struct {
		confirm ConfirmICI
		err     error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		confirm, err := ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
			Destination: dst,
			Priority:    netprim.NetworkPriorityNormal,
			ServiceRequest: ConfirmedRequest{
				ServiceChoice: ServiceChoiceReadProperty,
				Payload:       []byte{0xAA},
			},
		})
		resultCh <- callResult{confirm: confirm, err: err}
	}()

	sent := <-transport.ch
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}

	wrongAckBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeComplexACK, InvokeID: decoded.InvokeID, ServiceChoice: ServiceChoiceWhoIs, Payload: []byte{0xBB}})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	wrongAck, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, wrongAckBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), dst, *wrongAck)
	if !errors.Is(err, ErrUnexpectedPDU) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, ErrUnexpectedPDU)
	}

	select {
	case result := <-resultCh:
		t.Fatalf("BeginConfirmedServiceRequest completed early: %#v", result)
	case <-time.After(20 * time.Millisecond):
	}

	ackBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeComplexACK, InvokeID: decoded.InvokeID, ServiceChoice: ServiceChoiceReadProperty, Payload: []byte{0xCC}})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	ack, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, ackBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	if err := ase.OnInboundNPDU(context.Background(), dst, *ack); err != nil {
		t.Fatalf("OnInboundNPDU returned error: %v", err)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("BeginConfirmedServiceRequest returned error: %v", result.err)
	}
	if result.confirm.Result != ConfirmResultPositiveAck {
		t.Fatalf("Confirm.Result = %v, want %v", result.confirm.Result, ConfirmResultPositiveAck)
	}
}

func TestInvokeConfirmedRejectFromExpectedPeerCompletesTransaction(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	type callResult struct {
		confirm ConfirmICI
		err     error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		confirm, err := ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
			Destination: dst,
			Priority:    netprim.NetworkPriorityNormal,
			ServiceRequest: ConfirmedRequest{
				ServiceChoice: ServiceChoiceReadProperty,
				Payload:       []byte{0xAA},
			},
		})
		resultCh <- callResult{confirm: confirm, err: err}
	}()

	sent := <-transport.ch
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}

	rejectBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeReject, InvokeID: decoded.InvokeID, Payload: []byte{0x01}})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	reject, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, rejectBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	if err := ase.OnInboundNPDU(context.Background(), dst, *reject); err != nil {
		t.Fatalf("OnInboundNPDU returned error: %v", err)
	}

	result := <-resultCh
	if !errors.Is(result.err, ErrRemoteReject) {
		t.Fatalf("BeginConfirmedServiceRequest error = %v, want %v", result.err, ErrRemoteReject)
	}
	if result.confirm.Result != ConfirmResultReject {
		t.Fatalf("Confirm.Result = %v, want %v", result.confirm.Result, ConfirmResultReject)
	}
}

func TestInvokeConfirmedTimeout(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: 30 * time.Millisecond, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	_, err = ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
		},
	})
	if !errors.Is(err, ErrAPDUTimeout) {
		t.Fatalf("InvokeConfirmed error = %v, want %v", err, ErrAPDUTimeout)
	}
}

func TestInvokeConfirmedRetriesThenTimeout(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: 10 * time.Millisecond, APDURetries: 2, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	_, err = ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0x01, 0x02},
		},
	})
	if !errors.Is(err, ErrAPDUTimeout) {
		t.Fatalf("BeginConfirmedServiceRequest error = %v, want %v", err, ErrAPDUTimeout)
	}

	transport.mu.Lock()
	sentCount := len(transport.sent)
	transport.mu.Unlock()
	if sentCount != 3 {
		t.Fatalf("SendNPDU call count = %d, want 3 (initial + 2 retries)", sentCount)
	}
}

func TestInboundConfirmedDispatch(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, indication ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		if got := indication.ServiceRequest.Payload; len(got) != 1 || got[0] != 0x10 {
			t.Fatalf("handler payload = %v, want [16]", got)
		}
		return ConfirmedResponseICI{
			Destination:     indication.Source,
			InvokeID:        indication.InvokeID,
			ServiceResponse: ServiceResult{Payload: []byte{0x20}},
		}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed returned error: %v", err)
	}

	src, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	requestBytes, err := encodeAPDU(outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  7,
		ServiceChoice:             ServiceChoiceReadProperty,
		MaxSegmentsAccepted:       MaxSegmentsUnspecified,
		MaxAPDULengthAccepted:     1476,
		SegmentedResponseAccepted: false,
		Payload:                   []byte{0x10},
	})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	request, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, requestBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}
	if err := ase.OnInboundNPDU(context.Background(), src, *request); err != nil {
		t.Fatalf("OnInboundNPDU returned error: %v", err)
	}

	response := <-transport.ch
	decoded, err := decodeAPDU(response.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}
	if decoded.Type != PDUTypeComplexACK {
		t.Fatalf("response pdu type = %v, want %v", decoded.Type, PDUTypeComplexACK)
	}
	if decoded.InvokeID != 7 {
		t.Fatalf("response invoke ID = %d, want 7", decoded.InvokeID)
	}
	if decoded.ServiceChoice != ServiceChoiceReadProperty {
		t.Fatalf("response service choice = %v, want %v", decoded.ServiceChoice, ServiceChoiceReadProperty)
	}
	if len(decoded.Payload) != 1 || decoded.Payload[0] != 0x20 {
		t.Fatalf("response payload = %v, want [0x20]", decoded.Payload)
	}
}

func TestInboundConfirmedHandlerErrorSendsError(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	handlerErr := errors.New("handler failed")
	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, indication ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{Destination: indication.Source, InvokeID: indication.InvokeID}, handlerErr
	}); err != nil {
		t.Fatalf("RegisterConfirmed returned error: %v", err)
	}

	src, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	requestBytes, err := encodeAPDU(outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  11,
		ServiceChoice:             ServiceChoiceReadProperty,
		MaxSegmentsAccepted:       MaxSegmentsUnspecified,
		MaxAPDULengthAccepted:     1476,
		SegmentedResponseAccepted: false,
		Payload:                   []byte{0x10},
	})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	request, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, requestBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), src, *request)
	if !errors.Is(err, handlerErr) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, handlerErr)
	}

	select {
	case response := <-transport.ch:
		decoded, decErr := decodeAPDU(response.packet.APDUBytes())
		if decErr != nil {
			t.Fatalf("decodeAPDU returned error: %v", decErr)
		}
		if decoded.Type != PDUTypeError {
			t.Fatalf("response pdu type = %v, want %v", decoded.Type, PDUTypeError)
		}
		if decoded.InvokeID != 11 {
			t.Fatalf("response invoke ID = %d, want 11", decoded.InvokeID)
		}
		if decoded.ServiceChoice != ServiceChoiceReadProperty {
			t.Fatalf("response service choice = %v, want %v", decoded.ServiceChoice, ServiceChoiceReadProperty)
		}
		if len(decoded.Payload) != 0 {
			t.Fatalf("response payload = %v, want empty", decoded.Payload)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for error response")
	}
}

func TestInboundConfirmedNoHandlerSendsReject(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	src, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	requestBytes, err := encodeAPDU(outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  12,
		ServiceChoice:             ServiceChoiceReadProperty,
		MaxSegmentsAccepted:       MaxSegmentsUnspecified,
		MaxAPDULengthAccepted:     1476,
		SegmentedResponseAccepted: false,
		Payload:                   []byte{0x10},
	})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	request, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, requestBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), src, *request)
	if !errors.Is(err, ErrHandlerNotFound) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, ErrHandlerNotFound)
	}

	select {
	case response := <-transport.ch:
		decoded, decErr := decodeAPDU(response.packet.APDUBytes())
		if decErr != nil {
			t.Fatalf("decodeAPDU returned error: %v", decErr)
		}
		if decoded.Type != PDUTypeReject {
			t.Fatalf("response pdu type = %v, want %v", decoded.Type, PDUTypeReject)
		}
		if decoded.InvokeID != 12 {
			t.Fatalf("response invoke ID = %d, want 12", decoded.InvokeID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for reject response")
	}
}

func TestInboundConfirmedOversizedResponseSendsAbort(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4, MaxAPDUSizeAccepted: 1}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, indication ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{
			Destination:     indication.Source,
			InvokeID:        indication.InvokeID,
			ServiceResponse: ServiceResult{Payload: []byte{0x20, 0x21}},
		}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed returned error: %v", err)
	}

	src, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	requestBytes, err := encodeAPDU(outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  13,
		ServiceChoice:             ServiceChoiceReadProperty,
		MaxSegmentsAccepted:       MaxSegmentsUnspecified,
		MaxAPDULengthAccepted:     1476,
		SegmentedResponseAccepted: false,
		Payload:                   []byte{0x10},
	})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	request, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, requestBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), src, *request)
	if !errors.Is(err, ErrSegmentationNotSupported) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, ErrSegmentationNotSupported)
	}

	select {
	case response := <-transport.ch:
		decoded, decErr := decodeAPDU(response.packet.APDUBytes())
		if decErr != nil {
			t.Fatalf("decodeAPDU returned error: %v", decErr)
		}
		if decoded.Type != PDUTypeAbort {
			t.Fatalf("response pdu type = %v, want %v", decoded.Type, PDUTypeAbort)
		}
		if decoded.InvokeID != 13 {
			t.Fatalf("response invoke ID = %d, want 13", decoded.InvokeID)
		}
		if !decoded.Server {
			t.Fatal("abort server flag = false, want true")
		}
		if len(decoded.Payload) != 1 || decoded.Payload[0] != byte(AbortReasonSegmentationNotSupported) {
			t.Fatalf("response payload = %v, want [%d]", decoded.Payload, byte(AbortReasonSegmentationNotSupported))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for abort response")
	}
}

func buildSegmentACKNPDU(t *testing.T, invokeID InvokeID, sequenceNumber uint8, negativeAck bool, server bool, windowSize uint8) npdu.NetworkLayerProtocolDataUnit {
	t.Helper()

	b0 := byte(PDUTypeSegmentACK << 4)
	if negativeAck {
		b0 |= 0x02
	}
	if server {
		b0 |= 0x01
	}

	pkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, []byte{b0, byte(invokeID), sequenceNumber, windowSize})
	if err != nil {
		t.Fatalf("buildSegmentACKNPDU: %v", err)
	}

	return *pkt
}

func TestASESegmentedConfirmedResponseHappyPath(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:                    time.Second,
		SegmentedRequestTimeout:          20 * time.Millisecond,
		APDURetries:                      1,
		MaxConcurrentInvokes:             4,
		Segmentation:                     SegmentationSupportBoth,
		MaxAPDUSizeAccepted:              50,
		SegmentedTimedOutCollectorPeriod: 5 * time.Millisecond,
		TimeoutGracePeriod:               100 * time.Millisecond,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	payload := make([]byte, 60)
	for i := range payload {
		payload[i] = byte(i)
	}

	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, indication ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{
			Destination:     indication.Source,
			InvokeID:        indication.InvokeID,
			ResponseType:    ConfirmedResponseTypeACK,
			ServiceResponse: ServiceResult{Payload: payload},
		}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed returned error: %v", err)
	}

	src, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	requestBytes, err := encodeAPDU(outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  41,
		ServiceChoice:             ServiceChoiceReadProperty,
		MaxSegmentsAccepted:       MaxSegments4,
		MaxAPDULengthAccepted:     50,
		SegmentedResponseAccepted: true,
		Payload:                   []byte{0x10},
	})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}
	request, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, requestBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	if err := ase.OnInboundNPDU(context.Background(), src, *request); err != nil {
		t.Fatalf("OnInboundNPDU returned error: %v", err)
	}

	seg0 := <-transport.ch
	decoded0, err := decodeAPDU(seg0.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(first segment) returned error: %v", err)
	}
	if decoded0.Type != PDUTypeComplexACK || !decoded0.SegmentedMessage {
		t.Fatalf("first response = %#v, want segmented ComplexACK", decoded0)
	}
	if decoded0.SequenceNumber != 0 {
		t.Fatalf("first segment sequence = %d, want 0", decoded0.SequenceNumber)
	}
	if !decoded0.MoreFollows {
		t.Fatal("first segment MoreFollows = false, want true")
	}
	if len(decoded0.Payload) != 45 {
		t.Fatalf("first segment payload length = %d, want 45", len(decoded0.Payload))
	}

	ack0 := buildSegmentACKNPDU(t, 41, 0, false, false, 1)
	if err := ase.OnInboundNPDU(context.Background(), src, ack0); err != nil {
		t.Fatalf("OnInboundNPDU(first SegmentACK) returned error: %v", err)
	}

	seg1 := <-transport.ch
	decoded1, err := decodeAPDU(seg1.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(second segment) returned error: %v", err)
	}
	if decoded1.Type != PDUTypeComplexACK || !decoded1.SegmentedMessage {
		t.Fatalf("second response = %#v, want segmented ComplexACK", decoded1)
	}
	if decoded1.SequenceNumber != 1 {
		t.Fatalf("second segment sequence = %d, want 1", decoded1.SequenceNumber)
	}
	if decoded1.MoreFollows {
		t.Fatal("second segment MoreFollows = true, want false")
	}
	if len(decoded1.Payload) != 15 {
		t.Fatalf("second segment payload length = %d, want 15", len(decoded1.Payload))
	}

	ack1 := buildSegmentACKNPDU(t, 41, 1, false, false, 1)
	if err := ase.OnInboundNPDU(context.Background(), src, ack1); err != nil {
		t.Fatalf("OnInboundNPDU(final SegmentACK) returned error: %v", err)
	}

	impl, ok := ase.(*aseImpl)
	if !ok {
		t.Fatal("expected concrete aseImpl for test assertions")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if len(impl.outboundSegmentedServerEntries) != 0 {
		t.Fatalf("outboundSegmentedServerEntries = %d, want 0 after final SegmentACK", len(impl.outboundSegmentedServerEntries))
	}
}

func TestASESegmentedConfirmedResponseInitialWindowSendsMultipleSegments(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:                    time.Second,
		SegmentedRequestTimeout:          20 * time.Millisecond,
		APDURetries:                      1,
		MaxConcurrentInvokes:             4,
		Segmentation:                     SegmentationSupportBoth,
		PreferredWindowSize:              2,
		MaxAPDUSizeAccepted:              50,
		SegmentedTimedOutCollectorPeriod: 5 * time.Millisecond,
		TimeoutGracePeriod:               100 * time.Millisecond,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	payload := make([]byte, 120)
	for i := range payload {
		payload[i] = byte(i)
	}

	err = ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, indication ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{
			Destination:     indication.Source,
			InvokeID:        indication.InvokeID,
			ResponseType:    ConfirmedResponseTypeACK,
			ServiceResponse: ServiceResult{Payload: payload},
		}, nil
	})
	if err != nil {
		t.Fatalf("RegisterConfirmed returned error: %v", err)
	}

	src, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	requestBytes, err := encodeAPDU(outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  42,
		ServiceChoice:             ServiceChoiceReadProperty,
		MaxSegmentsAccepted:       MaxSegments4,
		MaxAPDULengthAccepted:     50,
		SegmentedResponseAccepted: true,
		Payload:                   []byte{0x10},
	})
	if err != nil {
		t.Fatalf("encodeAPDU returned error: %v", err)
	}

	request, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, requestBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), src, *request)
	if err != nil {
		t.Fatalf("OnInboundNPDU returned error: %v", err)
	}

	seg0 := <-transport.ch
	decoded0, err := decodeAPDU(seg0.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(first segment) returned error: %v", err)
	}

	if decoded0.Type != PDUTypeComplexACK || !decoded0.SegmentedMessage || decoded0.SequenceNumber != 0 {
		t.Fatalf("first response = %#v, want segmented ComplexACK seq 0", decoded0)
	}

	seg1 := <-transport.ch
	decoded1, err := decodeAPDU(seg1.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(second segment) returned error: %v", err)
	}

	if decoded1.Type != PDUTypeComplexACK || !decoded1.SegmentedMessage || decoded1.SequenceNumber != 1 {
		t.Fatalf("second response = %#v, want segmented ComplexACK seq 1", decoded1)
	}

	ackWindow := buildSegmentACKNPDU(t, 42, 1, false, false, 2)
	if err := ase.OnInboundNPDU(context.Background(), src, ackWindow); err != nil {
		t.Fatalf("OnInboundNPDU(window SegmentACK) returned error: %v", err)
	}

	seg2 := <-transport.ch
	decoded2, err := decodeAPDU(seg2.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(final segment) returned error: %v", err)
	}

	if decoded2.Type != PDUTypeComplexACK || !decoded2.SegmentedMessage || decoded2.SequenceNumber != 2 {
		t.Fatalf("final response = %#v, want segmented ComplexACK seq 2", decoded2)
	}

	if decoded2.MoreFollows {
		t.Fatal("final segment MoreFollows = true, want false")
	}

	ackFinal := buildSegmentACKNPDU(t, 42, 2, false, false, 2)
	err = ase.OnInboundNPDU(context.Background(), src, ackFinal)
	if err != nil {
		t.Fatalf("OnInboundNPDU(final SegmentACK) returned error: %v", err)
	}
}

func TestSendUnconfirmed(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	if err := ase.SendUnconfirmed(context.Background(), UnconfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityUrgent,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: ServiceChoiceWhoIs,
			Payload:       []byte{0x01, 0x02},
		},
	}); err != nil {
		t.Fatalf("SendUnconfirmed returned error: %v", err)
	}

	sent := <-transport.ch
	if sent.packet.Priority() != netprim.NetworkPriorityUrgent {
		t.Fatalf("npdu priority = %v, want %v", sent.packet.Priority(), netprim.NetworkPriorityUrgent)
	}
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU returned error: %v", err)
	}
	if decoded.Type != PDUTypeUnconfirmedRequest {
		t.Fatalf("pdu type = %v, want %v", decoded.Type, PDUTypeUnconfirmedRequest)
	}
	if decoded.ServiceChoice != ServiceChoiceWhoIs {
		t.Fatalf("service choice = %v, want %v", decoded.ServiceChoice, ServiceChoiceWhoIs)
	}
}

func TestBeginConfirmedServiceRequestAPDUSizeBoundary(t *testing.T) {
	transport := newTestNPDUTransport()
	transport.err = errors.New("write failed")
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		MaxAPDUSizeAccepted:  confirmedRequestAPDUHeaderLength + 1,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	_, err = ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0x01},
		},
	})
	if !errors.Is(err, ErrTransportFailure) {
		t.Fatalf("BeginConfirmedServiceRequest error = %v, want %v", err, ErrTransportFailure)
	}

	_, err = ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0x01, 0x02},
		},
	})
	if !errors.Is(err, ErrSegmentationNotSupported) {
		t.Fatalf("BeginConfirmedServiceRequest error = %v, want %v", err, ErrSegmentationNotSupported)
	}
}

func TestSendUnconfirmedAPDUSizeBoundary(t *testing.T) {
	transport := newTestNPDUTransport()
	transport.err = errors.New("write failed")
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		MaxAPDUSizeAccepted:  unconfirmedRequestAPDUHeaderLength + 1,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	err = ase.SendUnconfirmed(context.Background(), UnconfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: ServiceChoiceWhoIs,
			Payload:       []byte{0x01},
		},
	})
	if !errors.Is(err, ErrTransportFailure) {
		t.Fatalf("SendUnconfirmed error = %v, want %v", err, ErrTransportFailure)
	}

	err = ase.SendUnconfirmed(context.Background(), UnconfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: ServiceChoiceWhoIs,
			Payload:       []byte{0x01, 0x02},
		},
	})
	if !errors.Is(err, ErrSegmentationNotSupported) {
		t.Fatalf("SendUnconfirmed error = %v, want %v", err, ErrSegmentationNotSupported)
	}
}

func TestBeginConfirmedServiceRequestRejectsUnconfirmedServiceChoice(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	_, err = ase.BeginConfirmedServiceRequest(context.Background(), ConfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceWhoIs,
			Payload:       []byte{0x01},
		},
	})
	if !errors.Is(err, ErrInvalidServiceChoice) {
		t.Fatalf("BeginConfirmedServiceRequest error = %v, want %v", err, ErrInvalidServiceChoice)
	}
}

func TestSendUnconfirmedRejectsConfirmedServiceChoice(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	dst, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress returned error: %v", err)
	}

	err = ase.SendUnconfirmed(context.Background(), UnconfirmedRequestICI{
		Destination: dst,
		Priority:    netprim.NetworkPriorityNormal,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0x01},
		},
	})
	if !errors.Is(err, ErrInvalidServiceChoice) {
		t.Fatalf("SendUnconfirmed error = %v, want %v", err, ErrInvalidServiceChoice)
	}
}

func TestRegisterConfirmedRejectsUnconfirmedServiceChoice(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	err = ase.RegisterConfirmed(ServiceChoiceWhoIs, func(_ context.Context, _ ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{}, nil
	})
	if !errors.Is(err, ErrInvalidServiceChoice) {
		t.Fatalf("RegisterConfirmed error = %v, want %v", err, ErrInvalidServiceChoice)
	}
}

func TestRegisterUnconfirmedRejectsConfirmedServiceChoice(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}

	err = ase.RegisterUnconfirmed(ServiceChoiceReadProperty, func(_ context.Context, _ UnconfirmedIndicationICI) error {
		return nil
	})
	if !errors.Is(err, ErrInvalidServiceChoice) {
		t.Fatalf("RegisterUnconfirmed error = %v, want %v", err, ErrInvalidServiceChoice)
	}
}

func TestOnInboundNPDUNetworkLayerMessageRejected(t *testing.T) {
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, newTestNPDUTransport())
	if err != nil {
		t.Fatalf("NewASE returned error: %v", err)
	}
	src, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})

	packet, err := npdu.NewNetworkLayerMessage(0x04, []byte{0x00, 0x01}, netprim.NetworkPriorityNormal)
	if err != nil {
		t.Fatalf("NewNetworkLayerMessage returned error: %v", err)
	}

	err = ase.OnInboundNPDU(context.Background(), src, *packet)
	if !errors.Is(err, ErrInvalidPDUType) {
		t.Fatalf("OnInboundNPDU error = %v, want %v", err, ErrInvalidPDUType)
	}
}

// buildSegmentedConfirmedRequestNPDU creates a local NPDU wrapping a
// segmented confirmed-request APDU segment.
func buildSegmentedConfirmedRequestNPDU(
	t *testing.T,
	invokeID InvokeID,
	seq uint8,
	window uint8,
	moreFollows bool,
	serviceChoice ServiceChoice,
	payload []byte,
) npdu.NetworkLayerProtocolDataUnit {
	t.Helper()

	b0 := byte(PDUTypeConfirmedRequest << 4)
	b0 |= confirmedRequestFlagSegmentedMessage
	if moreFollows {
		b0 |= confirmedRequestFlagMoreFollows
	}

	var raw []byte
	// MaxSegments=more-than-64 (0x07), MaxAPDU=1476 (0x05)
	raw = append(raw, b0, 0x75, byte(invokeID), seq, window)
	if seq == 0 {
		raw = append(raw, byte(serviceChoice))
	}
	raw = append(raw, payload...)

	pkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, raw)
	if err != nil {
		t.Fatalf("buildSegmentedConfirmedRequestNPDU: %v", err)
	}
	return *pkt
}

// TestASESegmentedConfirmedRequestHappyPath feeds a two-segment confirmed
// request to an ASE that supports receiving segmented messages and verifies that
// the handler receives the fully assembled payload and that Segment-ACKs are
// sent after each window.
func TestASESegmentedConfirmedRequestHappyPath(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		Segmentation:         SegmentationSupportBoth,
		MaxAPDUSizeAccepted:  maxApduLengthAccepted1476Bytes,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	src := netprim.Address{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1234)}
	var receivedPayload []byte

	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, ind ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		receivedPayload = ind.ServiceRequest.Payload
		return ConfirmedResponseICI{
			Destination:     ind.Source,
			InvokeID:        ind.InvokeID,
			ServiceResponse: ServiceResult{Payload: []byte{0xFF}},
		}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed: %v", err)
	}

	// Segment 0 (window=1, moreFollows=true).
	pkt0 := buildSegmentedConfirmedRequestNPDU(t, 7, 0, 1, true, ServiceChoiceReadProperty, []byte{0xAA})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(seg0): %v", err)
	}

	// ASE must have sent a positive Segment-ACK for seq 0.
	ack0 := <-transport.ch
	ack0Decoded, err := decodeAPDU(ack0.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(seg-ack 0): %v", err)
	}
	if ack0Decoded.Type != PDUTypeSegmentACK {
		t.Fatalf("expected Segment-ACK, got %v", ack0Decoded.Type)
	}
	if ack0Decoded.SequenceNumber != 0 {
		t.Fatalf("Segment-ACK seqNum = %d, want 0", ack0Decoded.SequenceNumber)
	}

	// Segment 1 (last, moreFollows=false).
	pkt1 := buildSegmentedConfirmedRequestNPDU(t, 7, 1, 1, false, 0, []byte{0xBB})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt1); err != nil {
		t.Fatalf("OnInboundNPDU(seg1): %v", err)
	}

	// ASE must have sent a Segment-ACK for seq 1 and then the handler response.
	ack1 := <-transport.ch
	ack1Decoded, err := decodeAPDU(ack1.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(seg-ack 1): %v", err)
	}
	if ack1Decoded.Type != PDUTypeSegmentACK {
		t.Fatalf("expected Segment-ACK for seq1, got %v", ack1Decoded.Type)
	}
	if ack1Decoded.SequenceNumber != 1 {
		t.Fatalf("Segment-ACK seqNum = %d, want 1", ack1Decoded.SequenceNumber)
	}

	// Handler response (ComplexACK).
	resp := <-transport.ch
	respDecoded, err := decodeAPDU(resp.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(response): %v", err)
	}
	if respDecoded.Type != PDUTypeComplexACK {
		t.Fatalf("response type = %v, want complex-ack", respDecoded.Type)
	}

	// Verify assembled payload.
	if len(receivedPayload) != 2 || receivedPayload[0] != 0xAA || receivedPayload[1] != 0xBB {
		t.Fatalf("handler payload = %v, want [0xAA 0xBB]", receivedPayload)
	}
}

func TestASESegmentedConfirmedRequestNegotiatesWindowSize(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		Segmentation:         SegmentationSupportBoth,
		PreferredWindowSize:  2,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	src := netprim.Address{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1234)}
	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, ind ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{Destination: ind.Source, InvokeID: ind.InvokeID}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed: %v", err)
	}

	// Proposed window is 5, local preferred is 2 -> first segment must be mid-window (no ACK yet).
	pkt0 := buildSegmentedConfirmedRequestNPDU(t, 9, 0, 5, true, ServiceChoiceReadProperty, []byte{0x01})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(seg0): %v", err)
	}
	select {
	case sent := <-transport.ch:
		t.Fatalf("unexpected outbound APDU after first segment: %v", sent.packet.APDUBytes())
	default:
	}

	// Second segment fills negotiated window -> Segment-ACK with actualWindowSize=2.
	pkt1 := buildSegmentedConfirmedRequestNPDU(t, 9, 1, 5, true, 0, []byte{0x02})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt1); err != nil {
		t.Fatalf("OnInboundNPDU(seg1): %v", err)
	}
	ack := <-transport.ch
	ackDecoded, err := decodeAPDU(ack.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(seg-ack): %v", err)
	}
	if ackDecoded.Type != PDUTypeSegmentACK {
		t.Fatalf("expected Segment-ACK, got %v", ackDecoded.Type)
	}
	if ackDecoded.ProposedWindowSize != 2 {
		t.Fatalf("actual window size = %d, want 2", ackDecoded.ProposedWindowSize)
	}
}

// TestASESegmentedConfirmedRequestNotSupported verifies that an ASE with
// SegmentationSupportNo sends an Abort and returns ErrSegmentationNotSupported
// when it receives the first segment of a segmented confirmed request.
func TestASESegmentedConfirmedRequestNotSupported(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		Segmentation:         SegmentationSupportNo,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	src := netprim.Address{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1234)}
	pkt := buildSegmentedConfirmedRequestNPDU(t, 1, 0, 1, true, ServiceChoiceReadProperty, []byte{0x01})

	err = ase.OnInboundNPDU(context.Background(), src, pkt)
	if !errors.Is(err, ErrSegmentationNotSupported) {
		t.Fatalf("OnInboundNPDU error = %v, want ErrSegmentationNotSupported", err)
	}

	// An Abort PDU should have been sent.
	select {
	case sent := <-transport.ch:
		decoded, err := decodeAPDU(sent.packet.APDUBytes())
		if err != nil {
			t.Fatalf("decodeAPDU(abort): %v", err)
		}
		if decoded.Type != PDUTypeAbort {
			t.Fatalf("expected Abort PDU, got %v", decoded.Type)
		}
	default:
		t.Fatal("expected Abort PDU to be sent but transport channel is empty")
	}

	impl, ok := ase.(*aseImpl)
	if !ok {
		t.Fatal("expected concrete aseImpl for test assertions")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if len(impl.inboundSegmentedServerEntries) != 0 {
		t.Fatalf("inboundSegmentedServers = %d, want 0", len(impl.inboundSegmentedServerEntries))
	}
}

// TestASESegmentedConfirmedRequestOutOfOrder verifies that out-of-order
// continuation segments are NAKed immediately and completed via retransmit.
func TestASESegmentedConfirmedRequestOutOfOrder(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		Segmentation:         SegmentationSupportBoth,
		PreferredWindowSize:  2,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	src := netprim.Address{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1234)}

	var receivedPayload []byte
	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, ind ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		receivedPayload = ind.ServiceRequest.Payload
		return ConfirmedResponseICI{Destination: ind.Source, InvokeID: ind.InvokeID}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed: %v", err)
	}

	// First segment (window=2).
	pkt0 := buildSegmentedConfirmedRequestNPDU(t, 3, 0, 2, true, ServiceChoiceReadProperty, []byte{0x01})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(seg0): %v", err)
	}
	select {
	case sent := <-transport.ch:
		t.Fatalf("unexpected outbound APDU after seg0 in window=2: %v", sent.packet.APDUBytes())
	default:
	}

	// Out-of-order segment: seq=2 arrives before seq=1 and is NAKed immediately.
	pkt2 := buildSegmentedConfirmedRequestNPDU(t, 3, 2, 2, false, 0, []byte{0x03})
	err = ase.OnInboundNPDU(context.Background(), src, pkt2)
	if err != nil {
		t.Fatalf("OnInboundNPDU(out-of-order in-window) error = %v", err)
	}
	nak := <-transport.ch
	nakDecoded, err := decodeAPDU(nak.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(seg-nak): %v", err)
	}

	if nakDecoded.Type != PDUTypeSegmentACK {
		t.Fatalf("expected Segment-ACK, got %v", nakDecoded.Type)
	}

	nakRaw := nak.packet.APDUBytes()
	if len(nakRaw) < 1 {
		t.Fatalf("seg-nak raw bytes = %v, want at least 1 byte", nakRaw)
	}

	if nakRaw[0]&0x02 == 0 {
		t.Fatal("expected negative Segment-ACK for out-of-order segment")
	}

	if nakDecoded.SequenceNumber != 0 {
		t.Fatalf("Segment-NAK seqNum = %d, want 0", nakDecoded.SequenceNumber)
	}

	// Missing seq=1 arrives and is ACKed as normal progress.
	pkt1 := buildSegmentedConfirmedRequestNPDU(t, 3, 1, 2, true, 0, []byte{0x02})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt1); err != nil {
		t.Fatalf("OnInboundNPDU(seg1): %v", err)
	}

	ack := <-transport.ch
	ackDecoded, err := decodeAPDU(ack.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(seg-ack): %v", err)
	}
	if ackDecoded.Type != PDUTypeSegmentACK {
		t.Fatalf("expected Segment-ACK, got %v", ackDecoded.Type)
	}
	ackRaw := ack.packet.APDUBytes()
	if len(ackRaw) < 1 {
		t.Fatalf("seg-ack raw bytes = %v, want at least 1 byte", ackRaw)
	}
	if ackRaw[0]&0x02 != 0 {
		t.Fatal("expected positive Segment-ACK after reordered delivery")
	}
	if ackDecoded.SequenceNumber != 1 {
		t.Fatalf("Segment-ACK seqNum = %d, want 1", ackDecoded.SequenceNumber)
	}

	// Sender retransmits seq=2 after receiving NAK.
	pkt2Retransmit := buildSegmentedConfirmedRequestNPDU(t, 3, 2, 2, false, 0, []byte{0x03})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt2Retransmit); err != nil {
		t.Fatalf("OnInboundNPDU(retransmitted seg2): %v", err)
	}

	ackLast := <-transport.ch
	ackLastDecoded, err := decodeAPDU(ackLast.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(final seg-ack): %v", err)
	}

	if ackLastDecoded.Type != PDUTypeSegmentACK {
		t.Fatalf("expected final Segment-ACK, got %v", ackLastDecoded.Type)
	}

	if ackLastDecoded.SequenceNumber != 2 {
		t.Fatalf("final Segment-ACK seqNum = %d, want 2", ackLastDecoded.SequenceNumber)
	}

	resp := <-transport.ch
	respDecoded, err := decodeAPDU(resp.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(response): %v", err)
	}
	if respDecoded.Type != PDUTypeSimpleACK {
		t.Fatalf("response type = %v, want simple-ack", respDecoded.Type)
	}

	if len(receivedPayload) != 3 || receivedPayload[0] != 0x01 || receivedPayload[1] != 0x02 || receivedPayload[2] != 0x03 {
		t.Fatalf("handler payload = %v, want [0x01 0x02 0x03]", receivedPayload)
	}
}

func TestASESegmentedConfirmedRequestDuplicateFirstSegmentReACKsWithoutRedispatch(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		Segmentation:         SegmentationSupportBoth,
		PreferredWindowSize:  1,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	src := netprim.Address{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1234)}
	handlerCalls := 0

	err = ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, ind ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		handlerCalls++
		return ConfirmedResponseICI{Destination: ind.Source, InvokeID: ind.InvokeID}, nil
	})
	if err != nil {
		t.Fatalf("RegisterConfirmed: %v", err)
	}

	pkt0 := buildSegmentedConfirmedRequestNPDU(t, 23, 0, 1, true, ServiceChoiceReadProperty, []byte{0xAA})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(seg0): %v", err)
	}

	ack0 := <-transport.ch
	ack0Decoded, err := decodeAPDU(ack0.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(seg-ack 0): %v", err)
	}

	if ack0Decoded.Type != PDUTypeSegmentACK || ack0Decoded.SequenceNumber != 0 {
		t.Fatalf("first ack = %#v, want Segment-ACK seq 0", ack0Decoded)
	}

	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(duplicate seg0): %v", err)
	}

	dupAck := <-transport.ch
	dupAckDecoded, err := decodeAPDU(dupAck.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(duplicate seg-ack): %v", err)
	}

	if dupAckDecoded.Type != PDUTypeSegmentACK || dupAckDecoded.SequenceNumber != 0 {
		t.Fatalf("duplicate ack = %#v, want Segment-ACK seq 0", dupAckDecoded)
	}

	if handlerCalls != 0 {
		t.Fatalf("handlerCalls = %d, want 0 before final segment", handlerCalls)
	}

	pkt1 := buildSegmentedConfirmedRequestNPDU(t, 23, 1, 1, false, 0, []byte{0xBB})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt1); err != nil {
		t.Fatalf("OnInboundNPDU(seg1): %v", err)
	}

	<-transport.ch // final Segment-ACK
	<-transport.ch // handler response

	if handlerCalls != 1 {
		t.Fatalf("handlerCalls = %d, want 1 after final segment", handlerCalls)
	}
}

func TestASESegmentedConfirmedRequestExceedingMaxDuplicatesSendsAbort(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:        time.Second,
		MaxConcurrentInvokes: 4,
		Segmentation:         SegmentationSupportBoth,
		PreferredWindowSize:  1,
		MaxSegmentDuplicates: 1,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	src := netprim.Address{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1234)}
	handlerCalls := 0
	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, ind ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		handlerCalls++
		return ConfirmedResponseICI{Destination: ind.Source, InvokeID: ind.InvokeID}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed: %v", err)
	}

	pkt0 := buildSegmentedConfirmedRequestNPDU(t, 24, 0, 1, true, ServiceChoiceReadProperty, []byte{0xAA})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(seg0): %v", err)
	}

	<-transport.ch // initial Segment-ACK

	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(first duplicate seg0): %v", err)
	}

	<-transport.ch // duplicate Segment-ACK

	err = ase.OnInboundNPDU(context.Background(), src, pkt0)
	if !errors.Is(err, ErrSegmentationNotSupported) {
		t.Fatalf("OnInboundNPDU(second duplicate seg0) error = %v, want %v", err, ErrSegmentationNotSupported)
	}

	abort := <-transport.ch
	abortDecoded, err := decodeAPDU(abort.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU(abort): %v", err)
	}

	if abortDecoded.Type != PDUTypeAbort {
		t.Fatalf("abort type = %v, want %v", abortDecoded.Type, PDUTypeAbort)
	}

	if handlerCalls != 0 {
		t.Fatalf("handlerCalls = %d, want 0", handlerCalls)
	}
}

func TestASESegmentedConfirmedRequestTimeout(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:                    time.Second,
		SegmentedRequestTimeout:          10 * time.Millisecond,
		APDURetries:                      0,
		MaxConcurrentInvokes:             4,
		Segmentation:                     SegmentationSupportBoth,
		PreferredWindowSize:              1,
		MaxAPDUSizeAccepted:              0,
		SegmentedTimedOutCollectorPeriod: 500 * time.Millisecond,
		TimeoutGracePeriod:               2 * time.Second,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	src := netprim.Address{AddrPort: netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 1234)}
	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(_ context.Context, ind ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		return ConfirmedResponseICI{Destination: ind.Source, InvokeID: ind.InvokeID}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed: %v", err)
	}

	// Start segmented transaction.
	pkt0 := buildSegmentedConfirmedRequestNPDU(t, 15, 0, 1, true, ServiceChoiceReadProperty, []byte{0x01})
	if err := ase.OnInboundNPDU(context.Background(), src, pkt0); err != nil {
		t.Fatalf("OnInboundNPDU(seg0): %v", err)
	}
	<-transport.ch // positive Segment-ACK for first segment

	// Let segmented receive deadline expire before next segment arrives.
	time.Sleep(25 * time.Millisecond)

	pkt1 := buildSegmentedConfirmedRequestNPDU(t, 15, 1, 1, false, 0, []byte{0x02})
	err = ase.OnInboundNPDU(context.Background(), src, pkt1)
	if !errors.Is(err, ErrAPDUTimeout) {
		t.Fatalf("OnInboundNPDU(expired continuation) error = %v, want %v", err, ErrAPDUTimeout)
	}

	// Timeout handling should emit Abort.
	select {
	case sent := <-transport.ch:
		decoded, decErr := decodeAPDU(sent.packet.APDUBytes())
		if decErr != nil {
			t.Fatalf("decodeAPDU(abort): %v", decErr)
		}
		if decoded.Type != PDUTypeAbort {
			t.Fatalf("timeout response pdu type = %v, want %v", decoded.Type, PDUTypeAbort)
		}
	default:
		t.Fatal("expected Abort APDU on segmented request timeout")
	}

	impl, ok := ase.(*aseImpl)
	if !ok {
		t.Fatal("expected concrete aseImpl for test assertions")
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()
	if len(impl.inboundSegmentedServerEntries) != 0 {
		t.Fatalf("inboundSegmentedServers = %d, want 0 after timeout cleanup", len(impl.inboundSegmentedServerEntries))
	}
}

func TestNewASEDefaultsPreferredWindowSize(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4, PreferredWindowSize: 0}, transport)
	if err != nil {
		t.Fatalf("NewASE error = %v, want nil", err)
	}
	defer ase.Close()

	impl, ok := ase.(*aseImpl)
	if !ok {
		t.Fatal("expected concrete aseImpl")
	}
	if impl.cfg.PreferredWindowSize != 1 {
		t.Fatalf("PreferredWindowSize = %d, want 1", impl.cfg.PreferredWindowSize)
	}
}

func TestNewASEDefaultsSegmentedTimedOutCollectorPeriod(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: 500 * time.Millisecond, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE error = %v, want nil", err)
	}
	defer ase.Close()

	impl, ok := ase.(*aseImpl)
	if !ok {
		t.Fatal("expected concrete aseImpl")
	}
	if impl.cfg.SegmentedTimedOutCollectorPeriod != 500*time.Millisecond {
		t.Fatalf("SegmentedTimedOutCollectorPeriod = %v, want %v", impl.cfg.SegmentedTimedOutCollectorPeriod, 500*time.Millisecond)
	}
}

func TestASECloseStopsTimedOutCollector(t *testing.T) {
	transport := newTestNPDUTransport()
	// Use a very long collector period so that if the goroutine were spinning on
	// time.Sleep it would not exit within the test deadline.
	ase, err := NewASE(ASEConfig{
		InvokeTimeout:                    time.Second,
		MaxConcurrentInvokes:             4,
		SegmentedTimedOutCollectorPeriod: 30 * time.Second,
	}, transport)
	if err != nil {
		t.Fatalf("NewASE error = %v", err)
	}

	stopped := make(chan struct{})
	impl := ase.(*aseImpl)
	go func() {
		// Wait for stopCh to be closed — that is the signal the collector sees.
		<-impl.stopCh
		close(stopped)
	}()

	if err := ase.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case <-stopped:
		// OK — stopCh was closed promptly
	case <-time.After(time.Second):
		t.Fatal("stopCh was not closed within 1s after Close()")
	}
}

func TestASEDuplicateConfirmedRequestDropped(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE error = %v", err)
	}
	defer ase.Close()

	src, err := netprim.NewAddress(netprim.LocalNetwork, binary.BigEndian.AppendUint16([]byte{192, 168, 8, 237}, 47808))
	if err != nil {
		t.Fatalf("NewAddress error = %v", err)
	}

	handlerEntered := make(chan struct{})
	handlerUnblock := make(chan struct{})
	var invokeCount int

	if err := ase.RegisterConfirmed(ServiceChoiceReadProperty, func(ctx context.Context, ind ConfirmedIndicationICI) (ConfirmedResponseICI, error) {
		invokeCount++
		close(handlerEntered) // signal test that handler is running
		<-handlerUnblock      // block until test sends the duplicate
		return ConfirmedResponseICI{
			InvokeID:    ind.InvokeID,
			Destination: ind.Source,
		}, nil
	}); err != nil {
		t.Fatalf("RegisterConfirmed error = %v", err)
	}

	// Build a confirmed-request APDU.
	reqBytes, err := encodeAPDU(outboundAPDU{
		Type:          PDUTypeConfirmedRequest,
		InvokeID:      42,
		ServiceChoice: ServiceChoiceReadProperty,
	})
	if err != nil {
		t.Fatalf("encodeAPDU error = %v", err)
	}
	pkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, true, reqBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU error = %v", err)
	}

	// Dispatch the first request; the handler blocks.
	go func() {
		_ = ase.OnInboundNPDU(context.Background(), src, *pkt)
	}()

	// Wait until the handler is in-flight.
	<-handlerEntered

	// Send the duplicate — should be dropped silently.
	if err := ase.OnInboundNPDU(context.Background(), src, *pkt); err != nil {
		t.Fatalf("OnInboundNPDU (duplicate) error = %v, want nil", err)
	}

	// Unblock the original handler.
	close(handlerUnblock)

	// Drain the response sent by the handler.
	select {
	case <-transport.ch:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for handler response")
	}

	if invokeCount != 1 {
		t.Fatalf("handler invoked %d times, want 1", invokeCount)
	}
}
