# apdu starter skeleton

This package provides the BACnet application-layer scaffold used by the root
library runtime.

## implemented foundation

- Public `ASE` interface with `NewASE(cfg, transport)` constructor
- ICI-first API (`ConfirmedRequestICI`, `UnconfirmedRequestICI`, `ConfirmICI`) for B-X primitives
- Typed client wrapper (`Client`, `NewClient`) for discovery (`WhoIs`, `WhoHas`, typed `IAm`/`IHave` handlers, `Discover` orchestration helper), object access (`ReadProperty`, `ReadPropertyMultiple`, `WriteProperty`, `WritePropertyMultiple`, `ReadRange`), COV (`SubscribeCOV`, `SubscribeCOVProperty`, typed unconfirmed COV notification handlers), and device management (`DeviceCommunicationControl`, `ReinitializeDevice`)
- Internal APDU codec owned by `apdu` (not exposed as public extension surface)
- NPDU boundary integration (`apdu -> npdu`) with typed inbound NPDU handling
- Confirmed invoke lifecycle (`InvokeConfirmed`) with invoke ID tracking, timeout, and close handling
- Inbound dispatch via `OnInboundNPDU` for confirmed, unconfirmed, and terminal ACK/error/reject/abort PDUs
- `UserElement` wrapper (`NewUserElement`) for B-X.request/indication/response/confirm style integration
- APDU envelope and ICI helper types (`types.go`, `ici.go`) with BACnet-style `String()` fallbacks

## byte-slice ownership

- Clone at package boundaries (ingress from public APIs/transport and egress to external callers)
- Pass payload slices through internal ASE/state-machine paths without extra clones
- Keep per-transaction flow single-threaded; if that changes, reintroduce clone/synchronization at the affected boundary

## state-machine scaffold

Confirmed transactions are modeled with internal clause 5.4 state-machine types
(`confirmedClientMachine`, `confirmedServerMachine`):

- Client path: `idle -> await-response -> completed|aborted`
- Server path: `idle -> segmented-request-receiving -> await-response -> await-segment-ack -> completed|aborted`
- Terminal inbound PDUs map to explicit machine events and actions
- Segmented confirmed-request receive is implemented with window negotiation, Segment-ACK emission, and immediate NAK on out-of-order segments
- Segmented confirmed-server ComplexACK responses are implemented with Segment-ACK driven progression and retry/abort handling

## current limits

- Focused on segmented confirmed-request receive and segmented confirmed-server ComplexACK responses on the server side
- Segmented client send/receive transitions remain reserved for future work
- This package is a prototype scaffold; API and behavior can evolve as coverage expands

## v1 client segmentation limits

- Client confirmed requests are currently non-segmented only
- Inbound segmented ComplexACK on the client path is rejected and completed with `ErrSegmentationNotSupported`
- Inbound Segment-ACK on the client path is treated as unsupported in v1
- Client-side segmented send/receive state-machine transitions are intentionally deferred

## run tests

```sh
go test ./...
```

