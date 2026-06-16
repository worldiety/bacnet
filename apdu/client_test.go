package apdu

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.wdy.de/bacnet/common/netprim"
	"go.wdy.de/bacnet/common/types"
	bacencoding "go.wdy.de/bacnet/encoding"
	"go.wdy.de/bacnet/npdu"
)

func TestNewClient(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, err := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	if err != nil {
		t.Fatalf("NewASE: %v", err)
	}

	tests := []struct {
		name    string
		ase     ASE
		cfg     ClientConfig
		wantErr error
	}{
		{name: "nil ASE", ase: nil, wantErr: ErrNilASE},
		{name: "invalid max segments accepted", ase: ase, cfg: ClientConfig{MaxSegmentsAccepted: 9}, wantErr: ErrInvalidASEConfig},
		{name: "valid defaults", ase: ase, cfg: ClientConfig{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.ase, tt.cfg)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestWhoIsValidation(t *testing.T) {
	one, _ := types.NewDeviceInstance(1)
	two, _ := types.NewDeviceInstance(2)

	tests := []struct {
		name    string
		req     WhoIsRequest
		wantErr error
	}{
		{name: "both nil valid", req: WhoIsRequest{}},
		{name: "only low set invalid", req: WhoIsRequest{LowLimit: &one}, wantErr: ErrEncodeFailure},
		{name: "low > high invalid", req: WhoIsRequest{LowLimit: &two, HighLimit: &one}, wantErr: ErrEncodeFailure},
		{name: "range valid", req: WhoIsRequest{LowLimit: &one, HighLimit: &two}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWhoIsRequest(tt.req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestWhoHasValidation(t *testing.T) {
	one, _ := types.NewDeviceInstance(1)
	two, _ := types.NewDeviceInstance(2)
	objID, _ := types.NewObjectIdentifier(types.ObjectTypeAnalogInput, 11)

	name := "AI-11"
	nonASCII := "Raum-ä"

	tests := []struct {
		name    string
		req     WhoHasRequest
		wantErr error
	}{
		{name: "valid object identifier", req: WhoHasRequest{ObjectIdentifier: &objID}},
		{name: "valid object name", req: WhoHasRequest{ObjectName: &name}},
		{name: "limits mismatch", req: WhoHasRequest{LowLimit: &one, ObjectIdentifier: &objID}, wantErr: ErrEncodeFailure},
		{name: "limits inverted", req: WhoHasRequest{LowLimit: &two, HighLimit: &one, ObjectIdentifier: &objID}, wantErr: ErrEncodeFailure},
		{name: "missing object specifier", req: WhoHasRequest{}, wantErr: ErrEncodeFailure},
		{name: "both object specifiers", req: WhoHasRequest{ObjectIdentifier: &objID, ObjectName: &name}, wantErr: ErrEncodeFailure},
		{name: "empty object name", req: WhoHasRequest{ObjectName: func() *string { v := ""; return &v }()}, wantErr: ErrEncodeFailure},
		{name: "non-ascii object name", req: WhoHasRequest{ObjectName: &nonASCII}, wantErr: ErrEncodeFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWhoHasRequest(tt.req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientWhoIs(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})

	if err := client.WhoIs(context.Background(), dst, WhoIsRequest{}); err != nil {
		t.Fatalf("WhoIs (all devices): %v", err)
	}

	sent := <-transport.ch
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}
	if decoded.Type != PDUTypeUnconfirmedRequest {
		t.Fatalf("type = %v, want %v", decoded.Type, PDUTypeUnconfirmedRequest)
	}
	if decoded.ServiceChoice != ServiceChoiceWhoIs {
		t.Fatalf("service = %v, want %v", decoded.ServiceChoice, ServiceChoiceWhoIs)
	}
	if len(decoded.Payload) != 0 {
		t.Fatalf("payload len = %d, want 0", len(decoded.Payload))
	}

	low, _ := types.NewDeviceInstance(1)
	high, _ := types.NewDeviceInstance(1024)
	rangeReq := WhoIsRequest{LowLimit: &low, HighLimit: &high}

	if err := client.WhoIs(context.Background(), dst, rangeReq); err != nil {
		t.Fatalf("WhoIs (range): %v", err)
	}

	sent = <-transport.ch
	decoded, err = decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}
	wantPayload, err := encodeWhoIsPayload(rangeReq)
	if err != nil {
		t.Fatalf("encodeWhoIsPayload: %v", err)
	}
	if len(decoded.Payload) != len(wantPayload) {
		t.Fatalf("payload len = %d, want %d", len(decoded.Payload), len(wantPayload))
	}
	for i := range wantPayload {
		if decoded.Payload[i] != wantPayload[i] {
			t.Fatalf("payload[%d] = 0x%02x, want 0x%02x", i, decoded.Payload[i], wantPayload[i])
		}
	}
}

func TestClientWhoHas(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})
	objID, _ := types.NewObjectIdentifier(types.ObjectTypeAnalogInput, 7)

	reqByID := WhoHasRequest{ObjectIdentifier: &objID}
	if err := client.WhoHas(context.Background(), dst, reqByID); err != nil {
		t.Fatalf("WhoHas (object-id): %v", err)
	}

	sent := <-transport.ch
	decoded, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}
	if decoded.Type != PDUTypeUnconfirmedRequest {
		t.Fatalf("type = %v, want %v", decoded.Type, PDUTypeUnconfirmedRequest)
	}
	if decoded.ServiceChoice != ServiceChoiceWhoHas {
		t.Fatalf("service = %v, want %v", decoded.ServiceChoice, ServiceChoiceWhoHas)
	}
	wantByID, _ := encodeWhoHasPayload(reqByID)
	if len(decoded.Payload) != len(wantByID) {
		t.Fatalf("payload len = %d, want %d", len(decoded.Payload), len(wantByID))
	}

	name := "AI-7"
	reqByName := WhoHasRequest{ObjectName: &name}
	if err := client.WhoHas(context.Background(), dst, reqByName); err != nil {
		t.Fatalf("WhoHas (object-name): %v", err)
	}

	sent = <-transport.ch
	decoded, err = decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}
	wantByName, _ := encodeWhoHasPayload(reqByName)
	if len(decoded.Payload) != len(wantByName) {
		t.Fatalf("payload len = %d, want %d", len(decoded.Payload), len(wantByName))
	}
}

func TestClientReadProperty(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})
	objID, _ := types.NewObjectIdentifier(types.ObjectTypeAnalogInput, 7)
	arrayIndex := uint32(3)
	req := ReadPropertyRequest{
		ObjectIdentifier:   objID,
		PropertyIdentifier: types.PropertyIdentifierPresentValue,
		ArrayIndex:         &arrayIndex,
	}

	type result struct {
		ack ReadPropertyACK
		err error
	}
	resultCh := make(chan result, 1)

	go func() {
		ack, readErr := client.ReadProperty(context.Background(), dst, req)
		resultCh <- result{ack: ack, err: readErr}
	}()

	sent := <-transport.ch
	outbound, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}
	if outbound.Type != PDUTypeConfirmedRequest {
		t.Fatalf("type = %v, want %v", outbound.Type, PDUTypeConfirmedRequest)
	}
	if outbound.ServiceChoice != ServiceChoiceReadProperty {
		t.Fatalf("service = %v, want %v", outbound.ServiceChoice, ServiceChoiceReadProperty)
	}

	decodedReq, err := decodeReadPropertyRequestPayload(outbound.Payload)
	if err != nil {
		t.Fatalf("decodeReadPropertyRequestPayload: %v", err)
	}
	if decodedReq.ObjectIdentifier != req.ObjectIdentifier {
		t.Fatalf("object identifier = %v, want %v", decodedReq.ObjectIdentifier, req.ObjectIdentifier)
	}
	if decodedReq.PropertyIdentifier != req.PropertyIdentifier {
		t.Fatalf("property identifier = %v, want %v", decodedReq.PropertyIdentifier, req.PropertyIdentifier)
	}
	if decodedReq.ArrayIndex == nil || *decodedReq.ArrayIndex != *req.ArrayIndex {
		t.Fatalf("array index = %v, want %v", decodedReq.ArrayIndex, req.ArrayIndex)
	}

	propValue := []byte{0x44, 0x41, 0x20, 0x00, 0x00} // application real 10.0
	ackPayload, err := encodeReadPropertyACKPayload(ReadPropertyACK{
		ObjectIdentifier:   req.ObjectIdentifier,
		PropertyIdentifier: req.PropertyIdentifier,
		ArrayIndex:         req.ArrayIndex,
		PropertyValue:      propValue,
	})
	if err != nil {
		t.Fatalf("encodeReadPropertyACKPayload: %v", err)
	}

	ackAPDU, err := encodeAPDU(outboundAPDU{
		Type:          PDUTypeComplexACK,
		InvokeID:      outbound.InvokeID,
		ServiceChoice: ServiceChoiceReadProperty,
		Payload:       ackPayload,
	})
	if err != nil {
		t.Fatalf("encodeAPDU ack: %v", err)
	}

	npkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, ackAPDU)
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}
	if err := ase.OnInboundNPDU(context.Background(), dst, *npkt); err != nil {
		t.Fatalf("OnInboundNPDU: %v", err)
	}

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("ReadProperty: %v", res.err)
	}
	if res.ack.ObjectIdentifier != req.ObjectIdentifier {
		t.Fatalf("ack object identifier = %v, want %v", res.ack.ObjectIdentifier, req.ObjectIdentifier)
	}
	if res.ack.PropertyIdentifier != req.PropertyIdentifier {
		t.Fatalf("ack property identifier = %v, want %v", res.ack.PropertyIdentifier, req.PropertyIdentifier)
	}
	if res.ack.ArrayIndex == nil || *res.ack.ArrayIndex != *req.ArrayIndex {
		t.Fatalf("ack array index = %v, want %v", res.ack.ArrayIndex, req.ArrayIndex)
	}
	if len(res.ack.PropertyValue) != len(propValue) {
		t.Fatalf("ack property value len = %d, want %d", len(res.ack.PropertyValue), len(propValue))
	}
	for i := range propValue {
		if res.ack.PropertyValue[i] != propValue[i] {
			t.Fatalf("ack property value[%d] = 0x%02x, want 0x%02x", i, res.ack.PropertyValue[i], propValue[i])
		}
	}
}

