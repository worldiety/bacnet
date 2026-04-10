package apdu

import (
	"testing"

	"go.wdy.de/bacnet"
)

// --- NetworkPriority ---

func TestNetworkPriorityString(t *testing.T) {
	tests := []struct {
		p    bacnet.NetworkPriority
		want string
	}{
		{bacnet.NetworkPriorityNormal, "normal"},
		{bacnet.NetworkPriorityUrgent, "urgent"},
		{bacnet.NetworkPriorityCriticalEquipment, "critical-equipment"},
		{bacnet.NetworkPriorityLifeSafety, "life-safety"},
		{bacnet.NetworkPriority(4), "network-priority(4)"},
		{bacnet.NetworkPriority(255), "network-priority(255)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.p.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNetworkPriorityValid(t *testing.T) {
	tests := []struct {
		p    bacnet.NetworkPriority
		want bool
	}{
		{bacnet.NetworkPriorityNormal, true},
		{bacnet.NetworkPriorityUrgent, true},
		{bacnet.NetworkPriorityCriticalEquipment, true},
		{bacnet.NetworkPriorityLifeSafety, true},
		{bacnet.NetworkPriority(4), false},
		{bacnet.NetworkPriority(255), false},
	}
	for _, tt := range tests {
		t.Run(tt.p.String(), func(t *testing.T) {
			if got := tt.p.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- MaxSegmentsAccepted ---

func TestMaxSegmentsAcceptedString(t *testing.T) {
	tests := []struct {
		m    MaxSegmentsAccepted
		want string
	}{
		{MaxSegmentsUnspecified, "unspecified"},
		{MaxSegments2, "2"},
		{MaxSegments4, "4"},
		{MaxSegments8, "8"},
		{MaxSegments16, "16"},
		{MaxSegments32, "32"},
		{MaxSegments64, "64"},
		{MaxSegmentsMoreThan64, "more-than-64"},
		{MaxSegmentsAccepted(8), "max-segments(8)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.m.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaxSegmentsAcceptedValid(t *testing.T) {
	// All values 0-7 must be valid.
	for i := 0; i <= 7; i++ {
		if !MaxSegmentsAccepted(i).Valid() {
			t.Errorf("MaxSegmentsAccepted(%d).Valid() = false, want true", i)
		}
	}
	// Values 8 and above must be invalid.
	for _, v := range []MaxSegmentsAccepted{8, 9, 100, 255} {
		if v.Valid() {
			t.Errorf("MaxSegmentsAccepted(%d).Valid() = true, want false", v)
		}
	}
}

// --- ConfirmResult ---

func TestConfirmResultString(t *testing.T) {
	tests := []struct {
		r    ConfirmResult
		want string
	}{
		{ConfirmResultPositiveAck, "positive-ack"},
		{ConfirmResultError, "error"},
		{ConfirmResultReject, "reject"},
		{ConfirmResultAbort, "abort"},
		{ConfirmResult(4), "confirm-result(4)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- ICI struct field access ---

func TestConfirmedRequestICIFields(t *testing.T) {
	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})
	ici := ConfirmedRequestICI{
		Destination:           dst,
		MaxAPDULengthAccepted: 1476,
		SegmentationSupported: SegmentationBoth,
		MaxSegmentsAccepted:   MaxSegments16,
		Priority:              bacnet.NetworkPriorityUrgent,
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0x0C, 0x02},
		},
	}

	if !ici.Priority.Valid() {
		t.Error("Priority.Valid() = false, want true")
	}
	if !ici.MaxSegmentsAccepted.Valid() {
		t.Error("MaxSegmentsAccepted.Valid() = false, want true")
	}
	if ici.MaxAPDULengthAccepted != 1476 {
		t.Errorf("MaxAPDULengthAccepted = %d, want 1476", ici.MaxAPDULengthAccepted)
	}
	if ici.SegmentationSupported != SegmentationBoth {
		t.Errorf("SegmentationSupported = %v, want %v", ici.SegmentationSupported, SegmentationBoth)
	}
	if ici.ServiceRequest.ServiceChoice != ServiceChoiceReadProperty {
		t.Errorf("ServiceChoice = %v, want %v", ici.ServiceRequest.ServiceChoice, ServiceChoiceReadProperty)
	}
	if len(ici.ServiceRequest.Payload) != 2 {
		t.Errorf("Payload length = %d, want 2", len(ici.ServiceRequest.Payload))
	}
}

func TestUnconfirmedRequestICIFields(t *testing.T) {
	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0xFF})
	ici := UnconfirmedRequestICI{
		Destination: dst,
		Priority:    bacnet.NetworkPriorityNormal,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: ServiceChoiceWhoIs,
			Payload:       []byte{0x00, 0xFF},
		},
	}

	if ici.Priority != bacnet.NetworkPriorityNormal {
		t.Errorf("Priority = %v, want %v", ici.Priority, bacnet.NetworkPriorityNormal)
	}
	if ici.ServiceRequest.ServiceChoice != ServiceChoiceWhoIs {
		t.Errorf("ServiceChoice = %v, want %v", ici.ServiceRequest.ServiceChoice, ServiceChoiceWhoIs)
	}
}

func TestConfirmedIndicationICIDataExpectingReply(t *testing.T) {
	src, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x02})
	ici := ConfirmedIndicationICI{
		Source:                src,
		InvokeID:              InvokeID(42),
		MaxAPDULengthAccepted: 1476,
		SegmentationSupported: SegmentationNo,
		MaxSegmentsAccepted:   MaxSegmentsUnspecified,
		Priority:              bacnet.NetworkPriorityNormal,
		DataExpectingReply:    true, // always true for confirmed services
		ServiceRequest: ConfirmedRequest{
			ServiceChoice: ServiceChoiceReadProperty,
			Payload:       []byte{0x0C},
		},
	}

	if ici.InvokeID != 42 {
		t.Errorf("InvokeID = %d, want 42", ici.InvokeID)
	}
	if !ici.DataExpectingReply {
		t.Error("DataExpectingReply = false; confirmed service indications always expect a reply")
	}
	if ici.MaxSegmentsAccepted != MaxSegmentsUnspecified {
		t.Errorf("MaxSegmentsAccepted = %v, want %v", ici.MaxSegmentsAccepted, MaxSegmentsUnspecified)
	}
}

func TestUnconfirmedIndicationICIFields(t *testing.T) {
	src, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x03})
	ici := UnconfirmedIndicationICI{
		Source:   src,
		Priority: bacnet.NetworkPriorityNormal,
		ServiceRequest: UnconfirmedRequest{
			ServiceChoice: ServiceChoiceIAm,
			Payload:       []byte{0xDE, 0xAD},
		},
	}

	if ici.ServiceRequest.ServiceChoice != ServiceChoiceIAm {
		t.Errorf("ServiceChoice = %v, want %v", ici.ServiceRequest.ServiceChoice, ServiceChoiceIAm)
	}
	if len(ici.ServiceRequest.Payload) != 2 {
		t.Errorf("Payload length = %d, want 2", len(ici.ServiceRequest.Payload))
	}
}

