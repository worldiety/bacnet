# bip starter scaffold

This package provides a BACnet/IP Annex J BVLC scaffold:

- BVLC type/function constants with `String()` and `Valid()` helpers
- `Frame` constructor, encode, and decode with strict header validation
- BVLC type selection for both BACnet/IP (`0x81`) and BACnet/IP6 (`0x82`)
- Defensive copy behavior for slice-backed payloads
- Minimal UDP datagram transport abstraction (`DatagramConn`, `Transport`)
- Table-driven tests for codec behavior and transport flow

## current scope

Implemented as a lightweight core for:

- `Original-Unicast-NPDU`
- `Original-Broadcast-NPDU`
- Shared BVLC framing for other Annex J functions

The package does not yet implement BBMD/FDT management logic.

## run tests

```sh
go test ./...
```
