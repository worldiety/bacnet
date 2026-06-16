package apdu

import (
	"errors"
	"testing"
)

func TestEncodeDecodeConfirmedRequestControlFields(t *testing.T) {
	in := outboundAPDU{
		Type:                      PDUTypeConfirmedRequest,
		InvokeID:                  42,
		ServiceChoice:             ServiceChoiceReadProperty,
		SegmentedResponseAccepted: true,
		MaxSegmentsAccepted:       MaxSegments16,
		MaxAPDULengthAccepted:     1476,
		Payload:                   []byte{0xAA, 0xBB},
	}

	wire, err := encodeAPDU(in)
	if err != nil {
		t.Fatalf("encodeAPDU() error = %v", err)
	}

	got, err := decodeAPDU(wire)
	if err != nil {
		t.Fatalf("decodeAPDU() error = %v", err)
	}

	if got.Type != PDUTypeConfirmedRequest {
		t.Fatalf("Type = %v, want %v", got.Type, PDUTypeConfirmedRequest)
	}
	if got.InvokeID != 42 {
		t.Fatalf("InvokeID = %d, want 42", got.InvokeID)
	}
	if got.ServiceChoice != ServiceChoiceReadProperty {
		t.Fatalf("ServiceChoice = %v, want %v", got.ServiceChoice, ServiceChoiceReadProperty)
	}
	if !got.SegmentedResponseAccepted {
		t.Fatalf("SegmentedResponseAccepted = false, want true")
	}
	if got.MaxSegmentsAccepted != MaxSegments16 {
		t.Fatalf("MaxSegmentsAccepted = %v, want %v", got.MaxSegmentsAccepted, MaxSegments16)
	}
	if got.MaxAPDULengthAccepted != 1476 {
		t.Fatalf("MaxAPDULengthAccepted = %d, want 1476", got.MaxAPDULengthAccepted)
	}
	if len(got.Payload) != 2 || got.Payload[0] != 0xAA || got.Payload[1] != 0xBB {
		t.Fatalf("Payload = %v, want [170 187]", got.Payload)
	}
}

