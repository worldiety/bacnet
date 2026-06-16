package router

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"go.wdy.de/bacnet/common/netprim"
	"go.wdy.de/bacnet/npdu"
)

// learnedRouteTTL is a convenience TTL used by tests that add learned routes
// and do not exercise expiry behaviour.
const learnedRouteTTL = time.Hour

func TestEvaluateLocalAPDU(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	pdu, err := npdu.NewLocalAPDU(netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewLocalAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !decision.DeliverLocally {
		t.Fatal("DeliverLocally = false, want true")
	}

	if len(decision.Forwards) != 0 {
		t.Fatalf("len(Forwards) = %d, want 0", len(decision.Forwards))
	}

	if decision.DropReason != DropReasonNone {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonNone)
	}
}

func TestEvaluateForwardRoutedAPDU(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// Use a learned route so the forward is a transit (not a local-delivery) forward.
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10, 0x20})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.DeliverLocally {
		t.Fatal("DeliverLocally = true, want false")
	}
	if len(decision.Forwards) != 1 {
		t.Fatalf("len(Forwards) = %d, want 1", len(decision.Forwards))
	}
	if decision.Forwards[0].OutPort != 2 {
		t.Fatalf("OutPort = %d, want 2", decision.Forwards[0].OutPort)
	}
	if hc := decision.Forwards[0].NPDU.HopCount(); hc == nil || *hc != 4 {
		t.Fatalf("HopCount = %v, want 4", hc)
	}
	if got := decision.Forwards[0].NPDU.ApplicationPayloadBytes(); !bytes.Equal(got, []byte{0x10, 0x20}) {
		t.Fatalf("payload = %#v, want %#v", got, []byte{0x10, 0x20})
	}
}

func TestEvaluateDropsExpiredHopCount(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// Use a learned (transit) route: hop-count expiry is only meaningful for transit hops.
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 1, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.DropReason != DropReasonHopCountExpired {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonHopCountExpired)
	}
	if len(decision.Forwards) != 0 {
		t.Fatalf("len(Forwards) = %d, want 0", len(decision.Forwards))
	}
}

func TestEvaluateUnknownDestination(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.DropReason != DropReasonUnknownDestination {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonUnknownDestination)
	}
}

func TestEvaluateSuppressesSamePortForwarding(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddConnectedRoute(1, 100, nil); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.DropReason != DropReasonSamePort {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonSamePort)
	}
}

func TestEvaluateGlobalBroadcastFanout(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddConnectedRoute(1, 100, nil); err != nil {
		t.Fatalf("AddConnectedRoute(100): %v", err)
	}
	if err := r.AddConnectedRoute(2, 200, nil); err != nil {
		t.Fatalf("AddConnectedRoute(200): %v", err)
	}
	if err := r.AddConnectedRoute(3, 300, nil); err != nil {
		t.Fatalf("AddConnectedRoute(300): %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(npdu.UltimateDestinationNetworkNumber(netprim.GlobalBroadcastNetwork), nil, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.DeliverLocally {
		t.Fatal("DeliverLocally = false, want true")
	}
	if len(decision.Forwards) != 2 {
		t.Fatalf("len(Forwards) = %d, want 2", len(decision.Forwards))
	}
	if decision.Forwards[0].OutPort != 2 || decision.Forwards[1].OutPort != 3 {
		t.Fatalf("OutPorts = [%d %d], want [2 3]", decision.Forwards[0].OutPort, decision.Forwards[1].OutPort)
	}
	// Connected-port forwards are local-delivery: destination specifier is stripped and
	// there is no hop count on the outbound NPDU.
	for _, forward := range decision.Forwards {
		if forward.Kind != ForwardKindLocalDelivery {
			t.Fatalf("forward to port %d: Kind = %v, want %v", forward.OutPort, forward.Kind, ForwardKindLocalDelivery)
		}
		if forward.NPDU.HasDestinationSpecifier() {
			t.Fatalf("forward to port %d: has destination specifier, want none (local delivery)", forward.OutPort)
		}
		if forward.NPDU.HopCount() != nil {
			t.Fatalf("forward to port %d: HopCount != nil, want nil (destination stripped)", forward.OutPort)
		}
	}
}

func TestEvaluateNetworkLayerMessagePreservesHeaderAndPayload(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// Use a learned route so transit forwarding (with hop-count decrement) is exercised.
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	pdu, err := npdu.NewNetworkLayerNPDU(
		npdu.NPCI{
			Priority: netprim.NetworkPriorityNormal,
			Destination: &npdu.DestinationSpecifier{
				DNET:     npdu.UltimateDestinationNetworkNumber(100),
				DADR:     []byte{0xAA},
				HopCount: 3,
			},
		},
		npdu.NetworkLayerMessageHeader{MessageType: npdu.NetworkLayerMessageTypeNetworkNumberIs},
		[]byte{0x00, 0x64, 0x01},
	)
	if err != nil {
		t.Fatalf("NewNetworkLayerNPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.DeliverLocally {
		t.Fatal("DeliverLocally = false, want true for network-layer message")
	}
	if len(decision.Forwards) != 1 {
		t.Fatalf("len(Forwards) = %d, want 1", len(decision.Forwards))
	}
	forwarded := decision.Forwards[0].NPDU
	if !forwarded.IsNetworkLayerMessage() {
		t.Fatal("forwarded.IsNetworkLayerMessage() = false, want true")
	}
	header := forwarded.MustNetworkLayerMessageHeader()
	if header.MessageType != npdu.NetworkLayerMessageTypeNetworkNumberIs {
		t.Fatalf("MessageType = %v, want %v", header.MessageType, npdu.NetworkLayerMessageTypeNetworkNumberIs)
	}
	if got := forwarded.NetworkLayerPayloadBytes(); !bytes.Equal(got, []byte{0x00, 0x64, 0x01}) {
		t.Fatalf("payload = %#v, want %#v", got, []byte{0x00, 0x64, 0x01})
	}
	if hc := forwarded.HopCount(); hc == nil || *hc != 2 {
		t.Fatalf("HopCount = %v, want 2", hc)
	}
}

func TestEvaluateRejectsInvalidNPDU(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	_, err = r.Evaluate(1, &npdu.NetworkLayerProtocolDataUnit{})
	if !errors.Is(err, ErrInvalidNPDU) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidNPDU)
	}
}

