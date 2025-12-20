# VyOS Gateway Image Build

This directory contains the configuration and scripts for building custom VyOS gateway images using the official `vyos-build` toolchain.

## Overview

This approach:

1. Uses the official `vyos/vyos-build` Docker container
2. Bakes the gateway configuration directly into the image via build flavors
3. Produces a raw disk image suitable for Tinkerbell/NAS deployment
4. Injects SSH credentials from SOPS secrets at build time

## Directory Structure

```
vyos-build/
├── build-flavors/
│   └── gateway.toml      # Build flavor template with config.boot
├── scripts/
│   └── generate-flavor.sh # Injects SSH credentials into flavor
└── README.md
```

## Build Process

The GitHub Actions workflow (`.github/workflows/vyos-build.yml`) handles the full build:

1. Decrypts SSH public key from `images/packer-ssh.sops.yaml`
2. Generates the final flavor TOML with credentials injected
3. Clones `vyos-build` repository
4. Runs the build in the `vyos/vyos-build:current` container
5. Uploads the resulting image to iDrive e2

### Local Build (for testing)

```bash
# 1. Clone vyos-build
git clone -b current --single-branch https://github.com/vyos/vyos-build.git /tmp/vyos-build

# 2. Generate flavor with SSH key
./scripts/generate-flavor.sh "ssh-ed25519 AAAA..." /tmp/vyos-build/data/build-flavors/gateway.toml

# 3. Run build in container
docker run --rm -it --privileged \
  -v /tmp/vyos-build:/vyos \
  -v /dev:/dev \
  vyos/vyos-build:current bash

# Inside container:
cd /vyos
sudo ./build-vyos-image --architecture amd64 --build-by "local@test" gateway

# Output: /vyos/build/vyos-*.raw
```

## Configuration

The `gateway.toml` flavor file contains:

- **`image_format = "raw"`**: Output format for Tinkerbell deployment
- **`disk_size = 8`**: 8GB disk image
- **`default_config`**: Full VyOS configuration embedded in the image

The configuration matches `infrastructure/network/vyos/configs/gateway.conf` with SSH credentials added via placeholders:
- `%%SSH_KEY_TYPE%%` - SSH key type (e.g., `ssh-ed25519`)
- `%%SSH_PUBLIC_KEY%%` - SSH public key body

## Relationship to Other Files

| File | Purpose |
|------|---------|
| `configs/gateway.conf` | Source of truth for VyOS config (Ansible applies updates) |
| `vyos-build/build-flavors/gateway.toml` | Build-time config with SSH credentials |
