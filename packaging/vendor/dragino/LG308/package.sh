#!/usr/bin/env bash

PACKAGE_NAME="chirpstack-gateway-bridge"
PACKAGE_VERSION=$1
REV="r1"


BUILD_ARCH="mips"   # cross-compile label produced by packaging/build-binaries.sh
DIR=`dirname $0`
PACKAGE_DIR="${DIR}/package"

# Cleanup
rm -rf $PACKAGE_DIR

# CONTROL
mkdir -p $PACKAGE_DIR/CONTROL
cat > $PACKAGE_DIR/CONTROL/control << EOF
Package: $PACKAGE_NAME
Version: $PACKAGE_VERSION-$REV
Architecture: mips_24kc
Maintainer: Orne Brocaar <info@brocaar.com>
Priority: optional
Section: network
Source: N/A
Description: ChirpStack Gateway Bridge
EOF

cat > $PACKAGE_DIR/CONTROL/postinst << EOF
#!/bin/sh
/etc/init.d/chirpstack-gateway-bridge enable
EOF
chmod 755 $PACKAGE_DIR/CONTROL/postinst

cat > $PACKAGE_DIR/CONTROL/conffiles << EOF
/etc/$PACKAGE_NAME/$PACKAGE_NAME.toml
EOF

# Files
mkdir -p $PACKAGE_DIR/opt/$PACKAGE_NAME
mkdir -p $PACKAGE_DIR/etc/$PACKAGE_NAME
mkdir -p $PACKAGE_DIR/etc/init.d

cp files/$PACKAGE_NAME.toml $PACKAGE_DIR/etc/$PACKAGE_NAME/$PACKAGE_NAME.toml
cp files/$PACKAGE_NAME.init $PACKAGE_DIR/etc/init.d/$PACKAGE_NAME
# This fork bundles the locally cross-compiled binary instead of the upstream
# release. Run packaging/build-binaries.sh first to produce build/${BUILD_ARCH}/.
BINARY="${DIR}/../../../../build/${BUILD_ARCH}/${PACKAGE_NAME}"
if [ ! -f "$BINARY" ]; then
	echo "error: ${BINARY} not found; run packaging/build-binaries.sh ${PACKAGE_VERSION} first" >&2
	exit 1
fi
cp "$BINARY" $PACKAGE_DIR/opt/$PACKAGE_NAME/$PACKAGE_NAME

# Package
opkg-build -c -o root -g root $PACKAGE_DIR

# Cleanup
rm -rf $PACKAGE_DIR
