#!/usr/bin/env bash

# Build the vendor gateway packages inside a Linux container, so that
# cross-compilation and opkg-build / cpkg packaging work from macOS (or any
# host without opkg-utils). The fork's local source is mounted and compiled, so
# the resulting packages contain this repo's binary, not the upstream release.
#
# Output artifacts land under packaging/vendor/<vendor>/ (gitignored):
#   *.ipk            opkg vendors (Dragino, Multitech, Kerlink, Tektelic)
#   *_<ver>_r1.tar.gz  Cisco IXM-LPWA signed cpkg bundle
#
# Usage: packaging/docker-build.sh [version] [vendor ...]
#   version  default: `git describe --always` without the leading "v"
#   vendor   one or more subpaths under packaging/vendor (default: all)
#
# Examples:
#   packaging/docker-build.sh                         # all vendors, git version
#   packaging/docker-build.sh 3.17.8 dragino/LG308    # one vendor, explicit ver

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

IMAGE="chirpstack-gateway-bridge-pkg"
VERSION="${1:-$(git describe --always | sed -e 's/^v//')}"
[ $# -gt 0 ] && shift || true

VENDORS=("$@")
if [ ${#VENDORS[@]} -eq 0 ]; then
	VENDORS=(
		dragino/LG308
		multitech/conduit
		kerlink/keros-gws
		tektelic/kona
		cisco/IXM-LPWA
	)
fi

echo ">> building packaging image ${IMAGE}"
docker build -t "${IMAGE}" packaging

echo ">> cross-compiling + packaging ${VERSION} for: ${VENDORS[*]}"
# Named volumes cache Go modules and build artifacts across runs for speed.
docker run --rm \
	-v "${ROOT}:/src" \
	-v cgwb-pkg-gomod:/go/pkg/mod \
	-v cgwb-pkg-gocache:/root/.cache/go-build \
	-w /src \
	"${IMAGE}" \
	bash -euc '
		VERSION="$1"; shift
		git config --global --add safe.directory /src
		packaging/build-binaries.sh "$VERSION"
		for v in "$@"; do
			echo "=== packaging ${v} ==="
			( cd "packaging/vendor/${v}" && ./package.sh "$VERSION" )
		done
	' _ "${VERSION}" "${VENDORS[@]}"

echo ">> done. artifacts:"
find packaging/vendor -maxdepth 3 \( -name '*.ipk' -o -name "*_${VERSION}_*.tar.gz" \) -print 2>/dev/null
