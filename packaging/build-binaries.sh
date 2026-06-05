#!/usr/bin/env bash

# Cross-compile the ChirpStack Gateway Bridge binary for every gateway
# architecture used by the vendor packages in packaging/vendor/.
#
# This fork ships its own binary (including local code changes), so the vendor
# package.sh scripts consume the output of this script instead of downloading
# the upstream release from artifacts.chirpstack.io.
#
# Output: build/<label>/chirpstack-gateway-bridge
#   label  goarch  notes
#   mips    mips   GOMIPS=softfloat   (Dragino / OpenWrt)
#   armv5   arm    GOARM=5            (Multitech Conduit, Tektelic Kona, Cisco)
#   armv7   arm    GOARM=7            (Kerlink)
#
# Usage: packaging/build-binaries.sh [version]
#   version defaults to `git describe --always` (matching the Makefile).

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PACKAGE_NAME="chirpstack-gateway-bridge"
SRC="cmd/${PACKAGE_NAME}/main.go"
VERSION="${1:-$(git describe --always | sed -e 's/^v//')}"
LDFLAGS="-s -w -X main.version=${VERSION}"

build() {
	local label="$1"; shift
	echo "Building ${label} (${*})"
	mkdir -p "build/${label}"
	env "$@" go build -ldflags "${LDFLAGS}" -o "build/${label}/${PACKAGE_NAME}" "${SRC}"
}

build mips  GOOS=linux GOARCH=mips GOMIPS=softfloat
build armv5 GOOS=linux GOARCH=arm  GOARM=5
build armv7 GOOS=linux GOARCH=arm  GOARM=7

# Compress the flash-constrained MIPS binary when upx is available
# (mirrors the .goreleaser compress-mips post-build hook).
if command -v upx >/dev/null 2>&1; then
	echo "Compressing MIPS binary"
	upx -q "build/mips/${PACKAGE_NAME}"
else
	echo "upx not found; skipping MIPS binary compression"
fi

echo "Done. Binaries in build/<label>/${PACKAGE_NAME} (version ${VERSION})"
