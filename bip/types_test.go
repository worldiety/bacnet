package bip

import "testing"

func TestBVLCTypeString(t *testing.T) {
	tests := []struct {
		name  string
		input BVLCType
		want  string
	}{
		{name: "bacnet ip", input: BVLCTypeBACnetIP, want: "bacnet-ip"},
		{name: "bacnet ip6", input: BVLCTypeBACnetIP6, want: "bacnet-ip6"},
		{name: "fallback", input: BVLCType(0x99), want: "bvlc-type(153)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBVLCTypeValid(t *testing.T) {
	if !BVLCTypeBACnetIP.Valid() {
		t.Fatal("expected bacnet-ip type to be valid")
	}
	if !BVLCTypeBACnetIP6.Valid() {
		t.Fatal("expected bacnet-ip6 type to be valid")
	}
	if BVLCType(0x99).Valid() {
		t.Fatal("unexpected valid BVLC type")
	}
}

func TestFunctionString(t *testing.T) {
	tests := []struct {
		name  string
		input BVLCFunction
		want  string
	}{
		{name: "result", input: FunctionResult, want: "result"},
		{name: "forwarded", input: FunctionForwardedNPDU, want: "forwarded-npdu"},
		{name: "original unicast", input: FunctionOriginalUnicastNPDU, want: "original-unicast-npdu"},
		{name: "original broadcast", input: FunctionOriginalBroadcastNPDU, want: "original-broadcast-npdu"},
		{name: "fallback", input: BVLCFunction(0xFF), want: "bvlc-function(255)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFunctionValid(t *testing.T) {
	if !FunctionOriginalBroadcastNPDU.Valid() {
		t.Fatal("expected original-broadcast-npdu to be valid")
	}
	if BVLCFunction(0x80).Valid() {
		t.Fatal("unexpected valid function")
	}
}
