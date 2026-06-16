package types

import (
	"errors"
	"testing"

	bacneterrors "go.wdy.de/bacnet/common/errors"
	"go.wdy.de/bacnet/common/netprim"
)

func TestNewDeviceInstance(t *testing.T) {
	tests := []struct {
		name    string
		input   uint32
		wantErr error
	}{
		{name: "valid", input: 42},
		{name: "max", input: netprim.MaxInstanceNumber},
		{name: "invalid", input: netprim.MaxInstanceNumber + 1, wantErr: bacneterrors.ErrInvalidDeviceInstance},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDeviceInstance(tt.input)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("NewDeviceInstance(%d) error = %v", tt.input, err)
				}
				if uint32(got) != tt.input {
					t.Fatalf("NewDeviceInstance(%d) = %d", tt.input, got)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewDeviceInstance(%d) error = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestDeviceInstanceValid(t *testing.T) {
	if !DeviceInstance(netprim.MaxInstanceNumber).Valid() {
		t.Fatal("max valid device instance should be valid")
	}

	if DeviceInstance(netprim.MaxInstanceNumber + 1).Valid() {
		t.Fatal("out-of-range device instance should be invalid")
	}
}

func TestNewObjectIdentifier(t *testing.T) {
	id, err := NewObjectIdentifier(ObjectTypeDevice, 1234)
	if err != nil {
		t.Fatalf("NewObjectIdentifier returned error: %v", err)
	}

	if got := id.ObjectType(); got != ObjectTypeDevice {
		t.Fatalf("ObjectType() = %v, want %v", got, ObjectTypeDevice)
	}

	if got := id.Instance(); got != 1234 {
		t.Fatalf("Instance() = %d, want 1234", got)
	}

	if got := id.String(); got != "device,1234" {
		t.Fatalf("String() = %q, want %q", got, "device,1234")
	}
}

func TestNewObjectIdentifierRejectsInvalidInput(t *testing.T) {
	if _, err := NewObjectIdentifier(ObjectTypeMax+1, 1); !errors.Is(err, bacneterrors.ErrInvalidObjectType) {
		t.Fatalf("expected ErrInvalidObjectType, got %v", err)
	}

	if _, err := NewObjectIdentifier(ObjectTypeDevice, netprim.MaxInstanceNumber+1); !errors.Is(err, bacneterrors.ErrInvalidObjectInstance) {
		t.Fatalf("expected ErrInvalidObjectInstance, got %v", err)
	}
}

func TestPropertyIdentifierString(t *testing.T) {
	tests := []struct {
		name  string
		input PropertyIdentifier
		want  string
	}{
		{name: "acked transitions", input: PropertyIdentifierAckedTransitions, want: "acked-transitions"},
		{name: "application software version", input: PropertyIdentifierApplicationSoftwareVersion, want: "application-software-version"},
		{name: "description", input: PropertyIdentifierDescription, want: "description"},
		{name: "object identifier", input: PropertyIdentifierObjectIdentifier, want: "object-identifier"},
		{name: "object name", input: PropertyIdentifierObjectName, want: "object-name"},
		{name: "object type", input: PropertyIdentifierObjectType, want: "object-type"},
		{name: "present value", input: PropertyIdentifierPresentValue, want: "present-value"},
		{name: "protocol revision", input: PropertyIdentifierProtocolRevision, want: "protocol-revision"},
		{name: "protocol version", input: PropertyIdentifierProtocolVersion, want: "protocol-version"},
		{name: "status flags", input: PropertyIdentifierStatusFlags, want: "status-flags"},
		{name: "units", input: PropertyIdentifierUnits, want: "units"},
		{name: "vendor name", input: PropertyIdentifierVendorName, want: "vendor-name"},
		{name: "fallback", input: PropertyIdentifier(9999), want: "property-identifier(9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNetworkNumberHelpers(t *testing.T) {
	if !netprim.LocalNetwork.IsLocal() {
		t.Fatal("LocalNetwork should be local")
	}

	if !netprim.GlobalBroadcastNetwork.IsGlobalBroadcast() {
		t.Fatal("GlobalBroadcastNetwork should be a global broadcast")
	}
}

func TestObjectTypeString(t *testing.T) {
	tests := []struct {
		name  string
		input ObjectType
		want  string
	}{
		{name: "analog input", input: ObjectTypeAnalogInput, want: "analog-input"},
		{name: "analog output", input: ObjectTypeAnalogOutput, want: "analog-output"},
		{name: "analog value", input: ObjectTypeAnalogValue, want: "analog-value"},
		{name: "binary input", input: ObjectTypeBinaryInput, want: "binary-input"},
		{name: "binary output", input: ObjectTypeBinaryOutput, want: "binary-output"},
		{name: "binary value", input: ObjectTypeBinaryValue, want: "binary-value"},
		{name: "device", input: ObjectTypeDevice, want: "device"},
		{name: "file", input: ObjectTypeFile, want: "file"},
		{name: "loop", input: ObjectTypeLoop, want: "loop"},
		{name: "multi state input", input: ObjectTypeMultiStateInput, want: "multi-state-input"},
		{name: "multi state output", input: ObjectTypeMultiStateOutput, want: "multi-state-output"},
		{name: "notification class", input: ObjectTypeNotificationClass, want: "notification-class"},
		{name: "fallback", input: ObjectType(2048), want: "object-type(2048)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
