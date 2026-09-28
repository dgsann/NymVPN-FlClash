# Owner-only protocol trial server

This Linux command runs the existing Mihomo inbound implementations for a
separate, privately provisioned AnyTLS/Mieru pilot. It is not part of the Android
app entrypoint. FlClash's embedded executor deliberately leaves listener startup
to its caller; this command starts the configured listeners and closes them on
SIGTERM/SIGINT. Startup fails if any listener fails.

Build from `core` using the pinned submodule and module dependencies:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o protocol-trials ./cmd/protocol-trials
```

Run with `-d STATE_DIRECTORY -f PRIVATE_CONFIG`; `-t` validates the config without
opening listeners. Deploy as a separate systemd DynamicUser service with
LoadCredential for the config, certificate and key. Set SAFE_PATHS to the exact
credential directory to permit certificate loading. Do not enable a TUN, public
controller or unauthenticated local proxy. Server routing must reject private,
loopback and link-local destinations before its final DIRECT rule.

The pilot client JSON and passwords belong outside this repository. AnyTLS uses
a pinned SHA-256 server certificate. Credential rotation/revocation is manual;
the pilot is not a multi-user billing integration.

`TestProtocolTrialsOptInNetwork` accepts `NYMVPN_PROTOCOL_TEST_NODES` and an
explicit `NYMVPN_PROTOCOL_TEST_INTERFACE`. It checks AMS egress, a full 5 MiB
download, a later request and a validated UDP DNS response for every node.
`loopback-control` disables interface binding for an explicitly prepared local
control connection. Control success does not establish direct-path reachability.

On 2026-09-28, all three protocols passed the AMS loopback control. AnyTLS and
Mieru TCP also passed the Windows SSH-forward control. All three direct Windows
Ethernet paths timed out. Phone Wi-Fi and mobile results are pending. They are
manual subscription entries and are not eligible for NymVPN-AutoTCP yet.
