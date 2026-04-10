package npdu

import (
	"bytes"
	"errors"
	"testing"

	"go.wdy.de/bacnet"
)

// --- Valid() ---

func TestValid(t *testing.T) {
	// helpers to build pointers
	dnet := func(v uint16) *UltimateDestinationNetworkNumber {
		n := UltimateDestinationNetworkNumber(v)
		return &n
	}
	dlen := func(v uint8) *UltimateDestinationNetworkNumberMacAddressLength {
		l := UltimateDestinationNetworkNumberMacAddressLength(v)
		return &l
	}
	snet := func(v uint16) *OriginalSourceNetworkNumber {
		n := OriginalSourceNetworkNumber(v)
		return &n
	}
	slen := func(v uint8) *OriginalSourceNetworkNumberMacAddressLength {
		l := OriginalSourceNetworkNumberMacAddressLength(v)
		return &l
	}
	hopCount := func(v uint8) *uint8 { return &v }
	msgType := func(v uint8) *uint8 { return &v }
	vendorID := func(v uint16) *uint16 { return &v }

	tests := []struct {
		name string
		n    *NetworkLayerProtocolDataUnit
		want bool
	}{
		{
			name: "nil pointer",
			n:    nil,
			want: false,
		},
		{
			name: "zero value",
			n:    &NetworkLayerProtocolDataUnit{},
			want: false,
		},
		{
			name: "wrong protocol version",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x02},
			want: false,
		},
		{
			name: "reserved bit 4 set",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: reservedBit4Mask},
			want: false,
		},
		{
			name: "reserved bit 6 set",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: reservedBit6Mask},
			want: false,
		},
		{
			name: "minimal local APDU",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, apdu: []byte{0x10}},
			want: true,
		},
		{
			name: "local APDU expecting reply",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: expectingReplyMask, apdu: []byte{0x10}},
			want: true,
		},
		{
			name: "NL message flag but messageType nil",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: isNetworkLayerMessageMask},
			want: false,
		},
		{
			name: "standard NL message without vendorID",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: isNetworkLayerMessageMask, messageType: msgType(0x01)},
			want: true,
		},
		{
			name: "standard NL message with spurious vendorID",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: isNetworkLayerMessageMask, messageType: msgType(0x01), vendorId: vendorID(0x1234)},
			want: false,
		},
		{
			name: "proprietary NL message with vendorID",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: isNetworkLayerMessageMask, messageType: msgType(0x80), vendorId: vendorID(0x1234)},
			want: true,
		},
		{
			name: "proprietary NL message without vendorID",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: isNetworkLayerMessageMask, messageType: msgType(0x80)},
			want: false,
		},
		{
			name: "vendorID set without NL message flag",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, vendorId: vendorID(0x1234)},
			want: false,
		},
		{
			name: "dst specifier missing dnet",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: destinationSpecifierMask, dlen: dlen(0), hopCount: hopCount(255)},
			want: false,
		},
		{
			name: "dst specifier missing dlen",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: destinationSpecifierMask, dnet: dnet(1), hopCount: hopCount(255)},
			want: false,
		},
		{
			name: "dst specifier missing hopCount",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: destinationSpecifierMask, dnet: dnet(1), dlen: dlen(0)},
			want: false,
		},
		{
			name: "dst specifier broadcast (dlen=0, dadr=nil)",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: destinationSpecifierMask, dnet: dnet(0xFFFF), dlen: dlen(0), hopCount: hopCount(255), apdu: []byte{0x10}},
			want: true,
		},
		{
			name: "dst specifier broadcast with spurious dadr",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: destinationSpecifierMask, dnet: dnet(1), dlen: dlen(0), dadr: []byte{0xAA}, hopCount: hopCount(255), apdu: []byte{0x10}},
			want: false,
		},
		{
			name: "dst specifier unicast matching dlen",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: destinationSpecifierMask, dnet: dnet(1), dlen: dlen(2), dadr: []byte{0xAA, 0xBB}, hopCount: hopCount(255), apdu: []byte{0x10}},
			want: true,
		},
		{
			name: "dst specifier unicast dadr length mismatch",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: destinationSpecifierMask, dnet: dnet(1), dlen: dlen(3), dadr: []byte{0xAA, 0xBB}, hopCount: hopCount(255)},
			want: false,
		},
		{
			name: "spurious dnet without dst specifier flag",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, dnet: dnet(1)},
			want: false,
		},
		{
			name: "src specifier missing snet",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: sourceSpecifierMask, slen: slen(1), sadr: []byte{0x99}},
			want: false,
		},
		{
			name: "src specifier slen=0 (not allowed)",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: sourceSpecifierMask, snet: snet(2), slen: slen(0)},
			want: false,
		},
		{
			name: "src specifier matching slen",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: sourceSpecifierMask, snet: snet(2), slen: slen(1), sadr: []byte{0x99}, apdu: []byte{0x10}},
			want: true,
		},
		{
			name: "src specifier sadr length mismatch",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, flags: sourceSpecifierMask, snet: snet(2), slen: slen(2), sadr: []byte{0x99}},
			want: false,
		},
		{
			name: "spurious snet without src specifier flag",
			n:    &NetworkLayerProtocolDataUnit{protocolVersion: 0x01, snet: snet(2)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.n.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Known wire bytes ---

// TestEncodeLocalAPDU verifies the wire layout for a simple local APDU.
//
// Wire: [0x01][0x04][0xDE][0xAD]
//
//	0x01 = version
//	0x04 = NPCI (bit 2: expecting reply, priority normal)
//	0xDE, 0xAD = APDU
func TestEncodeLocalAPDU(t *testing.T) {
	n, err := NewLocalAPDU(bacnet.NetworkPriorityNormal, true, []byte{0xDE, 0xAD})
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}

	got, err := n.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := []byte{0x01, 0x04, 0xDE, 0xAD}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = %#v, want %#v", got, want)
	}
}

