#!/bin/bash
# VyOS Gateway Image Build Script
#
# Builds a raw disk image using the vyos-build Docker container.
# This script is designed to run inside the vyos-build container.
#
# Usage (inside container):
#   ./build.sh
#
# Environment variables (must be set before running):
#   VYOS_BUILD_BY   - Builder identifier (e.g., "ci@lab.gilman.io")
#   VYOS_VERSION    - Version string (optional, defaults to timestamp)
#
# Prerequisites:
#   - Running inside vyos/vyos-build:current container
#   - Flavor TOML with SSH credentials already generated at /vyos/build-flavors/gateway.toml
#   - Privileged container with /dev access for raw image creation

set -euo pipefail

# Configuration
BUILD_BY="${VYOS_BUILD_BY:-ci@lab.gilman.io}"
VERSION="${VYOS_VERSION:-$(date +%Y%m%d%H%M%S)}"
FLAVOR_NAME="gateway"
OUTPUT_DIR="/vyos/build"

echo "=== VyOS Gateway Image Build ==="
echo "Build By: ${BUILD_BY}"
echo "Version: ${VERSION}"
echo "Flavor: ${FLAVOR_NAME}"
echo ""

# Verify we're in the right environment
if [[ ! -f "/vyos/build-vyos-image" ]]; then
    echo "ERROR: build-vyos-image not found. Are you inside the vyos-build container?"
    echo ""
    echo "Run this script inside the container:"
    echo "  docker run --rm -it --privileged \\"
    echo "    -v \$(pwd):/vyos-lab \\"
    echo "    -v /dev:/dev \\"
    echo "    vyos/vyos-build:current bash"
    exit 1
fi

# Verify flavor file exists
FLAVOR_FILE="/vyos/data/build-flavors/${FLAVOR_NAME}.toml"
if [[ ! -f "${FLAVOR_FILE}" ]]; then
    echo "ERROR: Flavor file not found: ${FLAVOR_FILE}"
    echo "Make sure to copy the generated flavor TOML to this location."
    exit 1
fi

echo "Using flavor: ${FLAVOR_FILE}"
echo ""

# Clean previous builds
echo "Cleaning previous builds..."
sudo make clean || true

# Build the image
echo ""
echo "=== Starting VyOS Image Build ==="
echo ""

sudo ./build-vyos-image \
    --architecture amd64 \
    --build-by "${BUILD_BY}" \
    --build-type release \
    --version "${VERSION}" \
    "${FLAVOR_NAME}"

# Check for output
echo ""
echo "=== Build Complete ==="

if [[ -d "${OUTPUT_DIR}" ]]; then
    echo "Output files:"
    ls -lah "${OUTPUT_DIR}/"
else
    echo "WARNING: Output directory not found: ${OUTPUT_DIR}"
fi
