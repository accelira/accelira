#!/bin/bash

set -e

# Define variables
BUILD_DIR="build"
PACKAGE_NAME="accelira"
VERSION="v1.0.0"
TAR_NAME="${PACKAGE_NAME}-${VERSION}.tar.gz"

# Clean up previous builds
rm -rf ${BUILD_DIR}
mkdir -p ${BUILD_DIR}

# Build the project
go build -v -o ${BUILD_DIR}/${PACKAGE_NAME}

# Create the tarball
tar -czvf ${TAR_NAME} -C ${BUILD_DIR} ${PACKAGE_NAME}

# Output the tarball name
echo "Generated tarball: ${TAR_NAME}"