// TestEncodeRoutedBroadcast verifies the wire layout for a routed broadcast (DLEN=0).
//
// Wire: [0x01][0x20][0xFF][0xFF][0x00][0xFF][0x10][0x08]
//
//	0x01 = version
//	0x20 = NPCI (bit 5: dst specifier, normal priority)
//	0xFF, 0xFF = DNET (global broadcast = 65535)
//	0x00 = DLEN (0 = broadcast on DNET)
//	0xFF = HopCount
//	0x10, 0x08 = APDU
func TestEncodeRoutedBroadcast(t *testing.T) {
	n, err := NewRoutedAPDU(0xFFFF, nil, 0xFF, bacnet.NetworkPriorityNormal, false, []byte{0x10, 0x08})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	got, err := n.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := []byte{0x01, 0x20, 0xFF, 0xFF, 0x00, 0xFF, 0x10, 0x08}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = %#v, want %#v", got, want)
	}
}

// TestEncodeRoutedUnicast verifies the wire layout for a routed unicast APDU.
//
// Wire: [0x01][0x20][0x00][0x01][0x03][0x01][0x02][0x03][0x0A][0x0C]
func TestEncodeRoutedUnicast(t *testing.T) {
	n, err := NewRoutedAPDU(1, []byte{0x01, 0x02, 0x03}, 0x0A, bacnet.NetworkPriorityNormal, false, []byte{0x0C})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	got, err := n.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := []byte{0x01, 0x20, 0x00, 0x01, 0x03, 0x01, 0x02, 0x03, 0x0A, 0x0C}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = %#v, want %#v", got, want)
	}
}

