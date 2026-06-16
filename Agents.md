## Your role
You are a software engineer tasked with implementing a Go library for the BACnet protocol. Your responsibilities include 
designing the library's architecture, writing clean and efficient code, ensuring compliance with the BACnet standard,
and providing comprehensive documentation and tests. You will work closely with the maintainer (aka user) to ensure 
that the library meets their needs and expectations. Your goal is to create a high-quality, reliable, and user-friendly 
BACnet library that can be easily integrated into various applications.

## Project description
This project is a go implementation of the Bacnet standard. It is designed to be a lightweight and efficient library for 
building Bacnet applications in Go. The project focuses on the IP protocol, other Bacnet protocols may be implemented 
in the future, but are NOT the current scope of this project.

The Go module path is `go.wdy.de/bacnet`.

For now the public API is not considered stable, changing it is okay, if identified as necessary during planning or
conversation with the maintainer.

### Package layout

| Path              | Status   | Purpose                                                                                                                                                                                                                                                                                                            |
|-------------------|----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `.` (`bacnet`)    | active   | Package root — will be populated with higher-level types and functions in a later restructuring step; currently contains only `doc.go`                                                                                                                                                                             |
| `common/`         | active   | Landing doc for the `common/*` sub-packages                                                                                                                                                                                                                                                                        |
| `common/errors/`  | active   | `ValidationError`, `NewValidationError`, `ErrInvalid*` sentinels — shared validation primitives used across all packages                                                                                                                                                                                           |
| `common/log/`     | active   | Package-level `Logger` (`*slog.Logger`) — the single logger used by all packages                                                                                                                                                                                                                                   |
| `common/netprim/` | active   | Network primitives: `NetworkNumber`, `NetworkPriority` + constants, `LocalNetwork`, `GlobalBroadcastNetwork`, `ProtocolVersion`, `IpDefaultUdpPort`; `Address`, `NewAddress` and methods                                                                                                                           |
| `common/types/`   | active   | Application-layer types: `DeviceInstance`, `ObjectType` + constants, `ObjectIdentifier`, `PropertyIdentifier` + constants, `RejectReason` + constants, `MaxInstanceNumber`, `MaxObjectType`                                                                                                                        |
| `bip/`            | active   | BACnet/IP + BACnet/IP6 BVLC frame encode/decode + UDP datagram transport scaffold; all 12 Annex J BVLC function types in `bvlc_functions.go`; `BBMD` interface + `bbmdImpl` (BDT/FDT management) in `bbmd.go`; `DeviceIp4` interface + `deviceImpl` (local broadcast + foreign device registration) in `device.go` |
| `apdu/`           | active   | BACnet application layer scaffold (ASE dispatch + invoke tracking + clause 5.4 state-machine scaffolding); typed `Client` wrapper (`client.go`, `client_discovery.go`, `client_object_access.go`, `client_cov.go`, `client_extend.go`); `UserElement` wrapper                                                      |
| `encoding/`       | active   | BACnet tag/value encoding; `ParseTag`, `EncodeOpeningTag`, `EncodeApplicationPrimitive`, `EncodeUnsigned`/`DecodeUnsigned`                                                                                                                                                                                         |
| `npdu/`           | active   | BACnet network layer NPDU encode/decode scaffold (NPCI parsing, routed/local APDU constructors, network-layer-message constructors)                                                                                                                                                                                |
| `npdu/router/`    | active   | Routing table; `Router` interface, `Evaluate`, `AddConnectedRoute`/`AddLearnedRoute`, `Decision` type                                                                                                                                                                                                              |
| `internal/util/`  | active   | Non-public `CopyPointersValue[T any](in *T) *T` — generic pointer copy using Go 1.26 `new(*in)` syntax                                                                                                                                                                                                             |
| `testdata/npdu/`  | active   | Wire conformance vectors `nlm_vectors.txt` (format: `name\|hex\|valid`)                                                                                                                                                                                                                                            |
| `examples/`       | deferred | Deferred until the API is stable                                                                                                                                                                                                                                                                                   |

