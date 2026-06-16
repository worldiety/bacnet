package router

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	errors2 "go.wdy.de/bacnet/common/errors"
	"go.wdy.de/bacnet/common/log"
	"go.wdy.de/bacnet/common/netprim"
	"go.wdy.de/bacnet/internal/util"
	"go.wdy.de/bacnet/npdu"
)

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

type routeRecord struct {
	network   netprim.NetworkNumber
	port      PortID
	portInfo  []byte
	kind      RouteKind
	lastSeen  time.Time
	expiresAt *time.Time
}

type Router interface {
	Evaluate(inPort PortID, pdu *npdu.NetworkLayerProtocolDataUnit) (Decision, error)
	AddConnectedRoute(port PortID, network netprim.NetworkNumber, portInfo []byte) error
	AddLearnedRoute(port PortID, network netprim.NetworkNumber, ttl time.Duration) error
	RemoveRoute(network netprim.NetworkNumber)
	RemoveRouteOnPort(network netprim.NetworkNumber, port PortID)
	Lookup(network netprim.NetworkNumber) (*RouteEntry, bool)
	LookupAll(network netprim.NetworkNumber) []RouteEntry
	Snapshot() []RouteEntry
	RoutingTableEntries() ([]npdu.RoutingTablePortEntry, error)
}

// routerImpl holds routing-table state and forwarding policy.
type routerImpl struct {
	policy Policy
	clock  Clock
	routes map[netprim.NetworkNumber]map[PortID]routeRecord
}

// NewRouter constructs a router with the provided configuration.
func NewRouter(cfg Config) (Router, error) {
	policy := Policy{
		ForwardGlobalBroadcast: true,
	}

	if cfg.Policy != nil {
		policy = *cfg.Policy
	}

	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}

	return &routerImpl{
		policy: policy,
		clock:  clock,
		routes: make(map[netprim.NetworkNumber]map[PortID]routeRecord),
	}, nil
}

// cloneRouteRecord deeply clones a routeRecord
func cloneRouteRecord(rec routeRecord) routeRecord {
	return routeRecord{
		network:   rec.network,
		port:      rec.port,
		portInfo:  slices.Clone(rec.portInfo),
		kind:      rec.kind,
		lastSeen:  rec.lastSeen,
		expiresAt: util.CopyPointersValue(rec.expiresAt),
	}
}

// bestRouteLocked picks a deterministic best route for a destination network.
// Preference order: connected > learned; then freshest learned route; tie-break by lowest port.
func (r *routerImpl) bestRouteLocked(network netprim.NetworkNumber) (routeRecord, bool) {
	r.pruneExpiredLocked()
	ports, ok := r.routes[network]
	if !ok || len(ports) == 0 {
		return routeRecord{}, false
	}

	var best routeRecord
	hasBest := false
	for _, candidate := range ports {
		if !hasBest {
			best = candidate
			hasBest = true
			continue
		}

		if candidate.kind == RouteKindConnected && best.kind != RouteKindConnected {
			best = candidate
			continue
		}
		if candidate.kind != RouteKindConnected && best.kind == RouteKindConnected {
			continue
		}

		if candidate.kind == RouteKindLearned && best.kind == RouteKindLearned {
			if candidate.lastSeen.After(best.lastSeen) {
				best = candidate
				continue
			}
			if candidate.lastSeen.Equal(best.lastSeen) && candidate.port < best.port {
				best = candidate
				continue
			}
		}

		if candidate.kind == RouteKindConnected && best.kind == RouteKindConnected && candidate.port < best.port {
			best = candidate
		}
	}

	if !hasBest {
		return routeRecord{}, false
	}
	return cloneRouteRecord(best), true
}

func validateRoute(network netprim.NetworkNumber, _ PortID, portInfo []byte) error {
	if network.IsLocal() {
		return errors2.NewValidationError("network", network, ErrRouteToLocalNetwork)
	}

	if network.IsGlobalBroadcast() {
		return errors2.NewValidationError("network", network, ErrInvalidRoute)
	}

	if len(portInfo) > 255 {
		return errors2.NewValidationError("port info", len(portInfo), ErrInvalidRoute)
	}

	return nil
}

func (r *routerImpl) pruneExpiredLocked() {
	now := r.clock.Now()
	for network, ports := range r.routes {
		for port, route := range ports {
			if route.kind != RouteKindLearned || route.expiresAt == nil {
				continue
			}

			if !route.expiresAt.After(now) {
				delete(ports, port)
			}
		}
		if len(ports) == 0 {
			delete(r.routes, network)
		}
	}
}

