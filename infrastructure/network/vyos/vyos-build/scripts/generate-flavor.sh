#!/bin/bash
# Generate VyOS build flavor with SSH credentials from SOPS secrets
#
# Usage:
#   ./generate-flavor.sh <ssh_public_key> <output_file>
#
# Arguments:
#   ssh_public_key - Full SSH public key (e.g., "ssh-ed25519 AAAAC3Nz... comment")
#   output_file    - Path to write the generated flavor TOML
#
# Example:
#   ./generate-flavor.sh "$(sops -d --extract '["ssh_public_key"]' images/packer-ssh.sops.yaml)" gateway-final.toml

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_FILE="${SCRIPT_DIR}/../build-flavors/gateway.toml"

usage() {
    echo "Usage: $0 <ssh_public_key> <output_file>"
    echo ""
    echo "Arguments:"
    echo "  ssh_public_key - Full SSH public key string"
    echo "  output_file    - Path for generated flavor TOML"
    exit 1
}

if [[ $# -ne 2 ]]; then
    usage
fi

SSH_PUBLIC_KEY="$1"
OUTPUT_FILE="$2"

# Validate inputs
if [[ -z "${SSH_PUBLIC_KEY}" ]]; then
    echo "ERROR: SSH public key is required"
    exit 1
fi

if [[ ! -f "${TEMPLATE_FILE}" ]]; then
    echo "ERROR: Template file not found: ${TEMPLATE_FILE}"
    exit 1
fi

# Parse SSH public key: "type key comment" -> extract type and key
SSH_KEY_TYPE=$(echo "${SSH_PUBLIC_KEY}" | awk '{print $1}')
SSH_KEY_BODY=$(echo "${SSH_PUBLIC_KEY}" | awk '{print $2}')

if [[ -z "${SSH_KEY_TYPE}" ]] || [[ -z "${SSH_KEY_BODY}" ]]; then
    echo "ERROR: Could not parse SSH public key"
    echo "Expected format: 'type key [comment]'"
    echo "Got: '${SSH_PUBLIC_KEY}'"
    exit 1
fi

echo "=== Generating VyOS Build Flavor ==="
echo "SSH Key Type: ${SSH_KEY_TYPE}"
echo "SSH Key Length: ${#SSH_KEY_BODY} characters"
echo "Output: ${OUTPUT_FILE}"

# Generate the final flavor by replacing placeholders
sed \
    -e "s|%%SSH_KEY_TYPE%%|${SSH_KEY_TYPE}|g" \
    -e "s|%%SSH_PUBLIC_KEY%%|${SSH_KEY_BODY}|g" \
    "${TEMPLATE_FILE}" > "${OUTPUT_FILE}"

echo "=== Flavor generated successfully ==="
