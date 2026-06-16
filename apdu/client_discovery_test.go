package apdu

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.wdy.de/bacnet/common/netprim"
	"go.wdy.de/bacnet/common/types"
	"go.wdy.de/bacnet/npdu"
)

func TestDiscoverTimeoutWindow(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)
	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})

	type result struct {
		items []IAmIndication
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		items, discoverErr := client.Discover(context.Background(), DiscoverRequest{Destination: dst, WhoIs: WhoIsRequest{}, Window: 30 * time.Millisecond})
		ch <- result{items: items, err: discoverErr}
	}()

	sent := <-transport.ch
	apdu, err := decodeAPDU(sent.packet.APDUBytes())
	if err != nil {
		t.Fatalf("decodeAPDU: %v", err)
	}
	if apdu.ServiceChoice != ServiceChoiceWhoIs {
		t.Fatalf("service choice = %v, want %v", apdu.ServiceChoice, ServiceChoiceWhoIs)
	}

	res := <-ch
	if res.err != nil {
		t.Fatalf("Discover: %v", res.err)
	}
	if len(res.items) != 0 {
		t.Fatalf("items len = %d, want 0", len(res.items))
	}
}

func TestDiscoverDeduplicatesByDeviceAndSource(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)
	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})

	type result struct {
		items []IAmIndication
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		items, discoverErr := client.Discover(context.Background(), DiscoverRequest{Destination: dst, WhoIs: WhoIsRequest{}, Window: 120 * time.Millisecond})
		ch <- result{items: items, err: discoverErr}
	}()

	<-transport.ch // Who-Is outbound

	srcA, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x0A})
	srcB, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x0B})
	dev1, _ := types.NewObjectIdentifier(types.ObjectTypeDevice, 1001)
	dev2, _ := types.NewObjectIdentifier(types.ObjectTypeDevice, 1002)

	sendIAmInboundForDiscoveryTest(t, ase, srcA, dev1)
	sendIAmInboundForDiscoveryTest(t, ase, srcA, dev1) // duplicate
	sendIAmInboundForDiscoveryTest(t, ase, srcB, dev2)

	res := <-ch
	if res.err != nil {
		t.Fatalf("Discover: %v", res.err)
	}
	if len(res.items) != 2 {
		t.Fatalf("items len = %d, want 2", len(res.items))
	}
}

func TestDiscoverCancellation(t *testing.T) {
	transport := newTestNPDUTransport()
	ase, _ := NewASE(ASEConfig{InvokeTimeout: time.Second, MaxConcurrentInvokes: 4}, transport)
	clientRaw, err := NewClient(ase, ClientConfig{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client := clientRaw.(*clientImpl)
	dst, _ := netprim.NewAddress(netprim.LocalNetwork, []byte{0x01})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		items []IAmIndication
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		items, discoverErr := client.Discover(ctx, DiscoverRequest{Destination: dst, WhoIs: WhoIsRequest{}, Window: time.Second})
		ch <- result{items: items, err: discoverErr}
	}()

	<-transport.ch // Who-Is outbound
	cancel()

	res := <-ch
	if !errors.Is(res.err, context.Canceled) {
		t.Fatalf("Discover err = %v, want %v", res.err, context.Canceled)
	}
}

func sendIAmInboundForDiscoveryTest(t *testing.T, ase ASE, src netprim.Address, deviceID types.ObjectIdentifier) {
	t.Helper()
	payload := encodeIAmPayloadForTest(deviceID, 1476, SegmentationSupportNo, 117)
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
}