// portForNetwork returns the port that directly serves network, if any route is known.
// It does not prune expired routes; callers should invoke pruneExpiredLocked first when
// freshness matters.
func (r *routerImpl) portForNetwork(network netprim.NetworkNumber) (PortID, bool) {
	route, ok := r.bestRouteLocked(network)
	if !ok {
		return 0, false
	}
	return route.port, true
}

// sourcePortsForSNETLocked returns all active ports that currently serve the packet's
// source network (SNET). The returned set is used for loop suppression.
func (r *routerImpl) sourcePortsForSNETLocked(pdu *npdu.NetworkLayerProtocolDataUnit) map[PortID]struct{} {
	if pdu == nil || !pdu.HasSourceSpecifier() {
		return nil
	}

	snet := pdu.SNET()
	if snet == nil {
		return nil
	}

	byPort, ok := r.routes[snet.ToBacnetNetworkNumber()]
	if !ok || len(byPort) == 0 {
		return nil
	}

	ports := make(map[PortID]struct{}, len(byPort))
	for port := range byPort {
		ports[port] = struct{}{}
	}

	return ports
}

// buildRejectResponse constructs a local (non-routed) Reject-Message-To-Network NPDU
// that should be transmitted back on the ingress port. Returns nil if construction
// fails (treated as a best-effort helper; the drop is already recorded in Decision).
func buildRejectResponse(dnet netprim.NetworkNumber, reason npdu.NlmRejectReason) *npdu.NetworkLayerProtocolDataUnit {
	msg, err := npdu.NewRejectMessageToNetworkMessage(dnet, reason)
	if err != nil {
		log.Logger.Warn(
			"npdu router reject response build skipped",
			"error", err,
			"dnet", dnet,
			"reason", uint8(reason),
		)

		return nil
	}
	reject, err := npdu.NewNetworkLayerNPDUFromMessage(npdu.NPCI{Priority: netprim.NetworkPriorityNormal}, msg)
	if err != nil {
		log.Logger.Warn(
			"npdu router reject response npdu build skipped",
			"error", err,
			"dnet", dnet,
			"reason", uint8(reason),
		)

		return nil
	}
	return reject
}

