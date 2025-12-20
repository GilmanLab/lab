#!/usr/bin/env bash
# Build VyOS Gateway Image
# Creates a raw disk image for the VP6630 gateway router using vyos-build
#
# Prerequisites:
#   - Docker
#   - SSH public key
#
# Usage:
#   ./build-vyos-image.sh [options]
#
# Options:
#   -o, --output DIR     Output directory (default: ./output-vyos)
#   -k, --ssh-key PATH   SSH public key file (default: ~/.ssh/id_rsa.pub)
#   -v, --version VER    VyOS version string (default: timestamp)
#   -h, --help           Show this help message
#
# Network configuration is embedded in the build flavor at:
#   infrastructure/network/vyos/vyos-build/build-flavors/gateway.toml

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
VYOS_BUILD_DIR="${REPO_ROOT}/infrastructure/network/vyos/vyos-build"

# Defaults
OUTPUT_DIR="${SCRIPT_DIR}/output-vyos"
SSH_KEY_FILE="${HOME}/.ssh/id_rsa.pub"
VERSION="$(date +%Y%m%d%H%M%S)"
BUILD_BY="genesis@lab.gilman.io"

usage() {
    head -20 "$0" | grep -E '^#' | sed 's/^# \?//'
    exit 0
}

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[ERROR] $*" >&2
    exit 1
}

check_prerequisites() {
    log "Checking prerequisites..."

    if ! command -v docker &>/dev/null; then
        error "Docker not found. Install Docker to continue."
    fi

    if ! docker info &>/dev/null; then
        error "Docker daemon not running or not accessible."
    fi

    # Check SSH key exists
    if [[ ! -f "${SSH_KEY_FILE}" ]]; then
        error "SSH public key not found: ${SSH_KEY_FILE}\nUse --ssh-key to specify a different key file."
    fi

    log "Prerequisites satisfied"
}

generate_flavor() {
    log "Generating build flavor with SSH credentials..."

    # Extract SSH key components
    SSH_KEY_TYPE=$(awk '{print $1}' "${SSH_KEY_FILE}")
    SSH_KEY_BODY=$(awk '{print $2}' "${SSH_KEY_FILE}")

    if [[ -z "${SSH_KEY_TYPE}" ]] || [[ -z "${SSH_KEY_BODY}" ]]; then
        error "Invalid SSH public key format in ${SSH_KEY_FILE}"
    fi

    log "  SSH Key Type: ${SSH_KEY_TYPE}"

    # Create temp directory for build files
    BUILD_TEMP=$(mktemp -d)
    trap "rm -rf ${BUILD_TEMP}" EXIT

    # Generate flavor from template
    TEMPLATE_FILE="${VYOS_BUILD_DIR}/build-flavors/gateway.toml"
    GENERATED_FLAVOR="${BUILD_TEMP}/gateway.toml"

    if [[ ! -f "${TEMPLATE_FILE}" ]]; then
        error "Flavor template not found: ${TEMPLATE_FILE}"
    fi

    sed -e "s|%%SSH_KEY_TYPE%%|${SSH_KEY_TYPE}|g" \
        -e "s|%%SSH_PUBLIC_KEY%%|${SSH_KEY_BODY}|g" \
        "${TEMPLATE_FILE}" > "${GENERATED_FLAVOR}"

    log "Generated flavor: ${GENERATED_FLAVOR}"
}

run_vyos_build() {
    log "Starting vyos-build..."
    log "  Version: ${VERSION}"
    log "  Build By: ${BUILD_BY}"
    log "  Output: ${OUTPUT_DIR}"

    mkdir -p "${OUTPUT_DIR}"

    # Pull the vyos-build container
    log "Pulling vyos-build container..."
    docker pull vyos/vyos-build:current

    # Run the build inside the container
    # The container needs:
    #   - Privileged mode for raw disk image creation
    #   - /dev access for disk operations
    #   - Generated flavor file copied to build-flavors directory
    log "Running VyOS image build..."

    docker run --rm --privileged \
        -v "${BUILD_TEMP}/gateway.toml:/vyos/data/build-flavors/gateway.toml:ro" \
        -v "${OUTPUT_DIR}:/output" \
        -v /dev:/dev \
        -e VYOS_BUILD_BY="${BUILD_BY}" \
        -e VYOS_VERSION="${VERSION}" \
        vyos/vyos-build:current \
        bash -c "
            set -e
            echo 'Building VyOS gateway image...'
            cd /vyos
            sudo ./build-vyos-image \
                --architecture amd64 \
                --build-by '${BUILD_BY}' \
                --build-type release \
                --version '${VERSION}' \
                gateway

            echo 'Copying output files...'
            if [ -d /vyos/build ]; then
                cp -v /vyos/build/*.raw /output/ 2>/dev/null || true
                cp -v /vyos/build/*.qcow2 /output/ 2>/dev/null || true
            fi

            echo 'Build complete!'
        "

    log "vyos-build completed successfully!"
}

show_results() {
    echo ""
    echo "=============================================="
    echo "VyOS Gateway Image Build Complete"
    echo "=============================================="
    echo ""
    echo "Output directory: ${OUTPUT_DIR}"
    if [[ -d "${OUTPUT_DIR}" ]]; then
        echo ""
        echo "Files:"
        ls -lah "${OUTPUT_DIR}/"
    fi
    echo ""
    echo "Next steps:"
    echo "  1. Upload image to e2 storage for Synology Cloud Sync:"
    echo "     labctl images upload ${OUTPUT_DIR}/vyos-*.raw"
    echo ""
    echo "  2. Or copy directly to NAS:"
    echo "     scp ${OUTPUT_DIR}/vyos-*.raw nas:/volume1/images/vyos/"
    echo ""
    echo "  3. Or write directly to USB/SSD for manual install:"
    echo "     sudo dd if=${OUTPUT_DIR}/vyos-*.raw of=/dev/sdX bs=4M status=progress"
    echo ""
    echo "Network configuration is embedded in the build flavor at:"
    echo "  infrastructure/network/vyos/vyos-build/build-flavors/gateway.toml"
    echo ""
}

main() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -o|--output)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            -k|--ssh-key)
                SSH_KEY_FILE="$2"
                shift 2
                ;;
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            -h|--help)
                usage
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done

    log "VyOS Gateway Image Builder (vyos-build)"
    log "Repository root: ${REPO_ROOT}"

    check_prerequisites
    generate_flavor
    run_vyos_build
    show_results
}

main "$@"