// TestEncodeWithSourceSpecifier verifies the wire layout for an NPDU carrying only a
// source specifier (common in router-generated frames).
//
// Wire: [0x01][0x08][0x00][0x02][0x01][0x99][0x10]
//
//	0x01 = version
//	0x08 = NPCI (bit 3: src specifier, normal priority)
//	0x00, 0x02 = SNET = 2
//	0x01 = SLEN = 1
//	0x99 = SADR
//	0x10 = APDU
func TestEncodeWithSourceSpecifier(t *testing.T) {
	snet := OriginalSourceNetworkNumber(2)
	slenVal := OriginalSourceNetworkNumberMacAddressLength(1)
	n := &NetworkLayerProtocolDataUnit{
		protocolVersion: 0x01,
		flags:           sourceSpecifierMask,
		snet:            &snet,
		slen:            &slenVal,
		sadr:            []byte{0x99},
		apdu:            []byte{0x10},
	}

	got, err := n.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := []byte{0x01, 0x08, 0x00, 0x02, 0x01, 0x99, 0x10}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = %#v, want %#v", got, want)
	}
}

// TestEncodeWithBothSpecifiers verifies the wire layout for an NPDU with both dst and src
// specifiers. Per clause 6.2.2 the order is: DNET/DLEN/DADR → SNET/SLEN/SADR → HopCount.
//
// Wire: [0x01][0x28] [DNET=5,2B] [DLEN=2] [DADR=0xAB,0xCD] [SNET=3,2B] [SLEN=1] [SADR=0x99] [HC=4] [APDU=0xFF]
func TestEncodeWithBothSpecifiers(t *testing.T) {
	dnet := UltimateDestinationNetworkNumber(5)
	dlenVal := UltimateDestinationNetworkNumberMacAddressLength(2)
	snet := OriginalSourceNetworkNumber(3)
	slenVal := OriginalSourceNetworkNumberMacAddressLength(1)
	hc := uint8(4)
	n := &NetworkLayerProtocolDataUnit{
		protocolVersion: 0x01,
		flags:           destinationSpecifierMask | sourceSpecifierMask,
		dnet:            &dnet,
		dlen:            &dlenVal,
		dadr:            []byte{0xAB, 0xCD},
		snet:            &snet,
		slen:            &slenVal,
		sadr:            []byte{0x99},
		hopCount:        &hc,
		apdu:            []byte{0xFF},
	}

	got, err := n.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := []byte{0x01, 0x28, 0x00, 0x05, 0x02, 0xAB, 0xCD, 0x00, 0x03, 0x01, 0x99, 0x04, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = %#v, want %#v", got, want)
	}
}

// TestEncodeNetworkLayerMessage verifies the wire layout for a standard NL message.
//
// Wire: [0x01][0x80][0x01][0xDE][0xAD]
func TestEncodeNetworkLayerMessage(t *testing.T) {
	n, err := NewNetworkLayerMessage(0x01, []byte{0xDE, 0xAD}, bacnet.NetworkPriorityNormal)
	if err != nil {
		t.Fatalf("NewNetworkLayerMessage: %v", err)
	}

	got, err := n.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := []byte{0x01, 0x80, 0x01, 0xDE, 0xAD}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = %#v, want %#v", got, want)
	}
}

// TestEncodeProprietaryNetworkLayerMessage verifies the wire layout for a proprietary NL
// message, which includes a 2-byte VendorID after the MessageType byte.
//
// Wire: [0x01][0x80][0x80][0x12][0x34][0x42]
func TestEncodeProprietaryNetworkLayerMessage(t *testing.T) {
	n, err := NewProprietaryNetworkLayerMessage(0x80, 0x1234, []byte{0x42}, bacnet.NetworkPriorityNormal)
	if err != nil {
		t.Fatalf("NewProprietaryNetworkLayerMessage: %v", err)
	}

	got, err := n.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := []byte{0x01, 0x80, 0x80, 0x12, 0x34, 0x42}
	if !bytes.Equal(got, want) {
		t.Errorf("Encode() = %#v, want %#v", got, want)
	}
}

