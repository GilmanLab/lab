#!/usr/bin/env bash
set -euo pipefail

# Build + package + (optionally) upload the bootstrap PXE VM image.
#
# This script is intentionally simple:
# - Builds a RAW disk image via Packer.
# - Places it under artifacts/bootstrap/<bootstrap_version>/ (local output; not committed).
# - By default, leaves the image uncompressed (bootstrap-pxe.raw) so tests can run without
#   immediately undoing compression.
# - Optionally compresses (gzip) and/or uploads via the existing scripts/upload-artifacts.py.
#
# Usage:
#   provisioning/bootstrap/build.sh <talos_version> <bootstrap_version> <minipc_mac>
#
# Example:
#   provisioning/bootstrap/build.sh v1.11.6 2025-12-17 00:11:22:33:44:55
#
# Notes:
# - Requires packer + qemu/kvm + OVMF installed on the build machine.
# - Requires Talos netboot artifacts (kernel + initramfs). You can either:
#   - Provide them locally via TALOS_KERNEL_PATH and TALOS_INITRAMFS_PATH, or
#   - Let this script download them from the upstream Talos GitHub release.
# - If you keep your Talos configs SOPS-encrypted under provisioning/bootstrap/config/,
#   this script will decrypt them into build/bootstrap/ and feed the plaintext into Packer:
#     - config/controlplane.yaml (SOPS) -> build/bootstrap/controlplane.yaml (plaintext)
#     - config/talosconfig      (SOPS) -> build/bootstrap/talosconfig      (plaintext)
#   This avoids baking SOPS keys into the guest image.
#
# Optional env vars:
# - COMPRESS=true|false   : gzip the raw disk at the end (default: false)
# - UPLOAD=true|false     : upload artifacts via scripts/upload-artifacts.py (default: false)
#   If UPLOAD=true and COMPRESS is not explicitly set, COMPRESS defaults to true.
#

TALOS_VERSION="${1:?talos_version required (e.g. v1.11.6)}"
BOOTSTRAP_VERSION="${2:?bootstrap_version required (e.g. 2025-12-17)}"
MINIPC_MAC="${3:?minipc_mac required (e.g. 00:11:22:33:44:55)}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

OUT_DIR="${ROOT_DIR}/artifacts/bootstrap/${BOOTSTRAP_VERSION}"
BUILD_BOOTSTRAP_DIR="${ROOT_DIR}/build/bootstrap"
mkdir -p "${BUILD_BOOTSTRAP_DIR}"

talos_tag="v${TALOS_VERSION#v}"
TALOS_KERNEL_DEFAULT="${BUILD_BOOTSTRAP_DIR}/vmlinuz-amd64"
TALOS_INITRAMFS_DEFAULT="${BUILD_BOOTSTRAP_DIR}/initramfs-amd64.xz"
TALOS_KERNEL="${TALOS_KERNEL_PATH:-${TALOS_KERNEL_DEFAULT}}"
TALOS_INITRAMFS="${TALOS_INITRAMFS_PATH:-${TALOS_INITRAMFS_DEFAULT}}"

if [ ! -f "${TALOS_KERNEL}" ] || [ ! -f "${TALOS_INITRAMFS}" ]; then
  echo "Talos netboot artifacts missing; downloading from Talos release ${talos_tag}..." >&2
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' EXIT
  base="https://github.com/siderolabs/talos/releases/download/${talos_tag}"
  curl -fsSL -o "${tmp}/vmlinuz-amd64" "${base}/vmlinuz-amd64"
  curl -fsSL -o "${tmp}/initramfs-amd64.xz" "${base}/initramfs-amd64.xz"
  install -m 0644 "${tmp}/vmlinuz-amd64" "${TALOS_KERNEL}"
  install -m 0644 "${tmp}/initramfs-amd64.xz" "${TALOS_INITRAMFS}"
fi

mkdir -p "${OUT_DIR}"

cd "${ROOT_DIR}"

packer init provisioning/bootstrap/packer/bootstrap.pkr.hcl

PLAIN_CONTROLPLANE="${BUILD_BOOTSTRAP_DIR}/controlplane.yaml"
PLAIN_TALOSCONFIG="${BUILD_BOOTSTRAP_DIR}/talosconfig"

SOPS_CONTROLPLANE_DEFAULT="${ROOT_DIR}/provisioning/bootstrap/config/controlplane.yaml"
SOPS_TALOSCONFIG_DEFAULT="${ROOT_DIR}/provisioning/bootstrap/config/talosconfig"

