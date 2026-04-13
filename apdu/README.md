# apdu starter skeleton

This package provides the BACnet application-layer scaffold used by the root
library runtime.

## implemented foundation

- Public `ASE` interface with `NewASE(cfg, codec, transport)` constructor
- Pluggable `Codec` and `Transport` interfaces to keep wire format and I/O configurable
- Confirmed invoke lifecycle (`InvokeConfirmed`) with invoke ID tracking, timeout, and close handling
- Inbound dispatch via `OnInbound` for confirmed, unconfirmed, and terminal ACK/error/reject/abort PDUs
- `UserElement` wrapper (`NewUserElement`) for B-X.request/indication/response/confirm style integration
- APDU envelope and ICI helper types (`types.go`, `ici.go`) with BACnet-style `String()` fallbacks

## state-machine scaffold

Confirmed transactions are modeled with internal clause 5.4 state-machine types
(`confirmedClientMachine`, `confirmedServerMachine`):

- Client path: `idle -> await-response -> completed|aborted`
- Server path: `idle -> await-response -> completed|aborted`
- Terminal inbound PDUs map to explicit machine events and actions
- Segmented event paths are declared and tracked, but currently return `ErrSegmentationNotSupported`

## current limits

- Focused on unsegmented confirmed/unconfirmed flows
- Segment ACK and segmented response transitions are reserved for future work
- This package is a prototype scaffold; API and behavior can evolve as coverage expands

## run tests

```sh
go test ./...
```

