## Bootstrap appliance integration test (libvirt)

This is a **pre-ship** integration test that boots:

- the **bootstrap PXE appliance** (your built RAW disk), then
- a **PXE client VM** on the same isolated L2 network,

and asserts Talos installs + becomes healthy.

### Prerequisites (host)

- **libvirt** running (and your user can run `virsh`)
- `virt-install` (from `virt-manager` / `virt-install` package)
- `qemu-img`
- `curl`
- `talosctl`
- UEFI firmware available to libvirt/QEMU (virt-install `--boot uefi`)

### Artifact expected

By default, the test uses:

- `artifacts/bootstrap/local-test/bootstrap-pxe.raw`

Override with `IT_BOOTSTRAP_DISK`.

### Configuration (env vars)

- **IT_BOOTSTRAP_DISK**: path to the appliance raw disk
- **IT_BOOTSTRAP_VERSION**: build/output directory version under `artifacts/bootstrap/<version>/` (default: `local-test`)
- **IT_TALOS_VERSION**: Talos version string used in the built appliance’s HTTP paths (default: `v1.11.6`)
- **IT_BOOTSTRAP_INTERNAL_MAC**: MAC for the appliance’s *PXE/internal* NIC (default: read from `provisioning/bootstrap/packer/bootstrap.pkr.hcl` `bootstrap_internal_mac`)
- **IT_BOOTSTRAP_UPLINK_MAC**: MAC for the appliance’s *uplink* NIC (default: read from `provisioning/bootstrap/packer/bootstrap.pkr.hcl` `bootstrap_uplink_mac`)
- **IT_MINIPC_MAC**: MAC address that the appliance’s dnsmasq is pinned to (default: read from `provisioning/bootstrap/packer/bootstrap.pkr.hcl` `minipc_mac`)
- **IT_MINIPC_IP**: expected Talos node IP (default: read from `provisioning/bootstrap/packer/bootstrap.pkr.hcl` `minipc_ip`)
- **IT_BOOTSTRAP_IP**: expected appliance IP (default: read from `provisioning/bootstrap/packer/bootstrap.pkr.hcl` `vm_ip`)
- **IT_TALOSCONFIG_PATH**: path to a local plaintext `talosconfig` (optional; otherwise fetched from `http://10.10.10.2/configs/talosconfig`)
- **IT_TIMEOUT_S**: overall timeout in seconds (default: `900`)
- **IT_GUI**: if `true`, launch VMs with a graphical console (SPICE) instead of headless (default: `false`)
- **IT_KEEP_ON_FAIL**: if `true`, keep the libvirt network + VMs around on failure for debugging (default: `true` when `IT_GUI=true`)
- **IT_KEEP**: if `true`, keep the libvirt network + VMs around even on success (default: `false`)

### Networking note (why an uplink is needed)

Talos will pull container images during startup (e.g. Kubernetes components). The test network is isolated for safety, so the **bootstrap appliance** is launched with:

- a **PXE/internal NIC** on the isolated libvirt network (runs DHCP/TFTP/HTTP), and
- an **uplink NIC** on libvirt’s `default` network (DHCP), and the appliance **NATs** PXE traffic out via the uplink.

This keeps DHCP scoped to the isolated NIC (dnsmasq is `bind-interfaces` on that interface only) while still allowing outbound internet for image pulls.

### Run

From repo root:

```bash
cd /home/josh/code/lab/provisioning/bootstrap/integration

# Install python deps (pytest) via uv
uv sync --group dev

# One-shot build + test:
#   cd /home/josh/code/lab/provisioning/bootstrap
#   just test

# If you built the appliance with a non-default pinned MAC, set it here:
# export IT_MINIPC_MAC="00:11:22:33:44:55"

uv run pytest -m integration -q
```

### What it checks

- Appliance boots and serves:
  - `/boot.ipxe`
  - `/configs/minipc.yaml`
  - `/talos/<version>/{vmlinuz,initramfs.xz}`
- PXE client VM boots, installs Talos, and exposes Talos API on `:50000`
- `talosctl health` succeeds

### Debugging with a GUI

If you have a desktop session on this machine, you can run:

```bash
export IT_GUI=true
export IT_KEEP_ON_FAIL=true
```

Then start the test. If it stalls or fails, open **virt-manager**, connect to `qemu:///system`,
and open the consoles for the `bootstrap-appliance-*` and `bootstrap-client-*` domains.


