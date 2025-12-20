#!/bin/vbash
# VyOS Provisioning Script
# Loads gateway.conf and configures SSH key
#
# Arguments:
#   $1 - SSH key type (e.g., ssh-rsa, ssh-ed25519, ecdsa-sha2-nistp256)
#   $2 - SSH public key (base64 encoded key body)

set -e

SSH_KEY_TYPE="$1"
SSH_KEY="$2"

# Config file location (copied by Packer)
CONFIG_FILE="/tmp/gateway.conf"

# Source VyOS environment
source /opt/vyatta/etc/functions/script-template

echo "=== VyOS Lab Gateway Provisioning ==="

# =============================================================================
# Validate Required Arguments
# =============================================================================
if [ -z "${SSH_KEY_TYPE}" ] || [ -z "${SSH_KEY}" ]; then
    echo "ERROR: SSH key type and key are required"
    echo "Usage: provision.sh <key_type> <key_body>"
    echo "Example: provision.sh ssh-ed25519 AAAAC3Nz..."
    exit 1
fi

if [ ! -f "${CONFIG_FILE}" ]; then
    echo "ERROR: Configuration file not found: ${CONFIG_FILE}"
    exit 1
fi

echo "SSH Key Type: ${SSH_KEY_TYPE}"

# =============================================================================
# Load Configuration
# =============================================================================
echo "Loading configuration from gateway.conf..."

configure

# Load the base configuration file (source of truth)
load "${CONFIG_FILE}"

# =============================================================================
# Configure SSH Key
# =============================================================================
echo "Configuring SSH authentication..."

set system login user vyos authentication public-keys admin type "${SSH_KEY_TYPE}"
set system login user vyos authentication public-keys admin key "${SSH_KEY}"

# =============================================================================
# Commit and Save
# =============================================================================
echo "Committing configuration..."
commit

echo "Saving configuration..."
save

exit

echo ""
echo "=== VyOS Lab Gateway Provisioning Complete ==="
