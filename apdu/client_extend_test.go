package apdu

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.wdy.de/bacnet"
	"go.wdy.de/bacnet/npdu"
)

func TestClientDeviceManagementSimpleACK(t *testing.T) {
	tests := []struct {
		name    string
		service ServiceChoice
		invoke  func(*clientImpl, context.Context, bacnet.Address) error
	}{
		{
			name:    "device-communication-control",
			service: ServiceChoiceDeviceCommunicationControl,
			invoke: func(c *clientImpl, ctx context.Context, dst bacnet.Address) error {
				duration := uint16(60)
				password := "secret"
				req := DeviceCommunicationControlRequest{
					TimeDurationMinutes: &duration,
					EnableDisable:       DeviceCommunicationControlDisable,
					Password:            &password,
				}
				return c.DeviceCommunicationControl(ctx, dst, req)
			},
		},
		{
			name:    "reinitialize-device",
			service: ServiceChoiceReinitializeDevice,
			invoke: func(c *clientImpl, ctx context.Context, dst bacnet.Address) error {
				password := "secret"
				req := ReinitializeDeviceRequest{State: ReinitializeDeviceStateWarmStart, Password: &password}
				return c.ReinitializeDevice(ctx, dst, req)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newTestNPDUTransport()
			ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
			clientRaw, err := NewClient(ase, ClientConfig{})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			client := clientRaw.(*clientImpl)
			dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})

			ch := make(chan error, 1)
			go func() { ch <- tt.invoke(client, context.Background(), dst) }()

			sent := <-transport.ch
			outbound, err := decodeAPDU(sent.packet.APDUBytes())
			if err != nil {
				t.Fatalf("decodeAPDU: %v", err)
			}
			if outbound.ServiceChoice != tt.service {
				t.Fatalf("service = %v, want %v", outbound.ServiceChoice, tt.service)
			}

			ackBytes, err := encodeAPDU(outboundAPDU{Type: PDUTypeSimpleACK, InvokeID: outbound.InvokeID, ServiceChoice: tt.service})
			if err != nil {
				t.Fatalf("encodeAPDU: %v", err)
			}
			ackNPDU, _ := npdu.NewLocalAPDU(bacnet.NetworkPriorityNormal, false, ackBytes)
			if err := ase.OnInboundNPDU(context.Background(), dst, *ackNPDU); err != nil {
				t.Fatalf("OnInboundNPDU: %v", err)
			}

			if err := <-ch; err != nil {
				t.Fatalf("call error: %v", err)
			}
		})
	}
}

func TestClientRemoteErrorMapping(t *testing.T) {
	type methodCall struct {
		service ServiceChoice
		invoke  func(*clientImpl, context.Context, bacnet.Address) error
	}

	calls := []methodCall{
		{
			service: ServiceChoiceReadRange,
			invoke: func(c *clientImpl, ctx context.Context, dst bacnet.Address) error {
				objID, _ := bacnet.NewObjectIdentifier(bacnet.ObjectTypeAnalogInput, 7)
				req := ReadRangeRequest{
					ObjectIdentifier:   objID,
					PropertyIdentifier: bacnet.PropertyIdentifierPresentValue,
					ByPosition:         &ReadRangeByPosition{ReferenceIndex: 1, Count: 1},
				}
				_, err := c.ReadRange(ctx, dst, req)
				return err
			},
		},
		{
			service: ServiceChoiceDeviceCommunicationControl,
			invoke: func(c *clientImpl, ctx context.Context, dst bacnet.Address) error {
				req := DeviceCommunicationControlRequest{EnableDisable: DeviceCommunicationControlEnable}
				return c.DeviceCommunicationControl(ctx, dst, req)
			},
		},
		{
			service: ServiceChoiceReinitializeDevice,
			invoke: func(c *clientImpl, ctx context.Context, dst bacnet.Address) error {
				req := ReinitializeDeviceRequest{State: ReinitializeDeviceStateColdStart}
				return c.ReinitializeDevice(ctx, dst, req)
			},
		},
	}

	tests := []struct {
		name      string
		pduType   PDUType
		wantIsErr error
		wantType  any
	}{
		{name: "error", pduType: PDUTypeError, wantIsErr: ErrRemoteError, wantType: RemoteErrorAPDU{}},
		{name: "reject", pduType: PDUTypeReject, wantIsErr: ErrRemoteReject, wantType: RemoteRejectAPDU{}},
		{name: "abort", pduType: PDUTypeAbort, wantIsErr: ErrRemoteAbort, wantType: RemoteAbortAPDU{}},
	}

	for _, call := range calls {
		for _, tt := range tests {
			t.Run(call.service.String()+"/"+tt.name, func(t *testing.T) {
				transport := newTestNPDUTransport()
				ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
				clientRaw, err := NewClient(ase, ClientConfig{})
				if err != nil {
					t.Fatalf("NewClient: %v", err)
				}
				client := clientRaw.(*clientImpl)
				dst, _ := bacnet.NewAddress(bacnet.LocalNetwork, []byte{0x01})

				ch := make(chan error, 1)
				go func() { ch <- call.invoke(client, context.Background(), dst) }()

				sent := <-transport.ch
				outbound, err := decodeAPDU(sent.packet.APDUBytes())
				if err != nil {
					t.Fatalf("decodeAPDU: %v", err)
				}

				payload := []byte{0x01}
				if tt.pduType == PDUTypeError {
					payload = []byte{0x01, 0x02}
				}
				inboundBytes, err := encodeAPDU(outboundAPDU{Type: tt.pduType, InvokeID: outbound.InvokeID, ServiceChoice: call.service, Payload: payload})
				if err != nil {
					t.Fatalf("encodeAPDU: %v", err)
				}
				inbound, _ := npdu.NewLocalAPDU(bacnet.NetworkPriorityNormal, false, inboundBytes)
				if err := ase.OnInboundNPDU(context.Background(), dst, *inbound); err != nil {
					t.Fatalf("OnInboundNPDU: %v", err)
				}

				err = <-ch
				if !errors.Is(err, tt.wantIsErr) {
					t.Fatalf("err = %v, want errors.Is(_, %v)", err, tt.wantIsErr)
				}
				switch tt.wantType.(type) {
				case RemoteErrorAPDU:
					var typed RemoteErrorAPDU
					if !errors.As(err, &typed) {
						t.Fatalf("expected RemoteErrorAPDU, got %v", err)
					}
				case RemoteRejectAPDU:
					var typed RemoteRejectAPDU
					if !errors.As(err, &typed) {
						t.Fatalf("expected RemoteRejectAPDU, got %v", err)
					}
				case RemoteAbortAPDU:
					var typed RemoteAbortAPDU
					if !errors.As(err, &typed) {
						t.Fatalf("expected RemoteAbortAPDU, got %v", err)
					}
				}
			})
		}
	}
}
