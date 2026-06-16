# bacnet

`bacnet` is a lightweight, pure-Go library for building BACnet/IP applications.
It provides a complete BACnet/IP stack from BVLC framing through the application
layer, including a typed client API for common BACnet services.

## Goals

- Pure Go implementation — no cgo
- Minimal dependencies (standard library only)
- BACnet/IP focused (ANSI/ASHRAE 135-2024)
- Relying on the OS for physical layer and transport (UDP)
- Easy to test and extend via interface-first design

## Package layout

| Package | Purpose |
|---|---|
| `.` (`bacnet`) | Constants, core types, errors, addressing primitives |
| `bip/` | BACnet/IP BVLC frame encode/decode, UDP transport, BBMD, foreign-device registration, `ClientRuntime` end-to-end wiring |
| `apdu/` | Application layer: ICI-first ASE dispatch, clause 5.4 state machines, typed `Client` with confirmed/unconfirmed services, discovery |
| `npdu/` | NPDU encode/decode per clause 6.2.2, all standard network-layer-message types |
| `npdu/router/` | Routing table and forwarding policy (connected + TTL-learned routes) |
| `encoding/` | BACnet tag/value encoding primitives (tag parser, unsigned, object-id, character-string) |
| `internal/util/` | Non-public shared helpers (e.g. `CopyPointersValue[T]`) |
| `testdata/npdu/` | Wire conformance vectors for NLM encode/decode |
| `examples/` | Deferred until the API is stable |

## Project structure

```text
.
├── address.go
├── constants.go
├── doc.go
├── errors.go
├── logging.go
├── types.go
├── apdu/        (active: application layer, typed client, state machines)
├── bip/         (active: BVLC framing, UDP transport, BBMD, ClientRuntime)
├── encoding/    (active: BACnet tag/value encoding primitives)
├── npdu/        (active: NPDU encode/decode, all NLM types)
│   └── router/  (active: routing table and forwarding policy)
├── internal/
│   └── util/    (active: shared defensive-copy helpers)
├── testdata/
│   └── npdu/    (active: NLM wire conformance vectors)
└── examples/    (deferred until API is stable)
```

## Features

### BACnet/IP (bip/)

- All 12 Annex J BVLC function types — full encode/decode
- UDP datagram transport with configurable max datagram size
- BBMD: broadcast distribution table (BDT) and foreign device table (FDT) management with TTL expiry
- Foreign-device registration (local broadcast + unicast + foreign-device registration)
- `ClientRuntime` — wires transport, stack, ASE, and client into a single runnable object

### Application layer (apdu/)

- ICI-first ASE dispatch for all 8 APDU PDU types
- Confirmed request state machine: invoke-ID allocation, retry on timeout, ACK/Error/Reject/Abort handling
- Server-side: segmented confirmed-request receive and segmented ComplexACK send (transmit window, Segment-ACK)
- Duplicate confirmed-request suppression per §5.4.4
- Typed `Client` interface covering:
  - `WhoIs` / `WhoHas`
  - `ReadProperty` / `ReadPropertyMultiple`
  - `WriteProperty` / `WritePropertyMultiple`
  - `ReadRange` (by position, sequence number, or time)
  - `DeviceCommunicationControl` / `ReinitializeDevice`
  - `SubscribeCOV` / `SubscribeCOVProperty`
  - `Discover` — sends Who-Is and collects deduplicated I-Am responses within a time window
  - COV notification handlers (unconfirmed and multiple)

### Network layer (npdu/)

- Full NPDU encode/decode per clause 6.2.2
- Constructors for local, routed, sourced, and network-layer NPDUs
- All 13 standard network-layer-message types (0x00–0x09, 0x12–0x13) plus proprietary range

### Routing (npdu/router/)

- Routing table with connected and TTL-learned routes
- Forwarding decisions: local delivery, global broadcast fan-out, unicast forwarding
- Hop-count decrement and expiry, SNET-based loop suppression
- Router-busy policy, Reject-Message-To-Network response generation

### Encoding (encoding/)

- BACnet tag parser (short/extended tag numbers, all length forms)
- Application and context primitive encode/decode
- Unsigned, object-identifier, and ASCII character-string codecs

## Known limitations

- **Client-side segmented send**: not implemented — `ErrSegmentationNotSupported` is returned if the request exceeds the negotiated APDU size.
- **Client-side segmented ComplexACK receive**: not implemented — a segmented ComplexACK received by the client triggers an Abort and returns `ErrSegmentationNotSupported`.
- **Encoding coverage**: `encoding/` covers the types used by the current service surface. Raw/opaque `[]byte` is used for property values not yet decoded (e.g. Real, Double, Boolean, Date, Time, BitString).

## Example

Discover all BACnet devices on the local network:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"time"

	"go.wdy.de/bacnet/apdu"
	"go.wdy.de/bacnet/bip"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := bip.NewClientRuntime(netip.MustParseAddr("0.0.0.0"), bip.ClientRuntimeConfig{
		ASE: apdu.ASEConfig{
			InvokeTimeout:        2 * time.Second,
			APDURetries:          1,
			MaxConcurrentInvokes: 8,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer runtime.Close()

	go func() {
		if err := runtime.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("runtime stopped: %v", err)
		}
	}()

	broadcast, err := bip.AddrPortToAddress(netip.MustParseAddrPort("255.255.255.255:47808"))
	if err != nil {
		log.Fatal(err)
	}

	devices, err := runtime.Client().Discover(ctx, apdu.DiscoverRequest{
		Destination: broadcast,
		WhoIs:       apdu.WhoIsRequest{},
		Window:      3 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	for _, d := range devices {
		fmt.Printf("device=%s source=%v max-apdu=%d segmentation=%s vendor=%d\n",
			d.DeviceIdentifier,
			d.Source,
			d.MaxAPDULengthAccepted,
			d.SegmentationSupported,
			d.VendorID,
		)
	}
}
```

## Run tests

```sh
go test ./...
go test -coverprofile=coverage.out ./...
```

## Status

The project is in a prototype phase. The API is usable and exercised by tests,
but may change as BACnet coverage expands. The module path is:

```
module go.wdy.de/bacnet
```
