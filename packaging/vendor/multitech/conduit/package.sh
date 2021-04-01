#!/bin/bash

PACKAGE_NAME="chirpstack-gateway-bridge"
PACKAGE_VERSION=$1
REV="r1"


# PACKAGE_URL="https://artifacts.chirpstack.io/downloads/chirpstack-gateway-bridge/chirpstack-gateway-bridge_${PACKAGE_VERSION}_linux_armv5.tar.gz"
PACKAGE_FILE="../../../../dist/chirpstack-gateway-bridge_${PACKAGE_VERSION}_linux_armv5.tar.gz"
DIR=`dirname $0`
PACKAGE_DIR="${DIR}/package"

# Cleanup
rm -rf $PACKAGE_DIR

# CONTROL
mkdir -p $PACKAGE_DIR/CONTROL
cat > $PACKAGE_DIR/CONTROL/control << EOF
Package: $PACKAGE_NAME
Version: $PACKAGE_VERSION-$REV
Architecture: arm926ejste
Maintainer: myDevices, Inc. <support@mydevices.com>
Priority: optional
Section: network
Source: N/A
Description: ChirpStack Gateway Bridge
EOF

cat > $PACKAGE_DIR/CONTROL/postinst << EOF
update-rc.d chirpstack-gateway-bridge defaults
/etc/init.d/chirpstack-gateway-bridge start
EOF
chmod 755 $PACKAGE_DIR/CONTROL/postinst

cat > $PACKAGE_DIR/CONTROL/prerm << EOF
/etc/init.d/chirpstack-gateway-bridge stop
EOF
chmod 755 $PACKAGE_DIR/CONTROL/prerm

cat > $PACKAGE_DIR/CONTROL/postrm << EOF
update-rc.d chirpstack-gateway-bridge remove -f
EOF
chmod 755 $PACKAGE_DIR/CONTROL/postrm

cat > $PACKAGE_DIR/CONTROL/conffiles << EOF
/etc/opt/$PACKAGE_NAME/$PACKAGE_NAME.toml
EOF

# Files
mkdir -p $PACKAGE_DIR/opt/$PACKAGE_NAME
mkdir -p $PACKAGE_DIR/opt/mydevices
mkdir -p $PACKAGE_DIR/etc/opt/$PACKAGE_NAME
mkdir -p $PACKAGE_DIR/etc/init.d

cp files/$PACKAGE_NAME.toml $PACKAGE_DIR/etc/opt/$PACKAGE_NAME/$PACKAGE_NAME.toml
cp files/$PACKAGE_NAME.init $PACKAGE_DIR/etc/init.d/$PACKAGE_NAME
cp files/command-ctrl.sh $PACKAGE_DIR/opt/mydevices/
chmod 755 $PACKAGE_DIR/opt/mydevices/command-ctrl.sh
cp $PACKAGE_FILE $PACKAGE_DIR/opt/$PACKAGE_NAME 
tar zxf $PACKAGE_DIR/opt/$PACKAGE_NAME/*.tar.gz -C $PACKAGE_DIR/opt/$PACKAGE_NAME
rm $PACKAGE_DIR/opt/$PACKAGE_NAME/*.tar.gz

# Package
opkg-build -o root -g root $PACKAGE_DIR

# Cleanup
rm -rf $PACKAGE_DIR