// --- Decode error cases ---

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "empty",
			data:    []byte{},
			wantErr: ErrInvalidLength,
		},
		{
			name:    "only one byte",
			data:    []byte{0x01},
			wantErr: ErrInvalidLength,
		},
		{
			name:    "wrong protocol version",
			data:    []byte{0x02, 0x00},
			wantErr: ErrInvalidProtocolVersion,
		},
		{
			name:    "reserved bit 4 set",
			data:    []byte{0x01, 0x10},
			wantErr: ErrReservedBitSet,
		},
		{
			name:    "reserved bit 6 set",
			data:    []byte{0x01, 0x40},
			wantErr: ErrReservedBitSet,
		},
		{
			name:    "dst specifier truncated at DNET",
			data:    []byte{0x01, 0x20, 0x00},
			wantErr: ErrDecodeFailure,
		},
		{
			name:    "dst specifier truncated in DADR",
			data:    []byte{0x01, 0x20, 0x00, 0x01, 0x04, 0xAA},
			wantErr: ErrDecodeFailure,
		},
		{
			name:    "dst specifier missing HopCount",
			data:    []byte{0x01, 0x20, 0xFF, 0xFF, 0x00},
			wantErr: ErrDecodeFailure,
		},
		{
			name:    "src specifier truncated at SNET",
			data:    []byte{0x01, 0x08, 0x00},
			wantErr: ErrDecodeFailure,
		},
		{
			name:    "src specifier SLEN=0",
			data:    []byte{0x01, 0x08, 0x00, 0x02, 0x00},
			wantErr: ErrInvalidLength,
		},
		{
			name:    "src specifier truncated in SADR",
			data:    []byte{0x01, 0x08, 0x00, 0x02, 0x04, 0xAA},
			wantErr: ErrDecodeFailure,
		},
		{
			name:    "NL message missing MessageType",
			data:    []byte{0x01, 0x80},
			wantErr: ErrDecodeFailure,
		},
		{
			name:    "proprietary NL message missing VendorID",
			data:    []byte{0x01, 0x80, 0x80, 0x12},
			wantErr: ErrDecodeFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n NetworkLayerProtocolDataUnit
			err := n.Decode(tt.data)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Decode() err = %v, want sentinel %v", err, tt.wantErr)
			}
		})
	}
}

// --- Encode/Decode roundtrips ---

func TestRoundtrip(t *testing.T) {
	localApdu, _ := NewLocalAPDU(bacnet.NetworkPriorityUrgent, true, []byte{0x0C, 0x0C})
	routedBroadcast, _ := NewRoutedAPDU(0xFFFF, nil, 255, bacnet.NetworkPriorityNormal, false, []byte{0x10})
	routedUnicast, _ := NewRoutedAPDU(7, []byte{0xAA, 0xBB}, 10, bacnet.NetworkPriorityLifeSafety, true, []byte{0xFF})
	nlMsg, _ := NewNetworkLayerMessage(0x04, []byte{0x01, 0x02}, bacnet.NetworkPriorityNormal)
	propMsg, _ := NewProprietaryNetworkLayerMessage(0xFF, 0xABCD, []byte{0x99}, bacnet.NetworkPriorityCriticalEquipment)

	tests := []struct {
		name string
		orig *NetworkLayerProtocolDataUnit
	}{
		{"local APDU", localApdu},
		{"routed broadcast", routedBroadcast},
		{"routed unicast", routedUnicast},
		{"NL message", nlMsg},
		{"proprietary NL message", propMsg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := tt.orig.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			var decoded NetworkLayerProtocolDataUnit
			if err := decoded.Decode(wire); err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if !decoded.Valid() {
				t.Fatal("decoded NPDU is not valid")
			}

			if decoded.Version() != tt.orig.Version() {
				t.Errorf("Version = %d, want %d", decoded.Version(), tt.orig.Version())
			}
			if decoded.Flags() != tt.orig.Flags() {
				t.Errorf("Flags = 0x%02X, want 0x%02X", decoded.Flags(), tt.orig.Flags())
			}
			if !bytes.Equal(decoded.APDUBytes(), tt.orig.APDUBytes()) {
				t.Errorf("APDUBytes = %#v, want %#v", decoded.APDUBytes(), tt.orig.APDUBytes())
			}
		})
	}
}

// --- Constructor validation ---