// Evaluate decides whether an NPDU should be delivered locally, forwarded, or dropped.
//
// Local vs. routed delivery split (clause 6.6.3):
//   - No destination specifier → DeliverLocally only.
//   - DNET is global broadcast (0xFFFF) → DeliverLocally + fan-out forwards.
//   - DNET matches a directly connected (RouteKindConnected) network → DeliverLocally is set
//     and a ForwardKindLocalDelivery forward is produced with the destination specifier stripped.
//   - DNET matches a learned (transit) route → transit Forward with hop count decremented.
//
// Hop-count exhaustion (clause 6.6.3): when a transit forward is suppressed due to hop-count
// expiry, Decision.RejectResponse is populated with a Reject-Message-To-Network NLM that the
// caller must send back on the ingress port.
//
// Router busy (clause 6.6.4): when Policy.Busy is non-nil and returns true, forwarding is
// suppressed before any route-build work. Unicast packets receive DropReasonRouterBusy and a
// Reject-Message-To-Network NLM with reason RouterBusy in Decision.RejectResponse. Global-
// broadcast packets receive DropReasonRouterBusy but no RejectResponse.
//
// Unknown destination: when no route is known for the unicast DNET, DropReasonUnknownDestination
// is set and Decision.RejectResponse is populated with a Reject-Message-To-Network NLM.
//
// Loop prevention: forwarding is suppressed (DropReasonLoopSuppressed) when the candidate
// egress port is the same port that serves the source network (SNET) of the packet.
//
// Network-layer messages are always delivered locally in addition to any forwarding action.
func (r *routerImpl) Evaluate(inPort PortID, pdu *npdu.NetworkLayerProtocolDataUnit) (Decision, error) {
	if pdu == nil || !pdu.Valid() {
		return Decision{}, errors2.NewValidationError("npdu", pdu, ErrInvalidNPDU)
	}

	log.Logger.Debug(
		"npdu router evaluate inbound",
		"in_port", inPort,
		"has_destination", pdu.HasDestinationSpecifier(),
		"has_source", pdu.HasSourceSpecifier(),
		"is_network_layer_message", pdu.IsNetworkLayerMessage(),
	)

	decision := Decision{}
	if pdu.IsNetworkLayerMessage() {
		decision.DeliverLocally = true
	}

	if !pdu.HasDestinationSpecifier() {
		decision.DeliverLocally = true

		log.Logger.Debug(decision.String())

		return decision, nil
	}

	// Capture SNET-serving ports before route learning updates the table with inPort.
	// This preserves loop suppression against the pre-existing source-network path.
	r.pruneExpiredLocked()
	sourcePorts := r.sourcePortsForSNETLocked(pdu)

	// Update table entry for this pdu and ingress port.
	var src netprim.NetworkNumber
	if snet := pdu.SNET(); snet != nil {
		src = snet.ToBacnetNetworkNumber()
	} else {
		src = netprim.LocalNetwork
	}

	err := r.AddLearnedRoute(inPort, src, time.Minute)
	if err != nil && !errors.Is(err, ErrRouteToLocalNetwork) { // ignore error if it just indicates a local route
		return Decision{}, err
	}

	if pdu.DNET() == nil {
		return Decision{}, errors2.NewValidationError("dnet", pdu, ErrInvalidNPDU)
	}

	dnet := netprim.NetworkNumber(*pdu.DNET())

	if dnet.IsGlobalBroadcast() {
		decision.DeliverLocally = true
		if !r.policy.ForwardGlobalBroadcast {
			log.Logger.Debug(decision.String())
			return decision, nil
		}

		// Check busy before committing to fan-out work. No RejectResponse is produced
		// for broadcasts because there is no single addressable originator.
		if r.policy.Busy() {
			decision.DropReason = DropReasonRouterBusy
			log.Logger.Debug(decision.String())
			return decision, nil
		}

		forwards, dropReason, err := r.evaluateGlobalBroadcast(inPort, sourcePorts, pdu)
		if err != nil {
			return Decision{}, err
		}
		decision.Forwards = forwards
		decision.DropReason = dropReason
		log.Logger.Debug(decision.String())
		return decision, nil
	}

	route, ok := r.bestRouteLocked(dnet)
	if !ok {
		decision.DropReason = DropReasonUnknownDestination
		decision.RejectResponse = buildRejectResponse(
			dnet,
			npdu.NLMRejectReasonOther,
		)
		log.Logger.Debug(decision.String())
		return decision, nil
	}
	if route.port == inPort {
		decision.DropReason = DropReasonSamePort
		log.Logger.Debug(decision.String())
		return decision, nil
	}

	// SNET-based loop suppression: do not forward toward any port known to serve the
	// packet source network.
	if _, blocked := sourcePorts[route.port]; blocked {
		decision.DropReason = DropReasonLoopSuppressed
		log.Logger.Debug(decision.String())
		return decision, nil
	}

	// Check busy before committing to any forwarding work. Only unicast packets reach
	// this point, so a RejectResponse can be addressed to the specific DNET.
	if r.policy.Busy() {
		decision.DropReason = DropReasonRouterBusy
		decision.RejectResponse = buildRejectResponse(dnet, npdu.NLMRejectReasonRouterBusy)
		log.Logger.Debug(decision.String())
		return decision, nil
	}

	// Connected route: this router is the final hop — deliver locally on the connected port
	// with the destination specifier stripped (clause 6.6.3).
	if route.kind == RouteKindConnected {
		decision.DeliverLocally = true
		forward, err := r.buildLocalDeliveryForward(route.port, pdu)
		if err != nil {
			return Decision{}, err
		}
		decision.Forwards = []Forward{forward}
		log.Logger.Debug(decision.String())
		return decision, nil
	}

	// Learned (transit) route: forward with routing headers, decrement hop count.
	forward, dropReason, err := r.buildForward(route.port, pdu)
	if err != nil {
		return Decision{}, err
	}
	if dropReason != DropReasonNone {
		decision.DropReason = dropReason
		if dropReason == DropReasonHopCountExpired {
			decision.RejectResponse = buildRejectResponse(dnet, npdu.NLMRejectReasonTooManyHops)
		}
		log.Logger.Debug(decision.String())
		return decision, nil
	}
	decision.Forwards = []Forward{forward}
	log.Logger.Debug(decision.String())
	return decision, nil
}