func TestConfirmedResponseICIFields(t *testing.T) {
	dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x04})
	ici := ConfirmedResponseICI{
		Destination:           dst,
		InvokeID:              InvokeID(7),
		SegmentationSupported: SegmentationBoth,
		ServiceResponse:       ServiceResult{Payload: []byte{0x42, 0x43}},
	}

	if ici.InvokeID != 7 {
		t.Errorf("InvokeID = %d, want 7", ici.InvokeID)
	}
	if ici.SegmentationSupported != SegmentationBoth {
		t.Errorf("SegmentationSupported = %v, want %v", ici.SegmentationSupported, SegmentationBoth)
	}
	if len(ici.ServiceResponse.Payload) != 2 || ici.ServiceResponse.Payload[0] != 0x42 {
		t.Errorf("ServiceResponse.Payload = %v, want [0x42 0x43]", ici.ServiceResponse.Payload)
	}
}

func TestConfirmICIPositiveAck(t *testing.T) {
	resp := ServiceResult{Payload: []byte{0xBB}}
	ici := ConfirmICI{
		InvokeID:        InvokeID(3),
		Result:          ConfirmResultPositiveAck,
		ServiceResponse: &resp,
	}

	if ici.Result != ConfirmResultPositiveAck {
		t.Errorf("Result = %v, want %v", ici.Result, ConfirmResultPositiveAck)
	}
	if ici.ServiceResponse == nil {
		t.Fatal("ServiceResponse is nil, want non-nil for positive-ack")
	}
	if ici.ServiceResponse.Payload[0] != 0xBB {
		t.Errorf("ServiceResponse.Payload[0] = 0x%02X, want 0xBB", ici.ServiceResponse.Payload[0])
	}
}

func TestConfirmICINonPositiveAckNilServiceResponse(t *testing.T) {
	// For Error, Reject, and Abort outcomes, ServiceResponse must be nil.
	nonAck := []ConfirmResult{ConfirmResultError, ConfirmResultReject, ConfirmResultAbort}
	for _, r := range nonAck {
		t.Run(r.String(), func(t *testing.T) {
			ici := ConfirmICI{
				InvokeID:        InvokeID(9),
				Result:          r,
				ServiceResponse: nil,
			}
			if ici.ServiceResponse != nil {
				t.Errorf("ServiceResponse must be nil for %v", r)
			}
		})
	}
}

// --- String() fallback coverage ---

func TestNetworkPriorityStringFallback(t *testing.T) {
	got := bacnet.NetworkPriority(0xFF).String()
	want := "network-priority(255)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestMaxSegmentsAcceptedStringFallback(t *testing.T) {
	got := MaxSegmentsAccepted(8).String()
	want := "max-segments(8)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestConfirmResultStringFallback(t *testing.T) {
	got := ConfirmResult(10).String()
	want := "confirm-result(10)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