SOPS_CONTROLPLANE="${SOPS_CONTROLPLANE_PATH:-${SOPS_CONTROLPLANE_DEFAULT}}"
SOPS_TALOSCONFIG="${SOPS_TALOSCONFIG_PATH:-${SOPS_TALOSCONFIG_DEFAULT}}"

mkdir -p "${BUILD_BOOTSTRAP_DIR}"

if [ -f "${SOPS_CONTROLPLANE}" ]; then
  if command -v sops >/dev/null 2>&1; then
    echo "Decrypting SOPS controlplane config -> ${PLAIN_CONTROLPLANE}"
    sops -d "${SOPS_CONTROLPLANE}" > "${PLAIN_CONTROLPLANE}"
    chmod 0600 "${PLAIN_CONTROLPLANE}"
  else
    echo "sops not found but ${SOPS_CONTROLPLANE} exists; install sops or provide MACHINECONFIG_PATH=/path/to/plaintext.yaml" >&2
    exit 1
  fi
fi

if [ -f "${SOPS_TALOSCONFIG}" ]; then
  if command -v sops >/dev/null 2>&1; then
    echo "Decrypting SOPS talosconfig -> ${PLAIN_TALOSCONFIG}"
    sops -d "${SOPS_TALOSCONFIG}" > "${PLAIN_TALOSCONFIG}"
    chmod 0600 "${PLAIN_TALOSCONFIG}"
  else
    echo "sops not found but ${SOPS_TALOSCONFIG} exists; install sops (or skip talosconfig serving)" >&2
    exit 1
  fi
fi

MACHINECONFIG_PATH="${MACHINECONFIG_PATH:-${PLAIN_CONTROLPLANE}}"
TALOSCONFIG_PATH="${TALOSCONFIG_PATH:-${PLAIN_TALOSCONFIG}}"

if [ ! -f "${MACHINECONFIG_PATH}" ]; then
  echo "Machineconfig not found. Provide one of:" >&2
  echo "  - SOPS_CONTROLPLANE_PATH=/path/to/controlplane.yaml (encrypted) + sops installed" >&2
  echo "  - MACHINECONFIG_PATH=/path/to/plaintext-controlplane.yaml" >&2
  exit 1
fi

packer build \
  -var "talos_version=${TALOS_VERSION}" \
  -var "talos_kernel_path=${TALOS_KERNEL}" \
  -var "talos_initramfs_path=${TALOS_INITRAMFS}" \
  -var "minipc_mac=${MINIPC_MAC}" \
  -var "output_dir=${OUT_DIR}" \
  -var "machineconfig_path=${MACHINECONFIG_PATH}" \
  -var "talosconfig_path=${TALOSCONFIG_PATH}" \
  provisioning/bootstrap/packer/bootstrap.pkr.hcl

RAW="${OUT_DIR}/bootstrap-pxe.raw"
if [ ! -f "${RAW}" ]; then
  # fallback to default vm_name if changed
  RAW="$(ls -t "${OUT_DIR}"/*.raw | head -n1 || true)"
fi

if [ -z "${RAW}" ] || [ ! -f "${RAW}" ]; then
  echo "Unable to locate built RAW image under: ${OUT_DIR}" >&2
  ls -lah "${OUT_DIR}" >&2
  exit 1
fi

COMPRESS="${COMPRESS:-}"
if [ -z "${COMPRESS}" ]; then
  if [ "${UPLOAD:-false}" = "true" ]; then
    COMPRESS="true"
  else
    COMPRESS="false"
  fi
fi

if [ "${COMPRESS}" = "true" ]; then
  echo "Compressing ${RAW}..."
  gzip -f -9 "${RAW}"
  echo "Compressed artifact:"
  ls -lah "${RAW}.gz"
else
  echo "Skipping compression (set COMPRESS=true to gzip for publishing)."
  echo "Built artifact:"
  ls -lah "${RAW}"
fi

if [ "${UPLOAD:-false}" = "true" ]; then
  if [ "${COMPRESS}" != "true" ]; then
    echo "UPLOAD=true requires COMPRESS=true (set COMPRESS=true or leave it unset)." >&2
    exit 1
  fi
  echo "Uploading to iDrive e2 via scripts/upload-artifacts.py..."
  cd "${ROOT_DIR}"
  ./scripts/upload-artifacts.py bootstrap "${BOOTSTRAP_VERSION}"
fi