func TestClientInvokeConfirmedRaw(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})

	type result struct {
		payload []byte
		err     error
	}
	resultCh := make(chan result, 1)

	go func() {
		payload, invokeErr := client.InvokeConfirmedRaw(context.Background(), dst, ServiceChoiceReadProperty, []byte{0xAA})
		resultCh <- result{payload: payload, err: invokeErr}
	}()

	sent := <-transport.ch
	outbound, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}

	ackPayload := []byte{0xBB}
	ackBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeComplexACK, InvokeID: outbound.InvokeID, ServiceChoice: ServiceChoiceReadProperty, Payload: ackPayload})
	if err != nil {
		t.Fatalf("encodeAPDU: %v", err)
	}
	ack, _ := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, ackBytes)
	if err := ase.OnInboundNPDU(context.Background(), dst, *ack); err != nil {
		t.Fatalf("OnInboundNPDU: %v", err)
	}

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("InvokeConfirmedRaw: %v", res.err)
	}
	if len(res.payload) != 1 || res.payload[0] != 0xBB {
		t.Fatalf("payload = %v, want [0xBB]", res.payload)
	}
}

func TestClientInvokeConfirmedRawInvalidServiceChoice(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})
	_, err = client.InvokeConfirmedRaw(context.Background(), dst, ServiceChoiceWhoIs, nil)
	if !errors.Is(err, ErrInvalidServiceChoice) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidServiceChoice)
	}
}

