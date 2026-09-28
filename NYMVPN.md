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
CGO_ENABLED=0 go test . ./telemost ./portprofile
CGO_ENABLED=0 go vet . ./telemost ./portprofile
```

The manual `NymVPN Android canary` workflow runs these checks, races the transport
lifecycle tests, and builds an ARM64 APK. Builds starting with `nymvpn.2` use
`com.follow.clash.nymvpn`, labelled **NymVPN FlClash**, and a durable private
signing identity held in GitHub Actions secrets. They install alongside both
stable FlClash and the first `.dev` canary. Import your subscription into this
separate installation. The fork does not initialize upstream Firebase reporting.

No automatic update feed or server listeners are created by the APK. Android
installation, permission/revoke callbacks, and transfers over Wi-Fi and mobile
data still need device validation.

## Experimental Telemost transport

An opt-in inline node embeds the public olcrtc client, pinned to
[`08843d6`](https://github.com/alananisimov/olcrtc/tree/08843d6accd7f43a1d04aa0ac7a3d5ea90e27efa)
(WTFPL). It requires an already provisioned Telemost/vp8channel server and private
room/key. Placeholder example, not a working subscription:

```yaml
proxies:
  - name: NymVPN-Telemost
    type: socks5
    server: 127.0.0.1
    port: 1
    udp: false
    x-nymvpn-telemost:
      room: YOUR_PRIVATE_ROOM_ID
      key: YOUR_PRIVATE_64_HEX_KEY
      dns: 77.88.8.8:53
```

Add this name to a manual selector. There is at most one tunnel per profile;
it opens lazily on first use, shared by concurrent requests. It provides TCP
only. UDP applications need another node; HTTPS should use TCP rather than QUIC.
Initial connection can take longer than the ordinary latency-test timeout.
Stock FlClash does not implement this extension and cannot use the placeholder.

The wrapper swaps the parsed SOCKS placeholder before applying the configuration;
selectors retain the same proxy reference. The internal SOCKS listener uses an
ephemeral loopback port and random per-session authentication. Android sockets
use the existing VpnService protection callback. Stop, shutdown, and config reload
cancel the transport through the existing Core lifecycle. Individual request
timeouts do not tear down the shared session. Failed sessions back off before
reconnecting. Secrets stay in the private profile, never in the APK.

Android is the intended test target. Windows supports explicit physical-interface
binding for diagnosis; the new transport fails closed on other platforms.
An opt-in network test requires `NYMVPN_TELEMOST_TEST_PROFILE`,
`NYMVPN_TELEMOST_TEST_INTERFACE`, and `NYMVPN_TELEMOST_EXPECTED_EGRESS` and runs
with `go test -run TestTelemostOptInNetwork -v .`. Its upstream logs can contain
private connection details: keep the complete output private.

On 2026-09-28 the embedded adapter and actual selector passed a Windows test
bound to Ethernet: expected server egress, Telegram HTTPS, all 5 MiB downloaded
in 6.95 seconds, then Telegram again. This is not proof of Android connectivity
or of availability on a particular mobile operator.
