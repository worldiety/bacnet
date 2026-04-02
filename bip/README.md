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

### ipv6 notes

- Use `NewFrameForAddress(addr, function, payload)` to select `BVLCTypeBACnetIP` (`0x81`) or `BVLCTypeBACnetIP6` (`0x82`) from the destination address family.
- Use `NewFrameWithType(frameType, function, payload)` when the BVLC type must be explicit.
- `NewDatagramConn(addr)` binds with `udp4` for IPv4 and `udp6` for IPv6 addresses.

```go
addr4 := netip.MustParseAddr("192.168.1.20")
addr6 := netip.MustParseAddr("2001:db8::20")

f4, _ := bip.NewFrameForAddress(addr4, bip.FunctionOriginalUnicastNPDU, []byte{0x01})
f6, _ := bip.NewFrameForAddress(addr6, bip.FunctionOriginalUnicastNPDU, []byte{0x01})

_ = f4 // f4.Type == bip.BVLCTypeBACnetIP
_ = f6 // f6.Type == bip.BVLCTypeBACnetIP6
```
