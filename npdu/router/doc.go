// Package router provides BACnet network-layer router state and forwarding logic.
//
// It builds on package npdu's data and wire models but keeps routing-table
// management, forwarding decisions, and hop-count policy separate from NPDU
// encode/decode concerns.
package router
