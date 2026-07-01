package router

import (
	"fmt"
	"time"

	"github.com/worldiety/bacnet/common/netprim"
	"github.com/worldiety/bacnet/npdu"
)

// PortID identifies a router egress or ingress port.
type PortID uint8

// RouteKind identifies how a route entered the routing table.
type RouteKind uint8

const (
	// RouteKindConnected marks a directly connected network.
	RouteKindConnected RouteKind = iota
	// RouteKindLearned marks a dynamically learned route.
	RouteKindLearned
)

func (k RouteKind) String() string {
	switch k {
	case RouteKindConnected:
		return "connected"
	case RouteKindLearned:
		return "learned"
	default:
		return fmt.Sprintf("route-kind(%d)", k)
	}
}

// DropReason identifies why a packet was not forwarded.
type DropReason uint8

const (
	// DropReasonNone indicates that no drop occurred.
	DropReasonNone DropReason = iota
	// DropReasonUnknownDestination indicates that the destination network is unknown.
	DropReasonUnknownDestination
	// DropReasonSamePort indicates that forwarding back to the ingress port was suppressed.
	DropReasonSamePort
	// DropReasonHopCountExpired indicates that hop-count expiry suppressed forwarding.
	DropReasonHopCountExpired
	// DropReasonLoopSuppressed indicates that forwarding was suppressed because the egress
	// port serves the source network of the packet (SNET-based loop prevention).
	DropReasonLoopSuppressed
	// DropReasonRouterBusy indicates that the router has insufficient buffer space to relay
	// the message. A Reject-Message-To-Network NLM with reason RouterBusy is populated in
	// Decision.RejectResponse for unicast packets (clause 6.6.4).
	DropReasonRouterBusy
)

func (r DropReason) String() string {
	switch r {
	case DropReasonNone:
		return "none"
	case DropReasonUnknownDestination:
		return "unknown-destination"
	case DropReasonSamePort:
		return "same-port"
	case DropReasonHopCountExpired:
		return "hop-count-expired"
	case DropReasonLoopSuppressed:
		return "loop-suppressed"
	case DropReasonRouterBusy:
		return "router-busy"
	default:
		return fmt.Sprintf("drop-reason(%d)", r)
	}
}

// ForwardKind distinguishes final-hop local delivery from transit forwarding.
type ForwardKind uint8

const (
	// ForwardKindTransit indicates that the NPDU is forwarded on to another router with
	// routing headers intact (hop count decremented). Used for learned (remote) routes.
	ForwardKindTransit ForwardKind = iota
	// ForwardKindLocalDelivery indicates that the NPDU is being delivered onto a directly
	// connected network. The destination specifier is stripped from the NPDU (final-hop
	// behavior per clause 6.6.3 of ANSI/ASHRAE 135-2024).
	ForwardKindLocalDelivery
)

func (k ForwardKind) String() string {
	switch k {
	case ForwardKindTransit:
		return "transit"
	case ForwardKindLocalDelivery:
		return "local-delivery"
	default:
		return fmt.Sprintf("forward-kind(%d)", k)
	}
}

// RouteEntry is a routing-table snapshot entry.
type RouteEntry struct {
	Network   netprim.NetworkNumber
	Port      PortID
	PortInfo  []byte
	Kind      RouteKind
	ExpiresAt *time.Time
}

// Forward is one forwarding action selected by the router.
type Forward struct {
	OutPort PortID
	NPDU    *npdu.NetworkLayerProtocolDataUnit
	// Kind distinguishes transit forwarding (routing headers intact) from local delivery
	// (destination specifier stripped for the final hop to a directly connected network).
	Kind ForwardKind
}

// Decision is the result of evaluating an NPDU for local delivery and forwarding.
type Decision struct {
	DeliverLocally bool
	Forwards       []Forward
	DropReason     DropReason
	// RejectResponse is non-nil when the router has generated a Reject-Message-To-Network
	// NLM that the caller must transmit back on the ingress port (e.g. on hop-count
	// exhaustion per clause 6.6.3). The NPDU has no routing headers; the caller already
	// knows the ingress port.
	RejectResponse *npdu.NetworkLayerProtocolDataUnit
}

func (d Decision) String() string {
	transitForwards := 0
	localDeliveryForwards := 0
	for _, forward := range d.Forwards {
		if forward.Kind == ForwardKindTransit {
			transitForwards++
			continue
		}
		if forward.Kind == ForwardKindLocalDelivery {
			localDeliveryForwards++
		}
	}

	return fmt.Sprintf(
		"npdu router evaluate decision\ndeliver_locally %v\n"+
			"forwards: %v\n"+
			"transit_forwards: %v\n"+
			"local_delivery_forwards: %v\n"+
			"drop_reason: %v\n"+
			"has_reject_response: %v\n",
		d.DeliverLocally,
		len(d.Forwards),
		transitForwards,
		localDeliveryForwards,
		d.DropReason.String(),
		d.RejectResponse != nil,
	)
}

// Dropped reports whether the decision includes a drop outcome.
func (d Decision) Dropped() bool {
	return d.DropReason != DropReasonNone
}

// Policy configures forwarding behavior.
type Policy struct {
	// ForwardGlobalBroadcast controls whether global-broadcast packets are fanned out.
	ForwardGlobalBroadcast bool
	// BusyFunc, if non-nil, is called before each forwarding decision for packets that have
	// a destination specifier. When it returns true the router treats itself as having
	// insufficient buffer space to relay the message (clause 6.6.4):
	//   - Unicast packets receive DropReasonRouterBusy and a Reject-Message-To-Network NLM
	//     with reason RouterBusy in Decision.RejectResponse.
	//   - Global-broadcast packets receive DropReasonRouterBusy but no RejectResponse
	//     (no single addressable originator).
	// A nil Busy function means the router is never busy.
	BusyFunc func() bool
}

func (p Policy) Busy() bool {
	if p.BusyFunc == nil {
		return false
	}

	return p.BusyFunc()
}

// Config configures a routerImpl.
type Config struct {
	// Policy defines the forwarding behavior of the routerImpl
	Policy *Policy
	// Clock allows the routerImpl to get the current time. If nil defaults to use time.Now()
	Clock Clock
}

// Clock provides the current time for learned-route expiry evaluation.
type Clock interface {
	Now() time.Time
}
