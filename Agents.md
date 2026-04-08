## Project description
This project is a go implementation of the Bacnet standard. It is designed to be a lightweight and efficient library for building Bacnet applications in Go.
The project focuses on the IP protocol, other Bacnet protocols may be implemented in the future, but are NOT the current scope of this project.

The Go module path is `go.wdy.de/bacnet`.

### Package layout

| Path | Status | Purpose |
|---|---|---|
| `.` (`bacnet`) | active | Constants, core types, errors, addressing primitives |
| `bip/` | active | BACnet/IP + BACnet/IP6 BVLC frame encode/decode + UDP datagram transport scaffold; all 12 Annex J BVLC function types in `bvlc_functions.go`; `BBMD` interface + `bbmdImpl` (BDT/FDT management) in `client.go` |
| `apdu/` | active | BACnet application layer scaffold (ASE dispatch + invoke tracking) |
| `encoding/` | planned | BACnet tag/value encoding |
| `npdu/` | planned | BACnet network layer |
| `internal/buffer/` | planned | Non-public buffer helpers |
| `internal/testutil/` | planned | Non-public test utilities |
| `testdata/` | planned | Binary packet fixtures |
| `examples/` | deferred | Deferred until the API is stable |

## Technical requirements
- The project must be implemented in Go (minimum version: 1.26.1, as declared in `go.mod`).
- The project must be well-documented and easy to use.
- The project must be efficient and lightweight, with minimal dependencies.
- The project must NOT use cgo. Ever. Make sure dependencies do not use cgo. If you need to use a dependency that uses cgo, find an alternative that does not use cgo, or notify the user.

## Dependency requirements
- Do not use dependencies unless absolutely necessary. If you need to use a dependency, make sure it is well-maintained and has a good reputation. Double check with the user whether using a dependency is okay.
- The current baseline is standard-library-only (`go.mod` has no external module requirements); preserve that unless a reviewed exception is explicitly approved.

## Development requirements
- The project must be developed using best practices for Go development, including proper error handling.
- The project should include unit tests for all major functionality, with a goal of achieving at least 80% code coverage.
- Functions should be annotated with comments that explain their purpose, parameters, preconditions, and return values.
- The packages define interface types for functionality like network (e.g. `DatagramConn` in `bip/transport.go`) to allow users to implement their own mocks for testing. 
  - The project should include tests that use these interfaces with in-memory implementations (e.g. `bip/transport_test.go`) to verify behavior without external dependencies.
- The `apdu` package follows the same interface-first pattern (`Codec`, `Transport` in `apdu/ase.go`); tests use in-memory implementations in `apdu/ase_test.go` to verify dispatch and confirmed invoke lifecycle behavior.
- In `bip`, prefer address-family helpers at integration boundaries: use `NewFrameForAddress` / `NewFrameWithType` in `bip/frame.go` and `NewDatagramConn` in `bip/transport.go` so IPv4 (`0x81`/`udp4`) and IPv6 (`0x82`/`udp6`) behavior stays consistent.
- The example directory is ignored for development until the project has a stable API (and this line is removed)
- Run tests: `go test ./...`; generate coverage: `go test -coverprofile=coverage.out ./...`

### Coding conventions
- **Constructor pattern**: exported constructor functions are named `NewX(args) (T, error)` and validate all inputs before returning (e.g. `NewObjectIdentifier`, `NewDeviceInstance`, `NewAddress`).
- **Validation errors**: use the NewValidationError function to construct the error
- **`String()` methods**: use the BACnet specification's hyphen-separated names (e.g. `"analog-input"`, `"object-identifier"`, `"present-value"`). Unknown values fall back to `"type-name(N)"` (e.g. `"object-type(2048)"`). Composite types use comma-separated `"type,instance"` format (e.g. `ObjectIdentifier.String()` returns `"device,1234"`).
- **Defensive copies**: slice-backed fields must be copied on both construction and access (see `NewAddress` and `Address.MACBytes()`).
- **`Valid()` methods**: types with numeric constraints expose a `Valid() bool` method (e.g. `DeviceInstance.Valid()`, `ObjectType.Valid()`).
- **`PropertyIdentifier`**: implemented in `types.go` as a starter subset with named constants (e.g. `PropertyIdentifierPresentValue`, `PropertyIdentifierObjectIdentifier`) and a `String()` fallback pattern (`property-identifier(N)` for unknown values). Follow the same pattern when adding new property identifiers.
- **Boundary error wrapping**: on encode/decode/transport boundaries, wrap sentinel errors with `%w` and include the original error text (e.g. `fmt.Errorf("%w: %v", ErrEncodeFailure, err)` in `apdu/ase.go`, and `ErrReadFailure`/`ErrWriteFailure` wrapping in `bip/transport.go`).
- **BVLC function struct pattern**: each Annex J function is a pointer-receiver struct embedding a private `BVLCHeader` field and implementing the `BVLCFunction` interface (`BVLCFunctionType()`, `Valid()`, `Encode() ([]byte, error)`, `Decode([]byte) error`). Constructors (`NewOriginalUnicastNpdu`, `NewForwardedNpdu`, `NewRegisterForeignDevice`, etc.) validate all inputs, clone slice fields, and compute the `BVLCLength` field from the total wire size. See `bip/bvlc_functions.go`.
- **BBMD pattern**: `BBMD` in `bip/client.go` is an interface with one `Handle*` method per inbound BVLC function (0x01, 0x02, 0x05, 0x06, 0x08). `NewBBMD(bdt []BdtEntry) (BBMD, error)` constructs `bbmdImpl`, which holds a `sync.RWMutex`-protected BDT slice and `map[netip.AddrPort]bbmdFdtEntry` FDT. The internal `bbmdFdtEntry` stores the registered TTL and an absolute `expiresAt` time (`now + ttl + 30s` per §J.5.2.3); `toWireFdtEntry()` sets unexported `FdtEntry` fields directly (same package) to encode the current remaining TTL. Expired entries are purged lazily in `HandleReadForeignDeviceTable`.
- **`BVLCHeader`** is the shared internal header struct (`BVLCType` + `BVLCFunctionType` + `BVLCLength`) used in every BVLC function struct. `BVLCLength` has a `NewBVLCLength(int) (BVLCLength, error)` constructor for range-checked construction. Use `BVLCHeader.Encode()`/`BVLCHeader.Decode()` rather than encoding the header fields manually.

### Test conventions
- Test files use the same package as the code under test (e.g. `package bacnet`, `package apdu`, `package bip`) — **not** `*_test` external packages.
- Tests follow the table-driven pattern using `t.Run` with a `tests []struct{name, input, want/wantErr}` slice — use this for validation and `String()` coverage.
- Straight-line (non-table) tests are acceptable for mutation/side-effect checks where a table adds no clarity (e.g. `TestNewAddressCopiesMAC`).
- Use `errors.Is(err, ErrXxx)` for all error assertions — never compare error strings directly.

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
- Commit/version automation is configured in `.cz.yaml` (`name: cz_conventional_commits`, `version_scheme: semver`, `major_version_zero: true`, `tag_format: $version`, `update_changelog_on_bump: true`); keep `AGENTS.md` guidance aligned with that file when release workflow changes.
