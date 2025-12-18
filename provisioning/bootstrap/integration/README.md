# Bootstrap Integration Tests (Libvirt)

This directory contains an integration test suite that verifies the **Bootstrap PXE Appliance** works correctly in a secluded environment.

The test performs the following:
1. Boots the **Bootstrap Appliance** on an isolated Libvirt network.
2. Boots a **PXE Client VM** on the same network.
3. Verifies that the client:
    - Receives an IP via DHCP.
    - Boots via iPXE.
    - Installs Talos Linux.
    - Becomes healthy and reachable via the Talos API.

## Prerequisites

To run these tests, the host machine requires:

- **Libvirt**: Running and accessible (`virsh`).
- **QEMU / KVM**: Hypervisor.
- **Virt-Install**: Needed to spawn VMs (`virt-install`).
- **QEMU-IMG**: Needed to create client disks (`qemu-img`).
- **UV**: Python package manager (used to run the test suite).
- **Talosctl**: CLI tool to verify Talos health.
- **Curl**: Used for connectivity checks.
- **UEFI Firmware**: Required by QEMU (e.g., `ovmf` or `edk2-ovmf`).

## Configuration

The tests are configured via environment variables. Defaults are derived from `provisioning/bootstrap/packer/bootstrap.pkr.hcl` where possible.

### General
- **`IT_BOOTSTRAP_DISK`**: Path to the appliance raw disk image. Defaults to `artifacts/bootstrap/<IT_BOOTSTRAP_VERSION>/bootstrap-pxe.raw`.
- **`IT_BOOTSTRAP_VERSION`**: Version subdirectory to look for artifacts in (default: `local-test`).
- **`IT_TALOS_VERSION`**: Talos version string used in HTTP paths (default: `v1.11.6`).
- **`IT_TIMEOUT_S`**: Test timeout in seconds (default: `900`).

### Networking
- **`IT_BOOTSTRAP_IP`**: Expected IP of the appliance (default: `192.168.2.1`).
- **`IT_MINIPC_IP`**: Expected IP of the client node (default: `192.168.2.2`).
- **`IT_BOOTSTRAP_INTERNAL_MAC`**: MAC for the appliance's internal/PXE NIC (default: `02:11:32:24:64:5a`).
- **`IT_BOOTSTRAP_UPLINK_MAC`**: MAC for the appliance's uplink/NAT NIC (default: `02:11:32:24:64:5c`).
- **`IT_MINIPC_MAC`**: MAC for the client VM (default: `02:11:32:24:64:5b`).

### Libvirt
- **`IT_LIBVIRT_URI`**: Libvirt connection URI (default: `qemu:///system`).
- **`IT_LIBVIRT_POOL`**: Storage pool for volumes (default: `default`).
- **`IT_SUDO`**: If `true`, run `virsh` and `virt-install` with `sudo -n`. Useful for CI runners.

### Debugging
- **`IT_GUI`**: If `true`, launch VMs with SPICE graphics instead of headless (default: `false`).
- **`IT_KEEP_ON_FAIL`**: If `true`, keep Libvirt resources (VMs, networks) if the test fails (default: `true` if GUI is enabled, else `false`).
- **`IT_KEEP`**: If `true`, keep Libvirt resources even on success (default: `false`).
- **`IT_TALOSCONFIG_PATH`**: Path to a local `talosconfig`. If not set, it is fetched from the appliance.

## Usage

### Run via Just (Recommended)

The easiest way to run the tests is via the `justfile` in the parent directory.

**Common Permutations:**

```bash
# 1. Standard run (Headless, cleans up everything on exit)
just test

# 2. Debugging (GUI enabled, keeps artifacts ONLY on failure)
#    Note: gui=true automatically enables keep_on_fail=true
just test gui=true

# 3. Manual Inspection (Headless, keeps artifacts even on success)
just test keep=true

# 4. Full Manual (GUI enabled, keeps artifacts always)
just test gui=true keep=true
```

### Run via UV (Manual)

You can also run pytest directly using `uv`:

```bash
cd provisioning/bootstrap/integration
uv sync --group dev
uv run pytest -m integration -v
```

## Debugging

If a test fails and `IT_KEEP_ON_FAIL` is active, the Libvirt network and VMs will remain running.

1. **Connect**: Open `virt-manager` and connect to the session (usually `qemu:///system`).
2. **Inspect**: You should see `bootstrap-appliance-*` and `bootstrap-client-*`.
3. **Logs**: Check the console output of the VMs.

### Cleanup

To clean up resources left behind by a debug run:

```bash
cd ..
just cleanup
```


