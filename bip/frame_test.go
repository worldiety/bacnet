package bip

import (
	"errors"
	"net/netip"
	"testing"
)

func TestNewFrameCopiesPayload(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}
	frame, err := NewFrameWithType(BVLCTypeBACnetIP6, FunctionOriginalUnicastNPDU, payload)
	if err != nil {
		t.Fatalf("NewFrame returned error: %v", err)
	}

	payload[0] = 0xFF
	copied := frame.PayloadBytes()
	if copied[0] != 0x01 {
		t.Fatalf("payload was not copied, got 0x%02x", copied[0])
	}

	copied[1] = 0xEE
	again := frame.PayloadBytes()
	if again[1] != 0x02 {
		t.Fatalf("PayloadBytes should return defensive copy, got 0x%02x", again[1])
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		frameType BVLCType
		function  BVLCFunction
		payload   []byte
	}{
		{name: "ipv4 original unicast", frameType: BVLCTypeBACnetIP, function: FunctionOriginalUnicastNPDU, payload: []byte{0x11, 0x22}},
		{name: "ipv4 original broadcast", frameType: BVLCTypeBACnetIP, function: FunctionOriginalBroadcastNPDU, payload: []byte{0x33}},
		{name: "ipv4 result no payload", frameType: BVLCTypeBACnetIP, function: FunctionResult, payload: nil},
		{name: "ipv6 original unicast", frameType: BVLCTypeBACnetIP6, function: FunctionOriginalUnicastNPDU, payload: []byte{0x44, 0x55}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := NewFrameWithType(tt.frameType, tt.function, tt.payload)
			if err != nil {
				t.Fatalf("NewFrameWithType returned error: %v", err)
			}

			raw, err := frame.Encode()
			if err != nil {
				t.Fatalf("Encode returned error: %v", err)
			}

			decoded, err := DecodeFrame(raw)
			if err != nil {
				t.Fatalf("DecodeFrame returned error: %v", err)
			}

			if decoded.Type != tt.frameType {
				t.Fatalf("decoded type = %v, want %v", decoded.Type, tt.frameType)
			}
			if decoded.Function != tt.function {
				t.Fatalf("decoded function = %v, want %v", decoded.Function, tt.function)
			}

			got := decoded.PayloadBytes()
			if len(got) != len(tt.payload) {
				t.Fatalf("payload len = %d, want %d", len(got), len(tt.payload))
			}
			for i := range got {
				if got[i] != tt.payload[i] {
					t.Fatalf("payload[%d] = 0x%02x, want 0x%02x", i, got[i], tt.payload[i])
				}
			}
		})
	}
}

func TestNewFrameForAddressChoosesType(t *testing.T) {
	tests := []struct {
		name     string
		addr     netip.Addr
		wantType BVLCType
		wantErr  error
	}{
		{name: "ipv4", addr: netip.MustParseAddr("192.168.10.20"), wantType: BVLCTypeBACnetIP},
		{name: "ipv6", addr: netip.MustParseAddr("2001:db8::1"), wantType: BVLCTypeBACnetIP6},
		{name: "invalid", addr: netip.Addr{}, wantErr: ErrInvalidIPAddress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := NewFrameForAddress(tt.addr, FunctionOriginalUnicastNPDU, []byte{0xAA})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewFrameForAddress error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewFrameForAddress error = %v", err)
			}
			if frame.Type != tt.wantType {
				t.Fatalf("frame type = %v, want %v", frame.Type, tt.wantType)
			}
		})
	}
}

func TestDecodeFrameRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		wantErr error
	}{
		{name: "too short", raw: []byte{0x81, 0x0A, 0x00}, wantErr: ErrFrameTooShort},
		{name: "invalid type", raw: []byte{0x80, 0x0A, 0x00, 0x04}, wantErr: ErrInvalidBVLCType},
		{name: "invalid function", raw: []byte{0x81, 0xFF, 0x00, 0x04}, wantErr: ErrInvalidFunction},
		{name: "invalid length", raw: []byte{0x81, 0x0A, 0x00, 0x05}, wantErr: ErrInvalidLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeFrame(tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DecodeFrame error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