## Technical requirements
- The project must be implemented purely in Go. The minimum go version is declared in the `go.mod` file.
- The project must not make use of cgo or any C libraries.
- The project must be well-documented and easy to use.
- The project must be efficient and lightweight, with minimal dependencies.
- The project must NOT use cgo. Ever. Make sure dependencies do not use cgo. If you need to use a dependency that uses cgo, find an alternative that does not use cgo, or notify the maintainer.

## Dependency requirements
- Do not use external dependencies unless absolutely necessary. If you need to use a dependency, make sure it is well-maintained and has a good reputation. Double check with the maintainer whether using a dependency is okay.

- Library internal dependencies must be top down in the OSI Layer Model (e.g. `apdu` can depend on `npdu`, but `npdu` cannot depend on `apdu`).
- The `common/*` sub-packages are the canonical home for types and values shared across multiple packages. The internal dependency order within `common/` is: `common/errors` → nothing; `common/log` → nothing; `common/netprim` → `common/errors`; `common/types` → `common/errors`.
- The `internal/util` package is available for shared non-public helpers (e.g. `CopyPointersValue[T]`); use it for cross-package helpers to avoid duplication, but do not create package-local copies of the same helper.
- If you notice a package needs to share code with another package, but the shared code does not fit cleanly into either package's public API, consider creating a new function in `internal/util`, or a new internal package. This is encouraged to avoid duplication of code and to keep the codebase organized.
- Values shared across packages must live in the appropriate `common/*` sub-package (e.g. `NetworkPriority` in `common/netprim`, `PropertyIdentifier` in `common/types`). Values that are only used internally by the library can be defined in an internal package (e.g. `internal/constants`).

## Development requirements
- The project is still in the prototype phase, therefor the public API is not yet stable and may be changed when needed.
- The project must be developed using best practices for Go development, including proper error handling.
- The project should include unit tests for all major functionality, with a goal of achieving at least 80% code coverage.
- Function, type, parameter, variable, constant or field names should be descriptive and follow Go naming conventions (e.g. `NewAddress`, `DeviceInstance`, `PropertyIdentifierPresentValue`).
- Functions should be annotated with comments that explain their purpose, parameters, preconditions, and return values.
- Types should be annotated with comments that explain their purpose and any important details about their behavior or constraints.
- Constants and global variables should be annotated with brief comments that explain their purpose and any important details about their behavior or constraints wherever their identifier is not enough.
- The packages define interface types for functionality like network (e.g. `DatagramConn` in `bip/transport.go`) to allow users to implement their own mocks for testing. 
  - The project should include tests that use these interfaces with in-memory implementations (e.g. `bip/transport_test.go`) to verify behavior without external dependencies.
- The `apdu` package follows the same interface-first pattern (`ASE`, `NPDUTransport` in `apdu/ase.go`); tests use in-memory transport implementations in `apdu/ase_test.go` to verify dispatch and confirmed invoke lifecycle behavior.
- The example directory is ignored for development until the project has a stable API (and this line is removed)
- Commands to run tests: `go test ./...`; and generate coverage: `go test -coverprofile=coverage.out ./...`

### Coding conventions
- **Constructor pattern**: exported constructor functions are named `NewX(args) (T, error)` and validate all inputs before returning (e.g. `NewObjectIdentifier`, `NewDeviceInstance`, `NewAddress`).
- **Validation errors**: use the `errors.NewValidationError` function from `common/errors` to construct the error
- **`String()` methods**: use the BACnet specification's hyphen-separated names (e.g. `"analog-input"`, `"object-identifier"`, `"present-value"`). Unknown values fall back to `"type-name(N)"` (e.g. `"object-type(2048)"`). Composite types use comma-separated `"type,instance"` format (e.g. `ObjectIdentifier.String()` returns `"device,1234"`).
- **Defensive copies**: slice-backed fields must be copied on both construction and access (see `NewAddress` and `Address.MACBytes()`).
- **APDU/NPDU byte ownership**: clone byte slices at public package boundaries (API/transport ingress and egress to callers), but avoid redundant cloning on internal ASE/state-machine paths once ownership is established. See `apdu/README.md`, `apdu/ase.go` (`OnInboundNPDU`), and `npdu/doc.go`.
- **`Valid()` methods**: types with numeric constraints expose a `Valid() bool` method (e.g. `DeviceInstance.Valid()`, `ObjectType.Valid()`). Types with both standard and proprietary ranges additionally expose `ValidStandard() bool` (e.g. `RejectReason`, `NlmRejectReason`, `NetworkLayerMessageType`).
- **`PropertyIdentifier`**: implemented in `common/types/types.go` as a starter subset with named constants (e.g. `PropertyIdentifierPresentValue`, `PropertyIdentifierObjectIdentifier`) and a `String()` fallback pattern (`property-identifier(N)` for unknown values). Follow the same pattern when adding new property identifiers.
- **Boundary error wrapping**: on encode/decode/transport boundaries, wrap sentinel errors with `%w` and include the original error text (e.g. `fmt.Errorf("%w: %v", ErrEncodeFailure, err)` in `apdu/ase.go`, and `ErrReadFailure`/`ErrWriteFailure` wrapping in `bip/transport.go`).
- **Types**: define types for everything that has a specific meaning in the BACnet context, even if the type is just a thin wrapper of a primitive (e.g. `type DuplicateCount uint8` or `type transactionKey string`); this allows for better type safety and more descriptive code.