func (r *routerImpl) evaluateGlobalBroadcast(inPort PortID, sourcePorts map[PortID]struct{}, pdu *npdu.NetworkLayerProtocolDataUnit) ([]Forward, DropReason, error) {
	// pruneExpiredLocked was already called by Evaluate before this point.

	// Collect the set of egress ports and record whether each port has at least one
	// directly-connected route. Connected ports receive a local-delivery forward
	// (destination specifier stripped); ports that only serve learned routes receive
	// a transit forward (destination specifier and decremented hop count preserved).
	type portMeta struct {
		hasConnected bool
		hasLearned   bool
	}
	ports := make(map[PortID]portMeta)
	for _, byPort := range r.routes {
		for _, route := range byPort {
			if route.port == inPort {
				continue
			}
			// SNET-based loop prevention: skip ports that serve the source network.
			if _, blocked := sourcePorts[route.port]; blocked {
				continue
			}
			pm := ports[route.port]
			if route.kind == RouteKindConnected {
				pm.hasConnected = true
			} else {
				pm.hasLearned = true
			}
			ports[route.port] = pm
		}
	}
	if len(ports) == 0 {
		return nil, DropReasonNone, nil
	}

	orderedPorts := make([]int, 0, len(ports))
	for port := range ports {
		orderedPorts = append(orderedPorts, int(port))
	}
	sort.Ints(orderedPorts)

	forwards := make([]Forward, 0, len(orderedPorts))
	for _, p := range orderedPorts {
		port := PortID(p)
		pm := ports[port]

		if pm.hasConnected {
			// Final-hop delivery: strip the destination specifier.
			forward, err := r.buildLocalDeliveryForward(port, pdu)
			if err != nil {
				return nil, DropReasonNone, err
			}
			forwards = append(forwards, forward)
			continue
		}
		if !pm.hasLearned {
			continue
		}

		// Transit forward: decrement hop count.
		forward, dropReason, err := r.buildForward(port, pdu)
		if err != nil {
			return nil, DropReasonNone, err
		}
		if dropReason == DropReasonHopCountExpired {
			// Silently skip this port on broadcast hop-count expiry; no reject for
			// broadcasts since there is no single addressable originator.
			log.Logger.Warn(
				"npdu router global broadcast skip egress port",
				"reason", dropReason.String(),
				"out_port", port,
			)
			continue
		}
		if dropReason != DropReasonNone {
			return nil, dropReason, nil
		}
		forwards = append(forwards, forward)
	}
	return forwards, DropReasonNone, nil
}

func (r *routerImpl) buildForward(outPort PortID, pdu *npdu.NetworkLayerProtocolDataUnit) (Forward, DropReason, error) {
	npci := pdu.NPCI()
	if npci.Destination == nil {
		return Forward{}, DropReasonNone, fmt.Errorf("router buildForward called without destination specifier")
	}

	if npci.Destination.HopCount <= 1 {
		return Forward{}, DropReasonHopCountExpired, nil
	}
	npci.Destination.HopCount -= 1

	var forwarded *npdu.NetworkLayerProtocolDataUnit
	var err error
	if pdu.IsNetworkLayerMessage() {
		forwarded, err = npdu.NewNetworkLayerNPDU(npci, pdu.MustNetworkLayerMessageHeader(), pdu.NetworkLayerPayloadBytes())
	} else {
		forwarded, err = npdu.NewApplicationNPDU(npci, pdu.ApplicationPayloadBytes())
	}
	if err != nil {
		return Forward{}, DropReasonNone, fmt.Errorf("rebuild forwarded npdu: %w", err)
	}

	return Forward{OutPort: outPort, NPDU: forwarded, Kind: ForwardKindTransit}, DropReasonNone, nil
}

// buildLocalDeliveryForward constructs a Forward for final-hop delivery to a directly
// connected network. It strips the destination specifier from the NPDU (per clause 6.6.3),
// preserving the source specifier, priority, and ExpectingReply flag. The resulting NPDU
// carries no routing wrapper; the connected port transmits it as a local message.
func (r *routerImpl) buildLocalDeliveryForward(outPort PortID, pdu *npdu.NetworkLayerProtocolDataUnit) (Forward, error) {
	npci := pdu.NPCI()
	// Strip destination for final-hop local delivery.
	npci.Destination = nil

	var rebuilt *npdu.NetworkLayerProtocolDataUnit
	var err error
	if pdu.IsNetworkLayerMessage() {
		rebuilt, err = npdu.NewNetworkLayerNPDU(npci, pdu.MustNetworkLayerMessageHeader(), pdu.NetworkLayerPayloadBytes())
	} else {
		rebuilt, err = npdu.NewApplicationNPDU(npci, pdu.ApplicationPayloadBytes())
	}
	if err != nil {
		return Forward{}, fmt.Errorf("rebuild local-delivery npdu: %w", err)
	}

	return Forward{OutPort: outPort, NPDU: rebuilt, Kind: ForwardKindLocalDelivery}, nil
}
