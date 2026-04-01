# apdu starter skeleton

This package provides a concrete starter skeleton for a BACnet Application
Service Element (ASE):

- APDU envelope types and service choice constants
- ASE orchestration with confirmed transaction tracking
- Confirmed and unconfirmed handler registration/dispatch
- Pluggable `Codec` and `Transport` interfaces for integration
- Unit tests with a tiny in-memory codec/transport harness

## status

This is intentionally a starter model, not a full BACnet APDU wire
implementation yet. It focuses on extension points and behavior scaffolding.

## run tests

```sh
go test ./...
```

