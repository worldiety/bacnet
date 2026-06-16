package npdu

import (
	"bytes"
	"errors"
	"testing"

	"go.wdy.de/bacnet"
)

func TestDecodeNetworkLayerMessageModel(t *testing.T) {
	tests := []struct {
		name    string
		header  NetworkLayerMessageHeader
		payload []byte
		wantErr error
	}{
		{
			name:    "who-is-router-to-network empty",
			header:  NetworkLayerMessageHeader{MessageType: NetworkLayerMessageTypeWhoIsRouterToNetwork},
			payload: nil,
			wantErr: nil,
		},
		{
			name:    "reject-message-to-network valid",
			header:  NetworkLayerMessageHeader{MessageType: NetworkLayerMessageTypeRejectMessageToNetwork},
			payload: []byte{0x00, 0x02, 0x01},
			wantErr: nil,
		},
		{
			name:    "network-number-is invalid flag",
			header:  NetworkLayerMessageHeader{MessageType: NetworkLayerMessageTypeNetworkNumberIs},
			payload: []byte{0x00, 0x01, 0x02},
			wantErr: ErrInvalidMessage,
		},
		{
			name:    "initialize-routing-table invalid local connected network",
			header:  NetworkLayerMessageHeader{MessageType: NetworkLayerMessageTypeInitializeRoutingTable},
			payload: []byte{0x01, 0x00, 0x00, 0x11, 0x00},
			wantErr: ErrInvalidNetworkNumber,
		},
		{
			name:    "proprietary with missing vendor",
			header:  NetworkLayerMessageHeader{MessageType: NetworkLayerMessageTypeVendorProprietary},
			payload: []byte{0xAA},
			wantErr: ErrInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeNetworkLayerMessageModel(tt.header, tt.payload)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewNetworkLayerNPDUFromMessage(t *testing.T) {
	msg, err := NewNetworkNumberIsMessage(200, true)
	if err != nil {
		t.Fatalf("NewNetworkNumberIsMessage: %v", err)
	}

	n, err := NewNetworkLayerNPDUFromMessage(
		NPCI{Priority: bacnet.NetworkPriorityNormal},
		msg,
	)
	if err != nil {
		t.Fatalf("NewNetworkLayerNPDUFromMessage: %v", err)
	}

	if !n.IsNetworkLayerMessage() {
		t.Fatal("expected NPDU to carry network-layer message")
	}

	if got := n.APDUBytes(); !bytes.Equal(got, []byte{0x00, 0xC8, 0x01}) {
		t.Fatalf("payload = %#v, want %#v", got, []byte{0x00, 0xC8, 0x01})
	}
}

func TestNetworkLayerMessageModelAccessor(t *testing.T) {
	n, err := NewNetworkLayerMessage(uint8(NetworkLayerMessageTypeWhatIsNetworkNumber), nil, bacnet.NetworkPriorityNormal)
	if err != nil {
		t.Fatalf("NewNetworkLayerMessage: %v", err)
	}

	model, err := n.NetworkLayerMessageModel()
	if err != nil {
		t.Fatalf("NetworkLayerMessageModel: %v", err)
	}
	if model == nil {
		t.Fatal("expected model")
	}
	if model.Header().MessageType != NetworkLayerMessageTypeWhatIsNetworkNumber {
		t.Fatalf("message type = %v, want %v", model.Header().MessageType, NetworkLayerMessageTypeWhatIsNetworkNumber)
	}
}

func TestNetworkLayerMessageModelAccessorForApplicationNPDU(t *testing.T) {
	n, err := NewLocalAPDU(bacnet.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}

	model, err := n.NetworkLayerMessageModel()
	if err != nil {
		t.Fatalf("NetworkLayerMessageModel: %v", err)
	}
	if model != nil {
		t.Fatal("expected nil model for application NPDU")
	}
}

func TestMustNetworkLayerMessageHeaderFromModel(t *testing.T) {
	message, err := NewNetworkNumberIsMessage(321, true)
	if err != nil {
		t.Fatalf("NewNetworkNumberIsMessage: %v", err)
	}

	header := MustNetworkLayerMessageHeader(message)
	if header.MessageType != NetworkLayerMessageTypeNetworkNumberIs {
		t.Fatalf("message type = %v, want %v", header.MessageType, NetworkLayerMessageTypeNetworkNumberIs)
	}
	if header.VendorID != nil {
		t.Fatal("vendor id should be nil for standard message")
	}
}

func TestMustNetworkLayerMessageHeaderPanicsForNilModel(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = MustNetworkLayerMessageHeader(nil)
}

func TestMustNetworkLayerMessageHeaderFromNPDU(t *testing.T) {
	n, err := NewNetworkLayerMessage(uint8(NetworkLayerMessageTypeWhatIsNetworkNumber), nil, bacnet.NetworkPriorityNormal)
	if err != nil {
		t.Fatalf("NewNetworkLayerMessage: %v", err)
	}

	header := n.MustNetworkLayerMessageHeader()
	if header.MessageType != NetworkLayerMessageTypeWhatIsNetworkNumber {
		t.Fatalf("message type = %v, want %v", header.MessageType, NetworkLayerMessageTypeWhatIsNetworkNumber)
	}
}

func TestMustNetworkLayerMessageHeaderFromNPDUPanicsForApplication(t *testing.T) {
	n, err := NewLocalAPDU(bacnet.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = n.MustNetworkLayerMessageHeader()
}

func TestNetworkListMessageSemantics(t *testing.T) {
	tests := []struct {
		name    string
		build   func() error
		wantErr error
	}{
		{
			name: "i-am-router requires one network",
			build: func() error {
				_, err := NewIAmRouterToNetworkMessage(nil)
				return err
			},
			wantErr: ErrInvalidLength,
		},
		{
			name: "router-busy requires one network",
			build: func() error {
				_, err := NewRouterBusyToNetworkMessage(nil)
				return err
			},
			wantErr: ErrInvalidLength,
		},
		{
			name: "router-available requires one network",
			build: func() error {
				_, err := NewRouterAvailableToNetworkMessage(nil)
				return err
			},
			wantErr: ErrInvalidLength,
		},
		{
			name: "router-list rejects local network",
			build: func() error {
				_, err := NewIAmRouterToNetworkMessage([]bacnet.NetworkNumber{bacnet.LocalNetwork})
				return err
			},
			wantErr: ErrInvalidNetworkNumber,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