// --- hop-count exhaustion ---

// TestEvaluateHopCountExhaustedGeneratesReject verifies that when a transit forward
// is suppressed due to hop-count expiry the router populates Decision.RejectResponse
// with a Reject-Message-To-Network NLM (clause 6.6.3).
func TestEvaluateHopCountExhaustedGeneratesReject(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// Use a learned (transit) route so the hop-count path is exercised.
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	// hopCount=1 → after decrement it would be 0, so it is dropped.
	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 1, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.DropReason != DropReasonHopCountExpired {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonHopCountExpired)
	}
	if decision.RejectResponse == nil {
		t.Fatal("RejectResponse = nil, want non-nil Reject-Message-To-Network")
	}

	// Verify the reject NPDU decodes to the expected NLM.
	model, err := decision.RejectResponse.NetworkLayerMessageModel()
	if err != nil {
		t.Fatalf("NetworkLayerMessageModel: %v", err)
	}
	reject, ok := model.(npdu.RejectMessageToNetworkMessage)
	if !ok {
		t.Fatalf("model type = %T, want RejectMessageToNetworkMessage", model)
	}
	if reject.DNET != 100 {
		t.Fatalf("reject.DNET = %d, want 100", reject.DNET)
	}
	if reject.Reason != npdu.NLMRejectReasonTooManyHops {
		t.Fatalf("reject.Reason = %v, want %v", reject.Reason, npdu.NLMRejectReasonTooManyHops)
	}
	// Reject NPDU must be a local (non-routed) message.
	if decision.RejectResponse.HasDestinationSpecifier() {
		t.Fatal("RejectResponse has destination specifier, want none (local NLM)")
	}
}

// TestEvaluateNoRejectResponseOnSuccessfulForward verifies that RejectResponse is nil
// for normal transit forwarding.
func TestEvaluateNoRejectResponseOnSuccessfulForward(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.RejectResponse != nil {
		t.Fatal("RejectResponse != nil, want nil for successful forward")
	}
}

// --- local delivery split (connected vs learned routes) ---

// TestEvaluateConnectedDNETDeliversLocally verifies the final-hop local delivery split:
// when DNET maps to a directly connected (RouteKindConnected) route, DeliverLocally is
// set true and the forward carries ForwardKindLocalDelivery.
func TestEvaluateConnectedDNETDeliversLocally(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddConnectedRoute(2, 100, nil); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10, 0x20})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.DeliverLocally {
		t.Fatal("DeliverLocally = false, want true for connected DNET")
	}
	if len(decision.Forwards) != 1 {
		t.Fatalf("len(Forwards) = %d, want 1", len(decision.Forwards))
	}
	if decision.Forwards[0].Kind != ForwardKindLocalDelivery {
		t.Fatalf("Forward.Kind = %v, want %v", decision.Forwards[0].Kind, ForwardKindLocalDelivery)
	}
	if decision.Forwards[0].OutPort != 2 {
		t.Fatalf("OutPort = %d, want 2", decision.Forwards[0].OutPort)
	}
}

