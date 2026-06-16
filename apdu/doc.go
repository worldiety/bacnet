// Package apdu provides a BACnet application-layer scaffold.
//
// The package owns APDU wire encode/decode and composes APDUs into NPDUs via
// the npdu package. This keeps NPDU modeling APDU-agnostic while ASE exposes an
// ICI-first API with typed request/indication/response/confirm boundaries.
package apdu
