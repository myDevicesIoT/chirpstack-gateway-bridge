# Vendor packaging

This directory builds installable gateway packages (`.ipk` / Cisco `.cpkg`) for
the supported LoRaWAN gateways.

> **Fork note:** Unlike upstream, these packages bundle a binary that is
> **cross-compiled locally from this repository's source** — including any local
> code changes. The vendor `package.sh` scripts no longer download the release
> from `artifacts.chirpstack.io`.

## Quick start (recommended: Docker)

From the repository root, on any host (including macOS):

```bash
packaging/docker-build.sh                      # all vendors, version from git
packaging/docker-build.sh 3.17.8               # all vendors, explicit version
packaging/docker-build.sh 3.17.8 dragino/LG308 # one vendor only
```

This builds a Linux container with the full toolchain, cross-compiles the fork
binary, and runs each vendor's packaging step. Artifacts land under
`packaging/vendor/<vendor>/` (gitignored):

- `*.ipk` — opkg vendors (Dragino, Multitech, Kerlink, Tektelic)
- `*_<ver>_r1.tar.gz` — Cisco IXM-LPWA signed cpkg bundle

Go module and build caches are kept in named Docker volumes
(`cgwb-pkg-gomod`, `cgwb-pkg-gocache`) so repeat runs are fast.

## How it works

The flow has two stages:

### 1. Cross-compile the binary — `build-binaries.sh`

```bash
packaging/build-binaries.sh [version]   # version defaults to `git describe`
```

Cross-compiles `cmd/chirpstack-gateway-bridge` for every gateway architecture
into `build/<label>/chirpstack-gateway-bridge`:

| label  | GOARCH | flags              | gateways                              |
|--------|--------|--------------------|---------------------------------------|
| `mips` | mips   | `GOMIPS=softfloat` | Dragino LG308 (OpenWrt)               |
| `armv5`| arm    | `GOARM=5`          | Multitech Conduit, Tektelic Kona, Cisco IXM-LPWA |
| `armv7`| arm    | `GOARM=7`          | Kerlink keros-gws                     |

If [`upx`](https://upx.github.io/) is installed, the flash-constrained MIPS
binary is compressed (mirroring the `.goreleaser` `compress-mips` hook).
Otherwise compression is skipped with a warning.

### 2. Build each vendor package — `vendor/<vendor>/package.sh`

Each script picks up the matching binary via its `BUILD_ARCH` label
(`build/<BUILD_ARCH>/chirpstack-gateway-bridge`) and assembles the vendor
package. If the binary is missing it fails with a clear message telling you to
run `build-binaries.sh` first.

`docker-build.sh` ties stages 1 and 2 together inside the container; you only
need to invoke the stage scripts directly when building natively on Linux.

## Building natively on Linux

If you are already on Linux with the packaging tools installed
(`opkg-build` from [opkg-utils](https://git.yoctoproject.org/opkg-utils),
`openssl`, optionally `upx`), you can skip Docker:

```bash
packaging/build-binaries.sh 3.17.8
cd packaging/vendor/dragino/LG308 && ./package.sh 3.17.8
```

## The build container — `Dockerfile`

`docker-build.sh` builds this image. It provides the tools that aren't readily
available on macOS:

- **opkg-build** — cloned from the Yocto `opkg-utils` source (no longer
  packaged by Debian/Ubuntu) for the `.ipk` vendors.
- **openssl** — for the Cisco `.cpkg` signing flow.
- **upx** — installed from the official static release (Debian has no arm64
  package); used only to shrink the MIPS binary, so it's best-effort and never
  fails the image build.

The repository is mounted at `/src` at run time, so the image only carries the
tools, not the source.
