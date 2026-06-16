package npdu

import (
	"bytes"
	"errors"
	"testing"

	"go.wdy.de/bacnet"
)

func TestEncodeNetworkLayerMessageWire(t *testing.T) {
	dnet := bacnet.NetworkNumber(16)
	whoIs, err := NewWhoIsRouterToNetworkMessage(&dnet)
	if err != nil {
		t.Fatalf("NewWhoIsRouterToNetworkMessage: %v", err)
	}
	whatIs, err := NewWhatIsNetworkNumberMessage()
	if err != nil {
		t.Fatalf("NewWhatIsNetworkNumberMessage: %v", err)
	}
	networkNumberIs, err := NewNetworkNumberIsMessage(200, true)
	if err != nil {
		t.Fatalf("NewNetworkNumberIsMessage: %v", err)
	}
	proprietary, err := NewProprietaryNetworkLayerMessageModel(NetworkLayerMessageTypeVendorProprietary, 0x1234, []byte{0xAA})
	if err != nil {
		t.Fatalf("NewProprietaryNetworkLayerMessageModel: %v", err)
	}

	tests := []struct {
		name    string
		message NetworkLayerMessageModel
		want    []byte
		wantErr error
	}{
		{name: "who-is-router-to-network with dnet", message: whoIs, want: []byte{0x00, 0x00, 0x10}},
		{name: "what-is-network-number", message: whatIs, want: []byte{0x12}},
		{name: "network-number-is configured", message: networkNumberIs, want: []byte{0x13, 0x00, 0xC8, 0x01}},
		{name: "proprietary", message: proprietary, want: []byte{0x80, 0x12, 0x34, 0xAA}},
		{name: "nil message", message: nil, wantErr: ErrEncodeFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeNetworkLayerMessageWire(tt.message)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("wire = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDecodeNetworkLayerMessageWire(t *testing.T) {
	tests := []struct {
		name         string
		raw          []byte
		wantErr      error
		wantType     NetworkLayerMessageType
		wantPayload  []byte
		wantVendorID *uint16
	}{
		{name: "what-is-network-number", raw: []byte{0x12}, wantType: NetworkLayerMessageTypeWhatIsNetworkNumber, wantPayload: nil},
		{name: "network-number-is", raw: []byte{0x13, 0x00, 0xC8, 0x01}, wantType: NetworkLayerMessageTypeNetworkNumberIs, wantPayload: []byte{0x00, 0xC8, 0x01}},
		{name: "proprietary", raw: []byte{0x80, 0x12, 0x34, 0xAA}, wantType: NetworkLayerMessageTypeVendorProprietary, wantPayload: []byte{0xAA}, wantVendorID: func() *uint16 { v := uint16(0x1234); return &v }()},
		{name: "empty", raw: nil, wantErr: ErrDecodeFailure},
		{name: "truncated proprietary vendor id", raw: []byte{0x80, 0x12}, wantErr: ErrDecodeFailure},
		{name: "reserved standard type", raw: []byte{0x0A}, wantErr: ErrDecodeFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := DecodeNetworkLayerMessageWire(tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			if got := message.Header().MessageType; got != tt.wantType {
				t.Fatalf("message type = %v, want %v", got, tt.wantType)
			}
			if got := message.PayloadBytes(); !bytes.Equal(got, tt.wantPayload) {
				t.Fatalf("payload = %#v, want %#v", got, tt.wantPayload)
			}
			gotVendorID := message.Header().VendorID
			switch {
			case tt.wantVendorID == nil && gotVendorID != nil:
				t.Fatalf("vendor id = %v, want nil", *gotVendorID)
			case tt.wantVendorID != nil && gotVendorID == nil:
				t.Fatal("vendor id = nil, want non-nil")
			case tt.wantVendorID != nil && gotVendorID != nil && *gotVendorID != *tt.wantVendorID:
				t.Fatalf("vendor id = %v, want %v", *gotVendorID, *tt.wantVendorID)
			}
		})
	}
}

func TestNetworkLayerMessageWireRoundTrip(t *testing.T) {
	entry, err := NewRoutingTablePortEntry(300, 0x11, []byte{0xAA, 0xBB})
	if err != nil {
		t.Fatalf("NewRoutingTablePortEntry: %v", err)
	}
	message, err := NewInitializeRoutingTableMessage([]RoutingTablePortEntry{entry})
	if err != nil {
		t.Fatalf("NewInitializeRoutingTableMessage: %v", err)
	}

	raw, err := EncodeNetworkLayerMessageWire(message)
	if err != nil {
		t.Fatalf("EncodeNetworkLayerMessageWire: %v", err)
	}

	decoded, err := DecodeNetworkLayerMessageWire(raw)
	if err != nil {
		t.Fatalf("DecodeNetworkLayerMessageWire: %v", err)
	}

	if decoded.Header().MessageType != NetworkLayerMessageTypeInitializeRoutingTable {
		t.Fatalf("message type = %v, want %v", decoded.Header().MessageType, NetworkLayerMessageTypeInitializeRoutingTable)
	}
	if got := decoded.PayloadBytes(); !bytes.Equal(got, message.PayloadBytes()) {
		t.Fatalf("payload = %#v, want %#v", got, message.PayloadBytes())
	}
}

func TestNetworkLayerMessageWireMatchesNPDURepresentation(t *testing.T) {
	message, err := NewNetworkNumberIsMessage(321, false)
	if err != nil {
		t.Fatalf("NewNetworkNumberIsMessage: %v", err)
	}

	wire, err := EncodeNetworkLayerMessageWire(message)
	if err != nil {
		t.Fatalf("EncodeNetworkLayerMessageWire: %v", err)
	}

	n, err := NewNetworkLayerNPDUFromMessage(NPCI{Priority: bacnet.NetworkPriorityNormal}, message)
	if err != nil {
		t.Fatalf("NewNetworkLayerNPDUFromMessage: %v", err)
	}

	header := n.NetworkLayerHeader()
	if header == nil {
		t.Fatal("expected network-layer header")
	}
	if wire[0] != byte(header.MessageType) {
		t.Fatalf("wire message type = 0x%02X, want 0x%02X", wire[0], byte(header.MessageType))
	}
	if got := n.NetworkLayerPayloadBytes(); !bytes.Equal(got, wire[1:]) {
		t.Fatalf("NPDU payload = %#v, want %#v", got, wire[1:])
	}
}
