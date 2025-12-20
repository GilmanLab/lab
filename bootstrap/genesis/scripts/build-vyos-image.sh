#!/usr/bin/env bash
# Build VyOS Gateway Image
# Creates a raw disk image for the VP6630 gateway router
#
# Prerequisites:
#   - Packer >= 1.9.0
#   - QEMU with KVM support
#   - VyOS ISO (downloaded automatically or provided)
#
# Usage:
#   ./build-vyos-image.sh [options]
#
# Options:
#   -i, --iso PATH       Path to VyOS ISO (skips download)
#   -o, --output DIR     Output directory (default: output-vyos)
#   -k, --ssh-key PATH   SSH public key file (default: ~/.ssh/id_rsa.pub)
#   -h, --help           Show this help message
#
# Network configuration is defined in:
#   infrastructure/network/vyos/configs/gateway.conf

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
PACKER_DIR="${REPO_ROOT}/infrastructure/network/vyos/packer"

# Defaults
VYOS_ISO=""
OUTPUT_DIR="${PACKER_DIR}/output-vyos"
SSH_KEY_FILE="${HOME}/.ssh/id_rsa.pub"

# VyOS download settings
VYOS_VERSION="1.5-rolling-202412190007"
VYOS_URL="https://github.com/vyos/vyos-rolling-nightly-builds/releases/download/${VYOS_VERSION}/vyos-${VYOS_VERSION}-amd64.iso"
VYOS_CACHE_DIR="${HOME}/.cache/vyos"

usage() {
    head -30 "$0" | grep -E '^#' | sed 's/^# \?//'
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

    if ! command -v packer &>/dev/null; then
        error "Packer not found. Install with: brew install packer"
    fi

    if ! command -v qemu-system-x86_64 &>/dev/null; then
        error "QEMU not found. Install with: brew install qemu"
    fi

    # Check KVM availability (Linux only)
    if [[ "$(uname)" == "Linux" ]] && [[ ! -r /dev/kvm ]]; then
        error "KVM not available. Ensure virtualization is enabled and you have access to /dev/kvm"
    fi

    # Check SSH key exists
    if [[ ! -f "${SSH_KEY_FILE}" ]]; then
        error "SSH public key not found: ${SSH_KEY_FILE}\nUse --ssh-key to specify a different key file."
    fi

    log "Prerequisites satisfied"
}

download_vyos_iso() {
    if [[ -n "${VYOS_ISO}" ]]; then
        if [[ ! -f "${VYOS_ISO}" ]]; then
            error "Specified ISO not found: ${VYOS_ISO}"
        fi
        log "Using provided ISO: ${VYOS_ISO}"
        return
    fi

    mkdir -p "${VYOS_CACHE_DIR}"
    VYOS_ISO="${VYOS_CACHE_DIR}/vyos-${VYOS_VERSION}-amd64.iso"

    if [[ -f "${VYOS_ISO}" ]]; then
        log "Using cached ISO: ${VYOS_ISO}"
        return
    fi

    log "Downloading VyOS ${VYOS_VERSION}..."
    log "URL: ${VYOS_URL}"

    if ! curl -fSL -o "${VYOS_ISO}.tmp" "${VYOS_URL}"; then
        rm -f "${VYOS_ISO}.tmp"
        error "Failed to download VyOS ISO"
    fi

    mv "${VYOS_ISO}.tmp" "${VYOS_ISO}"
    log "Downloaded: ${VYOS_ISO}"
}

get_ssh_key_type() {
    # Extract key type (first field: ssh-rsa, ssh-ed25519, etc.)
    awk '{print $1}' "${SSH_KEY_FILE}"
}

get_ssh_key_body() {
    # Extract key body (second field: base64 encoded key)
    awk '{print $2}' "${SSH_KEY_FILE}"
}

run_packer_build() {
    log "Starting Packer build..."

    cd "${PACKER_DIR}"

    # Initialize Packer plugins
    log "Initializing Packer plugins..."
    packer init .

    # Get SSH key type and body
    SSH_KEY_TYPE=$(get_ssh_key_type)
    SSH_KEY_BODY=$(get_ssh_key_body)

    # Calculate ISO checksum
    log "Calculating ISO checksum..."
    if command -v sha256sum &>/dev/null; then
        ISO_CHECKSUM="sha256:$(sha256sum "${VYOS_ISO}" | awk '{print $1}')"
    else
        ISO_CHECKSUM="sha256:$(shasum -a 256 "${VYOS_ISO}" | awk '{print $1}')"
    fi

    # Determine accelerator based on platform
    if [[ "$(uname)" == "Darwin" ]]; then
        ACCELERATOR="hvf"
    else
        ACCELERATOR="kvm"
    fi

    log "Building VyOS image..."
    log "  ISO: ${VYOS_ISO}"
    log "  Output: ${OUTPUT_DIR}"
    log "  SSH Key: ${SSH_KEY_FILE} (${SSH_KEY_TYPE})"
    log "  Accelerator: ${ACCELERATOR}"

    # Run Packer build
    PACKER_LOG=1 packer build \
        -var "vyos_iso_url=file://${VYOS_ISO}" \
        -var "vyos_iso_checksum=${ISO_CHECKSUM}" \
        -var "output_directory=${OUTPUT_DIR}" \
        -var "ssh_key_type=${SSH_KEY_TYPE}" \
        -var "ssh_public_key=${SSH_KEY_BODY}" \
        .

    log "Packer build completed successfully!"
}

show_results() {
    echo ""
    echo "=============================================="
    echo "VyOS Gateway Image Build Complete"
    echo "=============================================="
    echo ""
    echo "Output image: ${OUTPUT_DIR}/vyos-lab.raw"
    echo ""
    echo "Next steps:"
    echo "  1. Copy image to Tinkerbell NAS:"
    echo "     scp ${OUTPUT_DIR}/vyos-lab.raw nas:/volume1/images/vyos-lab.raw"
    echo ""
    echo "  2. Or write directly to USB/SSD for manual install:"
    echo "     sudo dd if=${OUTPUT_DIR}/vyos-lab.raw of=/dev/sdX bs=4M status=progress"
    echo ""
    echo "To update network configuration, edit:"
    echo "  infrastructure/network/vyos/configs/gateway.conf"
    echo ""
}

main() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -i|--iso)
                VYOS_ISO="$2"
                shift 2
                ;;
            -o|--output)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            -k|--ssh-key)
                SSH_KEY_FILE="$2"
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

    log "VyOS Gateway Image Builder"
    log "Repository root: ${REPO_ROOT}"

    check_prerequisites
    download_vyos_iso
    run_packer_build
    show_results
}

main "$@"