func TestClientHandleIAm(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	indicationCh := make(chan IAmIndication, 1)
	err = client.RegisterIAmHandler(func(_ context.Context, indication IAmIndication) error {
		indicationCh <- indication
		return nil
	})
	if err != nil {
		t.Fatalf("HandleIAm: %v", err)
	}

	src, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x02})
	objID, _ := types.NewObjectIdentifier(types.ObjectTypeDevice, 1234)
	payload := encodeIAmPayloadForTest(objID, 1476, SegmentationSupportNo, 117)

	apduBytes, err := encodeAPDU(outboundAPDU{
		Type:          PDUTypeUnconfirmedRequest,
		ServiceChoice: ServiceChoiceIAm,
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("encodeAPDU: %v", err)
	}

	npkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, apduBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}
	if err := ase.OnInboundNPDU(context.Background(), src, *npkt); err != nil {
		t.Fatalf("OnInboundNPDU: %v", err)
	}

	select {
	case got := <-indicationCh:
		if !got.Source.Equal(src) {
			t.Fatalf("source = %v, want %v", got.Source, src)
		}
		if got.DeviceIdentifier != objID {
			t.Fatalf("device identifier = %v, want %v", got.DeviceIdentifier, objID)
		}
		if got.MaxAPDULengthAccepted != 1476 {
			t.Fatalf("max apdu = %d, want %d", got.MaxAPDULengthAccepted, 1476)
		}
		if got.SegmentationSupported != SegmentationSupportNo {
			t.Fatalf("segmentation = %v, want %v", got.SegmentationSupported, SegmentationSupportNo)
		}
		if got.VendorID != 117 {
			t.Fatalf("vendor id = %d, want %d", got.VendorID, 117)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for I-Am handler call")
	}
}