// TestEvaluateConnectedDNETStripsDestinationSpecifier verifies that the NPDU produced
// for a connected-route forward has no destination specifier (routing header stripped).
func TestEvaluateConnectedDNETStripsDestinationSpecifier(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddConnectedRoute(2, 100, nil); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0xFF})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	delivered := decision.Forwards[0].NPDU
	if delivered.HasDestinationSpecifier() {
		t.Fatal("local-delivery NPDU has destination specifier, want none")
	}
	if got := delivered.ApplicationPayloadBytes(); !bytes.Equal(got, []byte{0xFF}) {
		t.Fatalf("payload = %#v, want %#v", got, []byte{0xFF})
	}
}

// TestEvaluateConnectedDNETPreservesSourceSpecifier verifies that the source specifier
// (SNET/SADR) is preserved in the local-delivery NPDU so the receiving device can
// identify the original sender.
func TestEvaluateConnectedDNETPreservesSourceSpecifier(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddConnectedRoute(2, 100, nil); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedSourcedAPDU(
		100, []byte{0xAA}, 5,
		200, []byte{0xBB},
		netprim.NetworkPriorityNormal, false, []byte{0xFF},
	)
	if err != nil {
		t.Fatalf("NewRoutedSourcedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	delivered := decision.Forwards[0].NPDU
	if !delivered.HasSourceSpecifier() {
		t.Fatal("local-delivery NPDU has no source specifier, want preserved")
	}
	snet := delivered.SNET()
	if snet == nil || *snet != 200 {
		t.Fatalf("SNET = %v, want 200", snet)
	}
	if got := delivered.SADR(); !bytes.Equal(got, []byte{0xBB}) {
		t.Fatalf("SADR = %#v, want %#v", got, []byte{0xBB})
	}
}

// TestEvaluateTransitForwardKindIsTransit verifies that forwarding via a learned route
// produces ForwardKindTransit.
func TestEvaluateTransitForwardKindIsTransit(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(decision.Forwards) != 1 {
		t.Fatalf("len(Forwards) = %d, want 1", len(decision.Forwards))
	}
	if decision.Forwards[0].Kind != ForwardKindTransit {
		t.Fatalf("Forward.Kind = %v, want %v", decision.Forwards[0].Kind, ForwardKindTransit)
	}
	// Transit forward must preserve the destination specifier.
	if !decision.Forwards[0].NPDU.HasDestinationSpecifier() {
		t.Fatal("transit NPDU has no destination specifier, want preserved")
	}
}

// --- SNET-based loop prevention ---

// TestEvaluateLoopSuppressionBySNET verifies that a unicast forward is suppressed when
// the candidate egress port serves the source network of the packet.
func TestEvaluateLoopSuppressionBySNET(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// Network 100 is on port 1 (ingress). Network 200 is on port 2.
	// Network 300 (source) is also on port 2 — forwarding to port 2 should be suppressed.
	if err := r.AddConnectedRoute(1, 100, nil); err != nil {
		t.Fatalf("AddConnectedRoute(100 on port 1): %v", err)
	}
	if err := r.AddLearnedRoute(2, 200, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute(200 on port 2): %v", err)
	}
	if err := r.AddLearnedRoute(2, 300, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute(300 on port 2): %v", err)
	}

	// Packet destined for network 200, sourced from network 300 — both on port 2.
	pdu, err := npdu.NewRoutedSourcedAPDU(
		200, []byte{0xBB}, 5,
		300, []byte{0xCC},
		netprim.NetworkPriorityNormal, false, []byte{0x10},
	)
	if err != nil {
		t.Fatalf("NewRoutedSourcedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if decision.DropReason != DropReasonLoopSuppressed {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonLoopSuppressed)
	}

	if len(decision.Forwards) != 0 {
		t.Fatalf("len(Forwards) = %d, want 0", len(decision.Forwards))
	}
}

// TestEvaluateGlobalBroadcastLoopSuppression verifies that the global-broadcast fanout
// omits the port that serves the source network of the packet.
func TestEvaluateGlobalBroadcastLoopSuppression(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	// Three ports: 1 = ingress (network 100), 2 = network 200, 3 = network 300.
	// Packet source is network 200 (port 2) → port 2 must be omitted from fanout.
	if err := r.AddConnectedRoute(1, 100, nil); err != nil {
		t.Fatalf("AddConnectedRoute(100): %v", err)
	}
	if err := r.AddConnectedRoute(2, 200, nil); err != nil {
		t.Fatalf("AddConnectedRoute(200): %v", err)
	}
	if err := r.AddConnectedRoute(3, 300, nil); err != nil {
		t.Fatalf("AddConnectedRoute(300): %v", err)
	}

	pdu, err := npdu.NewRoutedSourcedAPDU(
		npdu.UltimateDestinationNetworkNumber(netprim.GlobalBroadcastNetwork), nil, 5,
		200, []byte{0xBB},
		netprim.NetworkPriorityNormal, false, []byte{0x10},
	)
	if err != nil {
		t.Fatalf("NewRoutedSourcedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.DeliverLocally {
		t.Fatal("DeliverLocally = false, want true for global broadcast")
	}
	// Only port 3 should receive a forward (port 1 is ingress, port 2 is suppressed as source).
	if len(decision.Forwards) != 1 {
		t.Fatalf("len(Forwards) = %d, want 1", len(decision.Forwards))
	}
	if decision.Forwards[0].OutPort != 3 {
		t.Fatalf("OutPort = %d, want 3", decision.Forwards[0].OutPort)
	}
}

// TestEvaluateGlobalBroadcastHopCountExpiredSkipsPort verifies that a global broadcast
// whose hop count is exhausted omits forwarding silently (no reject, local delivery unchanged).
func TestEvaluateGlobalBroadcastHopCountExpiredSkipsPort(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddLearnedRoute(2, 200, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute(200): %v", err)
	}

	// hopCount=1 → would expire on decrement; global broadcast should just skip forwarding.
	pdu, err := npdu.NewRoutedAPDU(
		npdu.UltimateDestinationNetworkNumber(netprim.GlobalBroadcastNetwork), nil, 1,
		netprim.NetworkPriorityNormal, false, []byte{0x10},
	)
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !decision.DeliverLocally {
		t.Fatal("DeliverLocally = false, want true")
	}
	if len(decision.Forwards) != 0 {
		t.Fatalf("len(Forwards) = %d, want 0 (hop expired, skip silently)", len(decision.Forwards))
	}
	if decision.DropReason != DropReasonNone {
		t.Fatalf("DropReason = %v, want none (global broadcast handles expiry silently)", decision.DropReason)
	}
	if decision.RejectResponse != nil {
		t.Fatal("RejectResponse != nil, want nil for broadcast hop-count expiry")
	}
}

func TestEvaluatePrefersConnectedRouteWhenMultipleRoutesExist(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddLearnedRoute(2, 700, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}
	if err := r.AddConnectedRoute(3, 700, nil); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(700, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(decision.Forwards) != 1 {
		t.Fatalf("len(Forwards) = %d, want 1", len(decision.Forwards))
	}
	if decision.Forwards[0].OutPort != 3 {
		t.Fatalf("OutPort = %d, want 3", decision.Forwards[0].OutPort)
	}
	if decision.Forwards[0].Kind != ForwardKindLocalDelivery {
		t.Fatalf("Kind = %v, want %v", decision.Forwards[0].Kind, ForwardKindLocalDelivery)
	}
}

// --- router-busy reject generation ---

// TestEvaluateRouterBusyDropsUnicastAndGeneratesReject verifies that when Policy.Busy
// returns true for a unicast routed APDU, the router sets DropReasonRouterBusy and
// populates Decision.RejectResponse with a Reject-Message-To-Network NLM carrying
// NLMRejectReasonRouterBusy, without producing any forwards (clause 6.6.4).
func TestEvaluateRouterBusyDropsUnicastAndGeneratesReject(t *testing.T) {
	alwaysBusy := func() bool { return true }
	r, err := NewRouter(Config{Policy: &Policy{
		ForwardGlobalBroadcast: true,
		BusyFunc:               alwaysBusy,
	}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.DropReason != DropReasonRouterBusy {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonRouterBusy)
	}
	if len(decision.Forwards) != 0 {
		t.Fatalf("len(Forwards) = %d, want 0", len(decision.Forwards))
	}
	if decision.RejectResponse == nil {
		t.Fatal("RejectResponse = nil, want non-nil Reject-Message-To-Network")
	}

	model, err := decision.RejectResponse.NetworkLayerMessageModel()
	if err != nil {
		t.Fatalf("NetworkLayerMessageModel: %v", err)
	}
	reject, ok := model.(npdu.RejectMessageToNetworkMessage)
	if !ok {
		t.Fatalf("model type = %T, want RejectMessageToNetworkMessage", model)
	}
	if reject.DNET != 100 {
		t.Fatalf("reject.DNET = %d, want 100", reject.DNET)
	}
	if reject.Reason != npdu.NLMRejectReasonRouterBusy {
		t.Fatalf("reject.Reason = %v, want %v", reject.Reason, npdu.NLMRejectReasonRouterBusy)
	}
	// Reject NPDU must be local (no routing headers).
	if decision.RejectResponse.HasDestinationSpecifier() {
		t.Fatal("RejectResponse has destination specifier, want none (local NLM)")
	}
}

// TestEvaluateRouterBusyDropsGlobalBroadcastWithoutReject verifies that when Policy.Busy
// returns true for a global-broadcast APDU, the router sets DropReasonRouterBusy and
// produces no forwards and no RejectResponse (no single addressable originator for broadcasts).
func TestEvaluateRouterBusyDropsGlobalBroadcastWithoutReject(t *testing.T) {
	alwaysBusy := func() bool { return true }
	r, err := NewRouter(Config{Policy: &Policy{
		ForwardGlobalBroadcast: true,
		BusyFunc:               alwaysBusy,
	}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddConnectedRoute(2, 200, nil); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(
		npdu.UltimateDestinationNetworkNumber(netprim.GlobalBroadcastNetwork), nil, 5,
		netprim.NetworkPriorityNormal, false, []byte{0x10},
	)
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Global broadcast is always delivered locally.
	if !decision.DeliverLocally {
		t.Fatal("DeliverLocally = false, want true for global broadcast")
	}
	if decision.DropReason != DropReasonRouterBusy {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonRouterBusy)
	}
	if len(decision.Forwards) != 0 {
		t.Fatalf("len(Forwards) = %d, want 0 (busy, fan-out suppressed)", len(decision.Forwards))
	}
	// No reject for broadcast: there is no single addressable originator.
	if decision.RejectResponse != nil {
		t.Fatal("RejectResponse != nil, want nil for global-broadcast busy drop")
	}
}

// TestEvaluateNotBusyForwardsNormally verifies that when Policy.Busy returns false
// the router proceeds with normal transit forwarding.
func TestEvaluateNotBusyForwardsNormally(t *testing.T) {
	neverBusy := func() bool { return false }
	r, err := NewRouter(Config{Policy: &Policy{
		ForwardGlobalBroadcast: true,
		BusyFunc:               neverBusy,
	}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddLearnedRoute(2, 100, learnedRouteTTL); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(100, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.DropReason != DropReasonNone {
		t.Fatalf("DropReason = %v, want %v", decision.DropReason, DropReasonNone)
	}
	if len(decision.Forwards) != 1 {
		t.Fatalf("len(Forwards) = %d, want 1", len(decision.Forwards))
	}
	if decision.RejectResponse != nil {
		t.Fatal("RejectResponse != nil, want nil for successful forward")
	}
}

func TestEvaluateUsesNewestLearnedRouteAndFallsBackAfterExpiry(t *testing.T) {
	clock := fakeClock{now: time.Unix(2000, 0)}
	r, err := NewRouter(Config{Clock: clock})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Older learned route on port 2.
	if err := r.AddLearnedRoute(2, 710, 40*time.Second); err != nil {
		t.Fatalf("AddLearnedRoute(2): %v", err)
	}

	// Newer learned route on port 3.
	r.(*routerImpl).clock = fakeClock{now: clock.now.Add(10 * time.Second)}
	if err := r.AddLearnedRoute(3, 710, 20*time.Second); err != nil {
		t.Fatalf("AddLearnedRoute(3): %v", err)
	}

	pdu, err := npdu.NewRoutedAPDU(710, []byte{0xAA}, 5, netprim.NetworkPriorityNormal, false, []byte{0x10})
	if err != nil {
		t.Fatalf("NewRoutedAPDU: %v", err)
	}

	decision, err := r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate first: %v", err)
	}
	if len(decision.Forwards) != 1 || decision.Forwards[0].OutPort != 3 {
		t.Fatalf("first forward out-port = %v, want 3", decision.Forwards)
	}

	// Advance past port-3 expiry but before port-2 expiry.
	r.(*routerImpl).clock = fakeClock{now: clock.now.Add(35 * time.Second)}
	decision, err = r.Evaluate(1, pdu)
	if err != nil {
		t.Fatalf("Evaluate second: %v", err)
	}
	if len(decision.Forwards) != 1 || decision.Forwards[0].OutPort != 2 {
		t.Fatalf("second forward out-port = %v, want 2", decision.Forwards)
	}
}
