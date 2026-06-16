# NPDU test vectors

`nlm_vectors.txt` contains BACnet network-layer-message conformance vectors used by `npdu` tests.

Format:

`name|wire_hex|valid`

- `name`: fixture identifier
- `wire_hex`: full network-layer-message wire bytes (`message-type`, optional `vendor-id`, payload)
- `valid`: `true` for vectors expected to decode successfully, `false` for vectors expected to be rejected

