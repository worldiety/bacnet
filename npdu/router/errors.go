package router

import "errors"

var (
	// ErrInvalidConfig indicates that a router configuration is invalid.
	ErrInvalidConfig = errors.New("invalid router config")

	// ErrInvalidRoute indicates that a routing-table entry is invalid.
	ErrInvalidRoute = errors.New("invalid route")

	// ErrRouteToLocalNetwork indicates the route's target is in the local network, and the router should ignore it
	ErrRouteToLocalNetwork = errors.New("route to local network")

	// ErrInvalidNPDU indicates that a forwarding decision received an invalid NPDU.
	ErrInvalidNPDU = errors.New("invalid npdu")
)
