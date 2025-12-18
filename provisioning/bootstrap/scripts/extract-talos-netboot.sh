#!/usr/bin/env bash
set -euo pipefail

ISO_PATH="${1:?iso path required}"
OUT_DIR="${2:?output dir required}"

mkdir -p "${OUT_DIR}"

if ! command -v bsdtar >/dev/null 2>&1; then
  echo "bsdtar not found; install libarchive-tools" >&2
  exit 1
fi

list="$(bsdtar -tf "${ISO_PATH}")"

VMLINUX_PATH="$(echo "${list}" | grep -E '(^|/)vmlinuz$' | head -n1 || true)"
INITRD_PATH="$(echo "${list}" | grep -E '(^|/)(initramfs\\.xz|initramfs\\.img)$' | head -n1 || true)"

if [ -z "${VMLINUX_PATH}" ] || [ -z "${INITRD_PATH}" ]; then
  echo "Unable to locate vmlinuz/initramfs in ISO: ${ISO_PATH}" >&2
  echo "Top-level ISO listing (first 200 lines):" >&2
  echo "${list}" | head -n 200 >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

bsdtar -xf "${ISO_PATH}" -C "${tmp}" "${VMLINUX_PATH}" "${INITRD_PATH}"

install -m 0644 "${tmp}/${VMLINUX_PATH}" "${OUT_DIR}/vmlinuz"
install -m 0644 "${tmp}/${INITRD_PATH}" "${OUT_DIR}/initramfs.xz"

echo "Extracted:"
ls -lah "${OUT_DIR}/vmlinuz" "${OUT_DIR}/initramfs.xz"


