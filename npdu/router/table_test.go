package router

import (
	"errors"
	"testing"
	"time"

	"go.wdy.de/bacnet"
)

type fakeClock struct {
	now time.Time
}

func (f fakeClock) Now() time.Time {
	return f.now
}

func TestAddConnectedRouteAndSnapshot(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.AddConnectedRoute(2, 200, []byte{0xBB}); err != nil {
		t.Fatalf("AddConnectedRoute(200): %v", err)
	}
	if err := r.AddConnectedRoute(1, 100, []byte{0xAA}); err != nil {
		t.Fatalf("AddConnectedRoute(100): %v", err)
	}

	snapshot := r.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("len(snapshot) = %d, want 2", len(snapshot))
	}
	if snapshot[0].Network != 100 || snapshot[1].Network != 200 {
		t.Fatalf("snapshot networks = [%d %d], want [100 200]", snapshot[0].Network, snapshot[1].Network)
	}
	if snapshot[0].Kind != RouteKindConnected {
		t.Fatalf("snapshot[0].Kind = %v, want %v", snapshot[0].Kind, RouteKindConnected)
	}

	snapshot[0].PortInfo[0] = 0xFF
	lookup, ok := r.Lookup(100)
	if !ok {
		t.Fatal("Lookup(100) = missing, want present")
	}
	if lookup.PortInfo[0] != 0xAA {
		t.Fatalf("lookup.PortInfo[0] = 0x%02X, want 0xAA", lookup.PortInfo[0])
	}
}

func TestAddLearnedRouteExpiry(t *testing.T) {
	clock := fakeClock{now: time.Unix(100, 0)}
	r, err := NewRouter(Config{Clock: clock})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.AddLearnedRoute(3, 300, time.Minute); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}
	if _, ok := r.Lookup(300); !ok {
		t.Fatal("Lookup(300) before expiry = missing, want present")
	}

	r.(*routerImpl).clock = fakeClock{now: clock.now.Add(time.Minute)}
	if _, ok := r.Lookup(300); ok {
		t.Fatal("Lookup(300) after expiry = present, want missing")
	}
}

func TestRoutingTableEntries(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := r.AddConnectedRoute(1, 100, []byte{0xAA, 0xBB}); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}
	if err := r.AddLearnedRoute(2, 200, time.Minute); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}

	entries, err := r.RoutingTableEntries()
	if err != nil {
		t.Fatalf("RoutingTableEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].ConnectedDNET() != 100 || entries[0].PortID() != 1 {
		t.Fatalf("entries[0] = %+v, want network 100 port 1", entries[0])
	}
	if got := entries[0].PortInfo(); len(got) != 2 || got[0] != 0xAA || got[1] != 0xBB {
		t.Fatalf("entries[0].PortInfo = %#v, want %#v", got, []byte{0xAA, 0xBB})
	}
	if entries[1].ConnectedDNET() != 200 || entries[1].PortID() != 2 {
		t.Fatalf("entries[1] = %+v, want network 200 port 2", entries[1])
	}
}

func TestAddRouteValidation(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "connected local network", err: r.AddConnectedRoute(1, bacnet.LocalNetwork, nil), wantErr: ErrRouteToLocalNetwork},
		{name: "connected global broadcast", err: r.AddConnectedRoute(1, bacnet.GlobalBroadcastNetwork, nil), wantErr: ErrInvalidRoute},
		{name: "learned invalid ttl", err: r.AddLearnedRoute(1, 100, 0), wantErr: ErrInvalidRoute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", tt.err, tt.wantErr)
			}
		})
	}
}

func TestLookupAllReturnsMultipleRoutes(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.AddLearnedRoute(2, 400, time.Hour); err != nil {
		t.Fatalf("AddLearnedRoute(2): %v", err)
	}
	if err := r.AddLearnedRoute(3, 400, time.Hour); err != nil {
		t.Fatalf("AddLearnedRoute(3): %v", err)
	}

	routes := r.LookupAll(400)
	if len(routes) != 2 {
		t.Fatalf("len(LookupAll(400)) = %d, want 2", len(routes))
	}
	if routes[0].Port != 3 || routes[1].Port != 2 {
		t.Fatalf("ports = [%d %d], want [3 2]", routes[0].Port, routes[1].Port)
	}
}

func TestLookupPrefersConnectedOverLearned(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.AddLearnedRoute(2, 410, time.Hour); err != nil {
		t.Fatalf("AddLearnedRoute: %v", err)
	}
	if err := r.AddConnectedRoute(3, 410, nil); err != nil {
		t.Fatalf("AddConnectedRoute: %v", err)
	}

	route, ok := r.Lookup(410)
	if !ok {
		t.Fatal("Lookup(410) = missing, want present")
	}
	if route.Kind != RouteKindConnected || route.Port != 3 {
		t.Fatalf("Lookup(410) = %+v, want connected on port 3", route)
	}
}

func TestAddLearnedRouteDuplicateRefreshesTimeout(t *testing.T) {
	clock := fakeClock{now: time.Unix(1000, 0)}
	r, err := NewRouter(Config{Clock: clock})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.AddLearnedRoute(2, 500, time.Minute); err != nil {
		t.Fatalf("AddLearnedRoute first: %v", err)
	}

	// Re-advertise the same learned route before expiry; timeout should extend.
	r.(*routerImpl).clock = fakeClock{now: clock.now.Add(30 * time.Second)}
	if err := r.AddLearnedRoute(2, 500, time.Minute); err != nil {
		t.Fatalf("AddLearnedRoute duplicate: %v", err)
	}

	// At +70s from initial insertion the first timeout would have expired, but
	// the refreshed timeout should still keep the route alive.
	r.(*routerImpl).clock = fakeClock{now: clock.now.Add(70 * time.Second)}
	if _, ok := r.Lookup(500); !ok {
		t.Fatal("Lookup(500) after refresh = missing, want present")
	}

	// Past the refreshed timeout (+95s from refresh = +125s total), it should expire.
	r.(*routerImpl).clock = fakeClock{now: clock.now.Add(125 * time.Second)}
	if _, ok := r.Lookup(500); ok {
		t.Fatal("Lookup(500) after refreshed expiry = present, want missing")
	}
}

func TestRemoveRouteOnPort(t *testing.T) {
	r, err := NewRouter(Config{})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	if err := r.AddLearnedRoute(2, 600, time.Hour); err != nil {
		t.Fatalf("AddLearnedRoute(2): %v", err)
	}
	if err := r.AddLearnedRoute(3, 600, time.Hour); err != nil {
		t.Fatalf("AddLearnedRoute(3): %v", err)
	}

	r.RemoveRouteOnPort(600, 2)
	routes := r.LookupAll(600)
	if len(routes) != 1 {
		t.Fatalf("len(LookupAll(600)) = %d, want 1", len(routes))
	}
	if routes[0].Port != 3 {
		t.Fatalf("remaining port = %d, want 3", routes[0].Port)
	}
}