func TestClientHandleIAmNilHandler(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	err = client.RegisterIAmHandler(nil)
	if !errors.Is(err, ErrHandlerNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrHandlerNotFound)
	}
}

func TestClientHandleIAmMalformedPayload(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	err = client.RegisterIAmHandler(func(_ context.Context, _ IAmIndication) error { return nil })
	if err != nil {
		t.Fatalf("HandleIAm: %v", err)
	}

	src, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x02})
	apduBytes, err := encodeAPDU(outboundAPDU{
		Type:          PDUTypeUnconfirmedRequest,
		ServiceChoice: ServiceChoiceIAm,
		Payload:       []byte{0x00},
	})
	if err != nil {
		t.Fatalf("encodeAPDU: %v", err)
	}

	npkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, apduBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}
	err = ase.OnInboundNPDU(context.Background(), src, *npkt)
	if !errors.Is(err, ErrDecodeFailure) {
		t.Fatalf("err = %v, want %v", err, ErrDecodeFailure)
	}
}

func TestClientHandleIHave(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	indicationCh := make(chan IHaveIndication, 1)
	err = client.HandleIHave(func(_ context.Context, indication IHaveIndication) error {
		indicationCh <- indication
		return nil
	})
	if err != nil {
		t.Fatalf("HandleIHave: %v", err)
	}

	src, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x02})
	devID, _ := types.NewObjectIdentifier(types.ObjectTypeDevice, 1234)
	objID, _ := types.NewObjectIdentifier(types.ObjectTypeAnalogInput, 7)
	payload := encodeIHavePayloadForTest(devID, objID, "AI-7")

	apduBytes, err := encodeAPDU(outboundAPDU{
		Type:          PDUTypeUnconfirmedRequest,
		ServiceChoice: ServiceChoiceIHave,
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("encodeAPDU: %v", err)
	}

	npkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, apduBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}
	if err := ase.OnInboundNPDU(context.Background(), src, *npkt); err != nil {
		t.Fatalf("OnInboundNPDU: %v", err)
	}

	select {
	case got := <-indicationCh:
		if !got.Source.Equal(src) {
			t.Fatalf("source = %v, want %v", got.Source, src)
		}
		if got.DeviceIdentifier != devID {
			t.Fatalf("device identifier = %v, want %v", got.DeviceIdentifier, devID)
		}
		if got.ObjectIdentifier != objID {
			t.Fatalf("object identifier = %v, want %v", got.ObjectIdentifier, objID)
		}
		if got.ObjectName != "AI-7" {
			t.Fatalf("object name = %q, want %q", got.ObjectName, "AI-7")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for I-Have handler call")
	}
}

func TestClientHandleIHaveNilHandler(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	err = client.HandleIHave(nil)
	if !errors.Is(err, ErrHandlerNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrHandlerNotFound)
	}
}

func TestClientHandleIHaveMalformedPayload(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)

	err = client.HandleIHave(func(_ context.Context, _ IHaveIndication) error { return nil })
	if err != nil {
		t.Fatalf("HandleIHave: %v", err)
	}

	src, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x02})
	apduBytes, err := encodeAPDU(outboundAPDU{
		Type:          PDUTypeUnconfirmedRequest,
		ServiceChoice: ServiceChoiceIHave,
		Payload:       []byte{0x00},
	})
	if err != nil {
		t.Fatalf("encodeAPDU: %v", err)
	}

	npkt, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, apduBytes)
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}
	err = ase.OnInboundNPDU(context.Background(), src, *npkt)
	if !errors.Is(err, ErrDecodeFailure) {
		t.Fatalf("err = %v, want %v", err, ErrDecodeFailure)
	}
}