func TestNewLocalAPDU(t *testing.T) {
	tests := []struct {
		name        string
		priority    bacnet.NetworkPriority
		expectReply bool
		apdu        []byte
		wantErr     error
	}{
		{"valid normal priority", bacnet.NetworkPriorityNormal, false, []byte{0x10}, nil},
		{"valid life safety", bacnet.NetworkPriorityLifeSafety, true, []byte{0x10}, nil},
		{"priority out of range", 4, false, []byte{0x10}, ErrInvalidPriority},
		{"empty apdu", bacnet.NetworkPriorityNormal, false, []byte{}, ErrInvalidLength},
		{"nil apdu", bacnet.NetworkPriorityNormal, false, nil, ErrInvalidLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLocalAPDU(tt.priority, tt.expectReply, tt.apdu)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !got.Valid() {
				t.Error("constructed NPDU is not Valid()")
			}
			if got.Priority() != tt.priority {
				t.Errorf("Priority = %d, want %d", got.Priority(), tt.priority)
			}
			if got.IsExpectingReply() != tt.expectReply {
				t.Errorf("IsExpectingReply = %v, want %v", got.IsExpectingReply(), tt.expectReply)
			}
			if got.HasDestinationSpecifier() || got.HasSourceSpecifier() {
				t.Error("local APDU must not have routing specifiers")
			}
		})
	}
}

func TestNewRoutedAPDU(t *testing.T) {
	tests := []struct {
		name     string
		dnet     UltimateDestinationNetworkNumber
		dadr     UltimateDestinationMacLayerAddress
		hopCount uint8
		priority bacnet.NetworkPriority
		apdu     []byte
		wantErr  error
	}{
		{"broadcast on remote net", 100, nil, 255, bacnet.NetworkPriorityNormal, []byte{0x10}, nil},
		{"unicast on remote net", 100, []byte{0xAA, 0xBB}, 10, bacnet.NetworkPriorityNormal, []byte{0x10}, nil},
		{"global broadcast", 0xFFFF, nil, 255, bacnet.NetworkPriorityNormal, []byte{0x10}, nil},
		{"priority out of range", 1, nil, 255, 5, []byte{0x10}, ErrInvalidPriority},
		{"empty apdu", 1, nil, 255, bacnet.NetworkPriorityNormal, nil, ErrInvalidLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRoutedAPDU(tt.dnet, tt.dadr, tt.hopCount, tt.priority, false, tt.apdu)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !got.Valid() {
				t.Error("constructed NPDU is not Valid()")
			}
			if !got.HasDestinationSpecifier() {
				t.Error("routed NPDU must have destination specifier")
			}
			if dnet := got.DNET(); dnet == nil || *dnet != tt.dnet {
				t.Errorf("DNET = %v, want %d", dnet, tt.dnet)
			}
			if hc := got.HopCount(); hc == nil || *hc != tt.hopCount {
				t.Errorf("HopCount = %v, want %d", hc, tt.hopCount)
			}
		})
	}
}

func TestNewNetworkLayerMessage(t *testing.T) {
	tests := []struct {
		name        string
		messageType uint8
		data        []byte
		priority    bacnet.NetworkPriority
		wantErr     error
	}{
		{"standard type", 0x01, []byte{0xDE, 0xAD}, bacnet.NetworkPriorityNormal, nil},
		{"standard type no data", 0x04, nil, bacnet.NetworkPriorityNormal, nil},
		{"proprietary type rejected", 0x80, nil, bacnet.NetworkPriorityNormal, ErrProprietaryMessageType},
		{"priority out of range", 0x01, nil, 5, ErrInvalidPriority},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewNetworkLayerMessage(tt.messageType, tt.data, tt.priority)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !got.Valid() {
				t.Error("constructed NPDU is not Valid()")
			}
			if !got.IsNetworkLayerMessage() {
				t.Error("must report IsNetworkLayerMessage() = true")
			}
			if mt := got.MessageType(); mt == nil || *mt != tt.messageType {
				t.Errorf("MessageType = %v, want %d", mt, tt.messageType)
			}
			if got.VendorID() != nil {
				t.Error("standard NL message must not carry a VendorID")
			}
		})
	}
}