func TestDecodeConfirmedRequestSegmented(t *testing.T) {
	tests := []struct {
		name    string
		wire    []byte
		wantErr error
	}{
		{
			// First segment (seq=0) but only 4 bytes — too short (missing seq, window, serviceChoice).
			name:    "first-segment-too-short",
			wire:    []byte{byte(PDUTypeConfirmedRequest<<4) | confirmedRequestFlagSegmentedMessage, 0x00, 0x01, byte(ServiceChoiceReadProperty)},
			wantErr: ErrDecodeFailure,
		},
		{
			// Continuation segment (seq=1) with only 4 bytes — too short (missing seq, window fields).
			name:    "continuation-segment-too-short",
			wire:    []byte{byte(PDUTypeConfirmedRequest<<4) | confirmedRequestFlagSegmentedMessage | confirmedRequestFlagMoreFollows, 0x00, 0x01, 0x01},
			wantErr: ErrDecodeFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeAPDU(tt.wire)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("decodeAPDU() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeConfirmedRequestSegmentedFirstSegment(t *testing.T) {
	// First segment: flags, maxSeg/maxAPDU, invokeID, seqNum=0, window=4, serviceChoice, payload.
	wire := []byte{
		byte(PDUTypeConfirmedRequest<<4) | confirmedRequestFlagSegmentedMessage | confirmedRequestFlagMoreFollows,
		0x45,                            // MaxSegments=4, MaxAPDU=1476
		0x07,                            // invokeID
		0x00,                            // SequenceNumber=0
		0x04,                            // ProposedWindowSize=4
		byte(ServiceChoiceReadProperty), // ServiceChoice
		0xDE, 0xAD,                      // payload
	}
	got, err := decodeAPDU(wire)
	if err != nil {
		t.Fatalf("decodeAPDU() error = %v", err)
	}
	if got.Type != PDUTypeConfirmedRequest {
		t.Fatalf("Type = %v, want confirmed-request", got.Type)
	}
	if !got.SegmentedMessage {
		t.Fatal("SegmentedMessage = false, want true")
	}
	if !got.MoreFollows {
		t.Fatal("MoreFollows = false, want true")
	}
	if got.SequenceNumber != 0 {
		t.Fatalf("SequenceNumber = %d, want 0", got.SequenceNumber)
	}
	if got.ProposedWindowSize != 4 {
		t.Fatalf("ProposedWindowSize = %d, want 4", got.ProposedWindowSize)
	}
	if got.InvokeID != 7 {
		t.Fatalf("InvokeID = %d, want 7", got.InvokeID)
	}
	if got.ServiceChoice != ServiceChoiceReadProperty {
		t.Fatalf("ServiceChoice = %v, want read-property", got.ServiceChoice)
	}
	if len(got.Payload) != 2 || got.Payload[0] != 0xDE || got.Payload[1] != 0xAD {
		t.Fatalf("Payload = %v, want [0xDE 0xAD]", got.Payload)
	}
}

func TestEncodeRejectAndAbortWithoutServiceChoiceField(t *testing.T) {
	tests := []struct {
		name       string
		in         outboundAPDU
		wantServer bool
	}{
		{
			name: "reject",
			in: outboundAPDU{
				Type:     PDUTypeReject,
				InvokeID: 7,
				Payload:  []byte{0x99},
			},
			wantServer: false,
		},
		{
			name: "abort",
			in: outboundAPDU{
				Type:     PDUTypeAbort,
				Server:   true,
				InvokeID: 8,
				Payload:  []byte{0x22},
			},
			wantServer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := encodeAPDU(tt.in)
			if err != nil {
				t.Fatalf("encodeAPDU() error = %v", err)
			}
			if len(wire) != 3 {
				t.Fatalf("wire length = %d, want 3", len(wire))
			}

			got, err := decodeAPDU(wire)
			if err != nil {
				t.Fatalf("decodeAPDU() error = %v", err)
			}
			if got.ServiceChoice != 0 {
				t.Fatalf("ServiceChoice = %v, want 0 for %s", got.ServiceChoice, tt.name)
			}

			if got.Server != tt.wantServer {
				t.Fatalf("Server = %v, want %v for %s", got.Server, tt.wantServer, tt.name)
			}

			if len(got.Payload) != 1 || got.Payload[0] != tt.in.Payload[0] {
				t.Fatalf("Payload = %v, want [%d]", got.Payload, tt.in.Payload[0])
			}
		})
	}
}

func TestEncodeDecodeSegmentedComplexACK(t *testing.T) {
	tests := []struct {
		name              string
		in                outboundAPDU
		wantServiceChoice ServiceChoice
		wantPayload       []byte
	}{
		{
			name: "first segment",
			in: outboundAPDU{
				Type:             PDUTypeComplexACK,
				SegmentedMessage: true,
				MoreFollows:      true,
				InvokeID:         9,
				SequenceNumber:   0,
				ActualWindowSize: 1,
				ServiceChoice:    ServiceChoiceReadProperty,
				Payload:          []byte{0xAA, 0xBB},
			},
			wantServiceChoice: ServiceChoiceReadProperty,
			wantPayload:       []byte{0xAA, 0xBB},
		},
		{
			name: "continuation segment",
			in: outboundAPDU{
				Type:             PDUTypeComplexACK,
				SegmentedMessage: true,
				MoreFollows:      false,
				InvokeID:         9,
				SequenceNumber:   1,
				ActualWindowSize: 1,
				ServiceChoice:    ServiceChoiceReadProperty,
				Payload:          []byte{0xCC},
			},
			wantServiceChoice: 0,
			wantPayload:       []byte{0xCC},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := encodeAPDU(tt.in)
			if err != nil {
				t.Fatalf("encodeAPDU() error = %v", err)
			}

			got, err := decodeAPDU(wire)
			if err != nil {
				t.Fatalf("decodeAPDU() error = %v", err)
			}

			if got.Type != PDUTypeComplexACK {
				t.Fatalf("Type = %v, want %v", got.Type, PDUTypeComplexACK)
			}

			if !got.SegmentedMessage {
				t.Fatal("SegmentedMessage = false, want true")
			}

			if got.MoreFollows != tt.in.MoreFollows {
				t.Fatalf("MoreFollows = %v, want %v", got.MoreFollows, tt.in.MoreFollows)
			}

			if got.InvokeID != tt.in.InvokeID {
				t.Fatalf("InvokeID = %d, want %d", got.InvokeID, tt.in.InvokeID)
			}

			if got.SequenceNumber != tt.in.SequenceNumber {
				t.Fatalf("SequenceNumber = %d, want %d", got.SequenceNumber, tt.in.SequenceNumber)
			}

			if got.ProposedWindowSize != tt.in.ActualWindowSize {
				t.Fatalf("WindowSize = %d, want %d", got.ProposedWindowSize, tt.in.ActualWindowSize)
			}

			if got.ServiceChoice != tt.wantServiceChoice {
				t.Fatalf("ServiceChoice = %v, want %v", got.ServiceChoice, tt.wantServiceChoice)
			}

			if string(got.Payload) != string(tt.wantPayload) {
				t.Fatalf("Payload = %v, want %v", got.Payload, tt.wantPayload)
			}
		})
	}
}

func TestDecodeSegmentACKFlags(t *testing.T) {
	got, err := decodeAPDU([]byte{byte(PDUTypeSegmentACK<<4) | 0x03, 0x11, 0x02, 0x04})
	if err != nil {
		t.Fatalf("decodeAPDU() error = %v", err)
	}

	if !got.NegativeAck {
		t.Fatal("NegativeAck = false, want true")
	}

	if !got.Server {
		t.Fatal("Server = false, want true")
	}

	if got.SequenceNumber != 2 {
		t.Fatalf("SequenceNumber = %d, want 2", got.SequenceNumber)
	}

	if got.ActualWindowSize != 4 {
		t.Fatalf("ActualWindowSize = %d, want 4", got.ActualWindowSize)
	}
}
