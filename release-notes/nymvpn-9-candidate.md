# NymVPN CDN recovery candidate

Adds a bounded pipeline for XHTTP header uploads of at most 8 KiB per packet.
Up to 16 sequence numbers can be outstanding; later acknowledgements cannot
advance the window past a missing packet. Upload errors cancel the stream,
and close/cancellation wake blocked writers. Body upload modes retain a
single outstanding request. No changes to Android service or TUN ownership.

The Android canary workflow applies `core/patches/cdn-packet-pipeline.patch`
to the pinned Mihomo submodule before testing and building. For a local build,
apply the same patch with `git -C core/Clash.Meta apply
../patches/cdn-packet-pipeline.patch` once; do not commit patched vendor files
or apply it twice. The patch intentionally excludes the standalone diagnostic
listener adaptation used on Windows.

Windows isolated Ethernet checks: five repeated downloads, uploads, and Discord
gateway requests passed after bounding the reordering window. An 8 MiB upload
completed in 19.29 s. Existing core sent 128 KiB in 2.74–4.39 s with the same
6 KiB/1 ms settings. These are synthetic transport checks, not proof of mobile
network availability, Discord voice, or fleet recovery. APK installation and
real Android transfers remain unverified.

This candidate does not repair the failed direct FI/RU routes. Updating the
subscription supplies the separately deployed 6 KiB/1 ms CDN parameters.
