# NymVPN FlClash

Development fork of [chen08209/FlClash](https://github.com/chen08209/FlClash),
based on `c7be7023d33615cb624148d41414f80a7d96cede`. Upstream copyright notices
and GPL-3.0 license are retained. No subscriber credentials are included.

## Subscription-controlled port candidates

The Go wrapper expands the optional `x-nymvpn-ports` field on inline proxies
during import, validation, and configuration loading. Example (placeholder data):

```yaml
proxies:
  - name: FI-Reality
    type: vless
    server: example.invalid
    port: 443
    uuid: 00000000-0000-4000-8000-000000000001
    x-nymvpn-ports: [2022, 8448]
proxy-groups:
  - name: VPN
    type: select
    proxies: [FI-Reality]
rules:
  - MATCH,VPN
```

This produces `FI-Reality @2022` and `FI-Reality @8448`, next to the original
entry in explicit manual selectors. Only advertise ports actually served by
that protocol on the server. The example is not a deployable VPN configuration.

- Any integer port from 1 through 65535 is accepted. Ports are not scanned.
- At most 16 ports per node and 128 additional nodes per profile.
- VLESS, VMess, Trojan, Shadowsocks, Hysteria2, and TUIC are supported.
- Keys, TLS verification, SNI, transport options, and routing are preserved.
- Port hopping stays on the original HY2 node; generated candidates use one port.
- Duplicate ports collapse. Conflicting names and invalid values fail validation.
- Expansion is idempotent across YAML import, Dart JSON serialization, and reload.
- Explicit automatic group lists are unchanged. Existing `include-all`/filter
  groups retain their upstream semantics and may discover the additional nodes.
- Remote proxy-provider payloads are not expanded by this wrapper.

This feature does not add a new transport or cure a filtered network path. The
existing production subscription already contains eight explicit alternate-port
nodes compatible with stock FlClash; the owner reported that they did not work.
Do not treat a successful build or latency check as restored connectivity.

## Verification and Android build

```sh
git submodule update --init --recursive
cd core
CGO_ENABLED=0 go test . ./portprofile
CGO_ENABLED=0 go vet . ./portprofile
```

The manual `NymVPN Android canary` workflow runs these checks and builds an ARM64
release-mode APK using upstream's development signing fallback. It uses the
separate `com.follow.clash.dev` package, so it does not update the stable
`com.follow.clash` installation. A locally generated CI debug signing key is
not a durable release signing identity. Establish a private stable signing key
before distributing ongoing updates. Existing FlClash development builds can
conflict with this package; do not uninstall them to test this artifact.

No automatic update feed, production profile changes, new server listeners, or
new tunneling transports are activated by this fork. Android installation and
real transfers over Wi-Fi and mobile data still need device validation.

## Next networking work

Add bounded transfer diagnostics that distinguish a successful handshake from
a complete download and report the stage of failure without subscription secrets.
Integrate a new transport only after reproducing it independently and proving its
Android socket protection, DNS routing, cancellation, and service lifecycle.
The existing Telemost experiment is not implemented in this fork.