func TestNewProprietaryNetworkLayerMessage(t *testing.T) {
	tests := []struct {
		name        string
		messageType uint8
		vendorID    uint16
		data        []byte
		priority    bacnet.NetworkPriority
		wantErr     error
	}{
		{"valid", 0x80, 0x1234, []byte{0x42}, bacnet.NetworkPriorityNormal, nil},
		{"max type", 0xFF, 0xFFFF, nil, bacnet.NetworkPriorityNormal, nil},
		{"standard type rejected", 0x7F, 0x1234, nil, bacnet.NetworkPriorityNormal, ErrInvalidMessageType},
		{"priority out of range", 0x80, 0x1234, nil, 9, ErrInvalidPriority},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProprietaryNetworkLayerMessage(tt.messageType, tt.vendorID, tt.data, tt.priority)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !got.Valid() {
				t.Error("constructed NPDU is not Valid()")
			}
			if vid := got.VendorID(); vid == nil || *vid != tt.vendorID {
				t.Errorf("VendorID = %v, want %d", vid, tt.vendorID)
			}
		})
	}
}

// --- Defensive copy ---

func TestNewLocalAPDUDefensiveCopy(t *testing.T) {
	apdu := []byte{0x10, 0x08}
	n, _ := NewLocalAPDU(bacnet.NetworkPriorityNormal, false, apdu)

	// Mutating original slice must not affect the NPDU.
	apdu[0] = 0xFF
	if got := n.APDUBytes(); got[0] == 0xFF {
		t.Error("NewLocalAPDU did not make a defensive copy of apdu")
	}
}

func TestAPDUBytesDefensiveCopy(t *testing.T) {
	n, _ := NewLocalAPDU(bacnet.NetworkPriorityNormal, false, []byte{0x10, 0x08})

	a := n.APDUBytes()
	a[0] = 0xFF
	if b := n.APDUBytes(); b[0] == 0xFF {
		t.Error("APDUBytes did not return a defensive copy")
	}
}

func TestDADRDefensiveCopy(t *testing.T) {
	n, _ := NewRoutedAPDU(1, []byte{0xAA, 0xBB}, 255, bacnet.NetworkPriorityNormal, false, []byte{0x10})

	dadr := n.DADR()
	dadr[0] = 0xFF
	if got := n.DADR(); got[0] == 0xFF {
		t.Error("DADR did not return a defensive copy")
	}
}

func TestDecodeIsDefensiveCopy(t *testing.T) {
	wire := []byte{0x01, 0x00, 0xDE, 0xAD}
	var n NetworkLayerProtocolDataUnit
	if err := n.Decode(wire); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Mutating the original wire buffer must not affect the decoded NPDU.
	wire[2] = 0xFF
	if got := n.APDUBytes(); got[0] == 0xFF {
		t.Error("Decode did not make a defensive copy of the payload")
	}
}

// TestPriority verifies that all four priority levels survive an encode/decode roundtrip.
func TestPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority bacnet.NetworkPriority
	}{
		{"Normal", bacnet.NetworkPriorityNormal},
		{"Urgent", bacnet.NetworkPriorityUrgent},
		{"CriticalEquipment", bacnet.NetworkPriorityCriticalEquipment},
		{"LifeSafety", bacnet.NetworkPriorityLifeSafety},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, _ := NewLocalAPDU(tt.priority, false, []byte{0x01})
			wire, _ := n.Encode()

			var decoded NetworkLayerProtocolDataUnit
			if err := decoded.Decode(wire); err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if decoded.Priority() != tt.priority {
				t.Errorf("Priority = %d, want %d", decoded.Priority(), tt.priority)
			}
		})
	}
}

// TestEncodeInvalidNPDU verifies that Encode rejects an invalid (zero-value) NPDU.
func TestEncodeInvalidNPDU(t *testing.T) {
	var n NetworkLayerProtocolDataUnit
	_, err := n.Encode()
	if !errors.Is(err, ErrEncodeFailure) {
		t.Errorf("Encode() err = %v, want %v", err, ErrEncodeFailure)
	}
}
