# Packer Build: Bootstrap Appliance

This directory contains the Packer template (`bootstrap.pkr.hcl`) used to build the **Bootstrap PXE Appliance** VM image.

The build process creates a headless Ubuntu 24.04 Server VM, installs necessary services (DNS/DHCP/HTTP), and embeds Talos Linux netboot artifacts.

## Overview

- **Builder**: `qemu` (KVM acceleration required)
- **Base Image**: Ubuntu 24.04.3 Live Server ISO
- **Output Format**: RAW disk image (`bootrap-pxe.raw`)
- **Default Credentials**: `packer` / `packer` (if SSH access is needed during debug)

## Build Process

The build is orchestrated by the root `build.sh` script, but `packer` can be run manually if variables are provided.

1. **Boot**: Boots the Ubuntu ISO and performs an autoinstall (user-data via `http` server).
2. **Provision**:
   - Installs `dnsmasq`, `nginx`, `ipxe`, `iptables`.
   - Copies Talos `vmlinuz`, `initramfs`, `machineconfig`, and `talosconfig` from the host.
   - Configures `netplan` for a static IP (`192.168.2.1`) on the internal interface.
   - Installs a custom NAT service (`bootstrap-nat`) to route traffic from the internal network to the WAN.
3. **Artifact**: Exports the VM disk as a raw image.

## Key Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `vm_name` | `bootstrap-pxe` | Name of the output image. |
| `vm_ip` | `192.168.2.1` | Static IP of the appliance on the PXE network. |
| `minipc_mac` | `02:11:32:24:64:5b` | MAC address of the target machine (DHCP reservation). |
| `minipc_ip` | `192.168.2.2` | IP assigned to the target machine. |
| `talos_version` | `1.11.6` | Version of Talos artifacts to embed. |
| `bootstrap_internal_mac` | `02:11:32:24:64:5a` | MAC address for the appliance's PXE interface. |
| `bootstrap_uplink_mac` | `02:11:32:24:64:5c` | MAC address for the appliance's uplink (NAT) interface. |

## Files

- **`bootstrap.pkr.hcl`**: Main Packer template.
- **`http/`**: Contains `user-data` and `meta-data` for Ubuntu autoinstall.
- **`scripts/provision-bootstrap.sh`**: Main provisioning script ran inside the VM.