func decodeReadPropertyRequestPayload(payload []byte) (ReadPropertyRequest, error) {
	_, objValue, next, err := decodeExpectedContextPrimitive(payload, 0, 0)
	if err != nil {
		return ReadPropertyRequest{}, err
	}
	if len(objValue) != 4 {
		return ReadPropertyRequest{}, errors.New("invalid object identifier length")
	}
	obj := types.ObjectIdentifier(uint32(objValue[0])<<24 | uint32(objValue[1])<<16 | uint32(objValue[2])<<8 | uint32(objValue[3]))

	_, propValue, next2, err := decodeExpectedContextPrimitive(payload, next, 1)
	if err != nil {
		return ReadPropertyRequest{}, err
	}
	propID, err := decodeUnsigned(propValue)
	if err != nil {
		return ReadPropertyRequest{}, err
	}

	res := ReadPropertyRequest{ObjectIdentifier: obj, PropertyIdentifier: types.PropertyIdentifier(propID)}
	if next2 < len(payload) {
		_, arrValue, end, arrErr := decodeExpectedContextPrimitive(payload, next2, 2)
		if arrErr != nil {
			return ReadPropertyRequest{}, arrErr
		}
		arrIdx, arrErr := decodeUnsigned(arrValue)
		if arrErr != nil {
			return ReadPropertyRequest{}, arrErr
		}
		res.ArrayIndex = &arrIdx
		if end != len(payload) {
			return ReadPropertyRequest{}, errors.New("unexpected trailing bytes")
		}
	}

	return res, nil
}

func encodeReadPropertyACKPayload(ack ReadPropertyACK) ([]byte, error) {
	if len(ack.PropertyValue) == 0 {
		return nil, errors.New("property value must not be empty")
	}

	objRaw := uint32(ack.ObjectIdentifier)
	out := make([]byte, 0, 16+len(ack.PropertyValue))
	out = append(out, encodeContextPrimitive(0, []byte{byte(objRaw >> 24), byte(objRaw >> 16), byte(objRaw >> 8), byte(objRaw)})...)
	out = append(out, encodeContextPrimitive(1, encodeUnsigned(uint32(ack.PropertyIdentifier)))...)
	if ack.ArrayIndex != nil {
		out = append(out, encodeContextPrimitive(2, encodeUnsigned(*ack.ArrayIndex))...)
	}

	out = append(out, byte(0x3E))
	out = append(out, ack.PropertyValue...)
	out = append(out, byte(0x3F))

	return out, nil
}

func encodeIAmPayloadForTest(deviceIdentifier types.ObjectIdentifier, maxAPDU MaxApduLengthAccepted, segmentation SegmentationSupport, vendorID uint16) []byte {
	out := make([]byte, 0, 12)
	out = append(out, byte(12<<4)|4)
	rawObj := uint32(deviceIdentifier)
	out = append(out, byte(rawObj>>24), byte(rawObj>>16), byte(rawObj>>8), byte(rawObj))

	maxAPDUBytes := encodeUnsigned(uint32(maxAPDU))
	out = append(out, byte(2<<4)|byte(len(maxAPDUBytes)))
	out = append(out, maxAPDUBytes...)

	segBytes := encodeUnsigned(uint32(segmentation))
	out = append(out, byte(9<<4)|byte(len(segBytes)))
	out = append(out, segBytes...)

	vendorBytes := encodeUnsigned(uint32(vendorID))
	out = append(out, byte(2<<4)|byte(len(vendorBytes)))
	out = append(out, vendorBytes...)

	return out
}

func encodeIHavePayloadForTest(deviceIdentifier types.ObjectIdentifier, objectIdentifier types.ObjectIdentifier, objectName string) []byte {
	out := make([]byte, 0, 16+len(objectName))

	rawDev := uint32(deviceIdentifier)
	out = append(out, encodeApplicationPrimitiveForTest(12, []byte{byte(rawDev >> 24), byte(rawDev >> 16), byte(rawDev >> 8), byte(rawDev)})...)

	rawObj := uint32(objectIdentifier)
	out = append(out, encodeApplicationPrimitiveForTest(12, []byte{byte(rawObj >> 24), byte(rawObj >> 16), byte(rawObj >> 8), byte(rawObj)})...)

	charValue, _ := bacencoding.EncodeCharacterStringASCIIValue(objectName)
	out = append(out, encodeApplicationPrimitiveForTest(7, charValue)...)

	return out
}

func encodeApplicationPrimitiveForTest(tagNumber uint8, value []byte) []byte {
	if len(value) <= 4 {
		out := make([]byte, 1+len(value))
		out[0] = (tagNumber << 4) | byte(len(value))
		copy(out[1:], value)
		return out
	}

	if len(value) <= 253 {
		out := make([]byte, 0, 2+len(value))
		out = append(out, (tagNumber<<4)|0x05, byte(len(value)))
		out = append(out, value...)
		return out
	}

	// Current tests do not require larger values.
	return nil
}
