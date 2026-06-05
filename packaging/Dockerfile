# Build environment for cross-compiling and packaging the ChirpStack Gateway
# Bridge vendor packages from any host (including macOS). It bundles the Go
# toolchain plus the packaging tools that are not available on macOS:
#   - opkg-build (from the Yocto opkg-utils source) for the .ipk vendors
#   - openssl for the Cisco .cpkg signing flow
#   - upx for compressing the flash-constrained MIPS binary
#
# Used by packaging/docker-build.sh. The repository is mounted at /src at run
# time, so this image only needs the tools, not the source.
FROM golang:1.22-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
        git \
        binutils \
        tar \
        gzip \
        xz-utils \
        file \
        openssl \
        ca-certificates \
        curl \
    && rm -rf /var/lib/apt/lists/* \
    && git clone --depth 1 https://git.yoctoproject.org/opkg-utils /opt/opkg-utils

# Debian has no arm64 UPX package, so install it from the official static
# release for the build host's architecture. A host-arch upx can compress the
# MIPS/ARM target binaries. Used only to shrink the flash-constrained MIPS
# binary, so this is best-effort and never fails the image build.
ARG TARGETARCH
ARG UPX_VERSION=4.2.4
RUN set -eux; \
    case "${TARGETARCH}" in \
        amd64) upx_arch=amd64 ;; \
        arm64) upx_arch=arm64 ;; \
        *)     upx_arch= ;; \
    esac; \
    if [ -n "${upx_arch}" ] \
        && curl -fsSL "https://github.com/upx/upx/releases/download/v${UPX_VERSION}/upx-${UPX_VERSION}-${upx_arch}_linux.tar.xz" -o /tmp/upx.tar.xz; then \
        tar -xJf /tmp/upx.tar.xz -C /tmp; \
        install -m0755 "/tmp/upx-${UPX_VERSION}-${upx_arch}_linux/upx" /usr/local/bin/upx; \
        rm -rf /tmp/upx.tar.xz "/tmp/upx-${UPX_VERSION}-${upx_arch}_linux"; \
        upx --version | head -1; \
    else \
        echo "upx unavailable for ${TARGETARCH}; MIPS compression will be skipped"; \
    fi

ENV PATH=/opt/opkg-utils:$PATH
WORKDIR /src