### Test conventions
- Test files use the same package as the code under test (e.g. `package netprim`, `package apdu`, `package bip`) — **not** `*_test` external packages.
- Tests follow the table-driven pattern using `t.Run` with a `tests []struct{name, input, want/wantErr}` slice — use this for validation and `String()` coverage.
- Straight-line (non-table) tests are acceptable for mutation/side effect checks where a table adds no clarity (e.g. `TestNewAddressCopiesMAC`).
- Use `errors.Is(err, ErrXxx)` for all error assertions — never compare error strings directly.
- `npdu/npdu_test.go` is the reference for NPDU expectations: it covers known wire bytes, constructor validation, encode/decode roundtrips, priority preservation, and defensive-copy behavior.

## BACnet standard requirements
- The project must implement the BACnet standard as defined in ANSI/ASHRAE 135-2024.

## Versioning
- The project must follow semantic versioning (semver) for all releases. This means that version numbers should be in the format `MAJOR.MINOR.PATCH`, where:
  - `MAJOR` version is incremented for incompatible API changes,
  - `MINOR` version is incremented for added functionality in a backwards-compatible manner,
  - `PATCH` version is incremented for backwards-compatible bug fixes.
- The project uses Git for version control, and all releases must be tagged with the appropriate version number in the Git repository.
- The project uses commitizen for consistent commit messages, following the format `type(scope): description` (e.g. `feat(address): add NewAddress constructor`).
- All commits must be made with the appropriate type (e.g. `feat`, `fix`, `docs`, `refactor`, etc.) and scope (e.g. `address`, `types`, `errors`, etc.) to ensure clear commit history and accurate changelog generation.
- Commit/version automation is configured in `.cz.yaml` (`name: cz_conventional_commits`, `version_scheme: semver`, `major_version_zero: true`, `tag_format: $version`, `update_changelog_on_bump: true`); keep `Agents.md` guidance aligned with that file when release workflow changes.

# Some more info
- In go 1.26 (the current version as of this writing), new() has been updated to also accept an initial value not just types.
  - This means `v := new(2)` and `a := 2; v := &a` both result in v being a pointer to an int with value 2.

## Logging
- `log.Logger` is the single package-level `*slog.Logger` used by all packages — do not instantiate per-package loggers.
- Import: `"go.wdy.de/bacnet/common/log"`.
- Pattern: `log.Logger.Debug("description", "field", value, "error", err)`.

## `apdu` package gotchas
- Client-side segmented send/receive is **not implemented** (v1 scope). A segmented ComplexACK received by a client returns `ErrSegmentationNotSupported`. Server-side segmented receive and ComplexACK response are implemented.
- Duplicate confirmed requests (same src + invokeID while a handler is in-flight) are **silently dropped** per §5.4.4 — this is intentional.
- `ASEConfig` zero values for `PreferredWindowSize`, `MaxSegmentDuplicates`, `SegmentedTimedOutCollectorPeriod`, and `SegmentedRequestTimeout` are silently defaulted; do not treat 0 as "disabled".