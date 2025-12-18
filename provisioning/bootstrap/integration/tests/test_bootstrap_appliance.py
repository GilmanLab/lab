import os
import ipaddress
import re
import shutil
import socket
import subprocess
import tempfile
import textwrap
import time
from pathlib import Path
from uuid import uuid4

import pytest


def _run(cmd: list[str], *, check: bool = True, capture: bool = True, env: dict | None = None) -> subprocess.CompletedProcess:
    kwargs = {
        "check": check,
        "text": True,
        "env": {**os.environ, **(env or {})},
    }
    if capture:
        kwargs["stdout"] = subprocess.PIPE
        kwargs["stderr"] = subprocess.STDOUT
    return subprocess.run(cmd, **kwargs)


def _require_bin(name: str) -> None:
    if shutil.which(name) is None:
        pytest.skip(f"Missing required binary on PATH: {name}")


def _wait_until(deadline: float, *, interval_s: float, what: str, fn):
    last_exc = None
    start = time.time()
    last_log = 0.0
    while time.time() < deadline:
        try:
            if fn():
                return
        except Exception as e:  # noqa: BLE001 - used for retry loop diagnostics
            last_exc = e
        now = time.time()
        # Emit periodic progress so long-running waits aren't silent under -s.
        if now - last_log >= 15:
            remaining = max(0, int(deadline - now))
            elapsed = int(now - start)
            print(f"[wait] {what} (elapsed={elapsed}s remaining~{remaining}s)", flush=True)
            last_log = now
        time.sleep(interval_s)
    if last_exc:
        raise AssertionError(f"Timed out waiting for: {what}. Last error: {last_exc}") from last_exc
    raise AssertionError(f"Timed out waiting for: {what}")


def _libvirt_uri() -> str:
    # Prefer system libvirt (needed for bridge-based isolated networks),
    # but allow override.
    return os.environ.get("IT_LIBVIRT_URI", "qemu:///system")


def _virsh(*args: str, check: bool = True) -> subprocess.CompletedProcess:
    return _run(["virsh", "-c", _libvirt_uri(), *args], check=check, capture=True)


def _virsh_exists(kind: str, name: str) -> bool:
    # kind: "dom" or "net"
    if kind == "dom":
        cp = _virsh("dominfo", name, check=False)
        return cp.returncode == 0
    if kind == "net":
        cp = _virsh("net-info", name, check=False)
        return cp.returncode == 0
    raise ValueError(f"unknown kind: {kind}")


def _destroy_domain(name: str) -> None:
    if not _virsh_exists("dom", name):
        return
    _virsh("destroy", name, check=False)
    # uefi nvram cleanup when present
    _virsh("undefine", name, "--nvram", check=False)
    _virsh("undefine", name, check=False)


def _destroy_network(name: str) -> None:
    if not _virsh_exists("net", name):
        return
    _virsh("net-destroy", name, check=False)
    _virsh("net-undefine", name, check=False)


def _write_file(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def _curl_ok(url: str) -> bool:
    # Use -o /dev/null so we don't try to decode binary payloads (e.g. vmlinuz).
    cp = _run(["curl", "-fsS", "--max-time", "2", "-o", "/dev/null", url], check=False, capture=True)
    return cp.returncode == 0


def _virsh_stdout(*args: str) -> str:
    cp = _virsh(*args, check=True)
    return (cp.stdout or "").strip()


def _debug_dump(*, net_name: str, appliance_name: str, client_name: str) -> None:
    """
    Best-effort debug output to make failures actionable.
    Uses only virsh queries (no interactive console).
    """
    def dump(cmd: list[str]) -> None:
        cp = _run(cmd, check=False, capture=True)
        print(f"\n[debug] $ {' '.join(cmd)}\n{cp.stdout}", flush=True)

    uri = _libvirt_uri()
    dump(["virsh", "-c", uri, "net-info", net_name])
    dump(["virsh", "-c", uri, "net-dumpxml", net_name])
    dump(["virsh", "-c", uri, "domstate", appliance_name])
    dump(["virsh", "-c", uri, "domiflist", appliance_name])
    dump(["virsh", "-c", uri, "domblklist", appliance_name])
    dump(["virsh", "-c", uri, "domstate", client_name])
    dump(["virsh", "-c", uri, "domiflist", client_name])
    dump(["virsh", "-c", uri, "domblklist", client_name])


def _ensure_libvirt_volume_from_file(*, pool: str, src_path: Path, vol_name: str) -> str:
    """
    Copy src_path into a libvirt storage pool volume and return the volume's path.
    This is mainly to avoid system-qemu access/SELinux issues when the source file
    lives under /home.
    """
    # Ensure pool exists/active
    cp = _virsh("pool-info", pool, check=False)
    if cp.returncode != 0:
        raise RuntimeError(f"libvirt pool not available: {pool}\n{cp.stdout}")

    refresh = _env_bool("IT_LIBVIRT_REFRESH_VOLUME", False)

    # If volume doesn't exist, create it.
    cp = _virsh("vol-info", vol_name, pool, check=False)
    if cp.returncode != 0:
        _virsh("vol-create-as", pool, vol_name, str(src_path.stat().st_size), "--format", "raw", check=True)
        refresh = True

    # Upload if requested (or if we just created the volume).
    if refresh:
        print(f"[info] Refreshing libvirt volume {pool}/{vol_name} from {src_path}", flush=True)
        _virsh("vol-upload", vol_name, str(src_path), pool, "--sparse", check=True)

    vol_path = _virsh_stdout("vol-path", vol_name, pool)
    if not vol_path:
        raise RuntimeError(f"Unable to resolve volume path for {pool}/{vol_name}")
    return vol_path


def _delete_libvirt_volume(*, pool: str, vol_name: str) -> None:
    _virsh("vol-delete", vol_name, pool, check=False)


def _create_empty_volume(*, pool: str, vol_name: str, capacity_bytes: int, fmt: str) -> str:
    cp = _virsh("vol-info", vol_name, pool, check=False)
    if cp.returncode != 0:
        _virsh("vol-create-as", pool, vol_name, str(capacity_bytes), "--format", fmt, check=True)
    vol_path = _virsh_stdout("vol-path", vol_name, pool)
    if not vol_path:
        raise RuntimeError(f"Unable to resolve volume path for {pool}/{vol_name}")
    return vol_path


def _env_bool(name: str, default: bool) -> bool:
    v = os.environ.get(name)
    if v is None:
        return default
    return v.strip().lower() in {"1", "true", "yes", "y", "on"}


def _virt_install_console_args() -> list[str]:
    """
    Control whether virt-install tries to open a GUI console.

    - IT_GUI=true: launch with SPICE graphics and allow autoconsole
    - otherwise: headless
    """
    if _env_bool("IT_GUI", False):
        # Note: requires a desktop session on the host running virt-install.
        return ["--graphics", "spice"]
    return ["--graphics", "none", "--noautoconsole"]


def _keep_artifacts_on_failure() -> bool:
    # If GUI is enabled, default to keeping things around so the user can inspect.
    return _env_bool("IT_KEEP_ON_FAIL", _env_bool("IT_GUI", False))


def _keep_artifacts_always() -> bool:
    # Keep artifacts even on success (explicit opt-in for debugging / manual validation).
    return _env_bool("IT_KEEP", False)


def _ensure_raw_disk(disk_path: Path) -> Path:
    """
    Ensure we have a plaintext .raw disk image to hand to libvirt.

    If only <disk>.gz exists (as produced by build.sh), we gunzip -k it to <disk>.
    """
    if disk_path.exists():
        return disk_path

    gz = Path(str(disk_path) + ".gz")
    if gz.exists():
        _require_bin("gunzip")
        print(f"[info] Decompressing {gz} -> {disk_path}", flush=True)
        _run(["gunzip", "-k", "-f", str(gz)], check=True, capture=True)
        return disk_path

    return disk_path


def _default_minipc_mac_from_packer(repo_root: Path) -> str | None:
    return _default_var_from_packer(repo_root, "minipc_mac")


def _default_bootstrap_internal_mac_from_packer(repo_root: Path) -> str | None:
    return _default_var_from_packer(repo_root, "bootstrap_internal_mac")


def _default_bootstrap_uplink_mac_from_packer(repo_root: Path) -> str | None:
    return _default_var_from_packer(repo_root, "bootstrap_uplink_mac")


def _default_var_from_packer(repo_root: Path, var_name: str) -> str | None:
    """
    Derive a default from the packer template so the integration test stays in sync with:
      variable "<var_name>" { default = "..." }
    """
    pkr = repo_root / "provisioning/bootstrap/packer/bootstrap.pkr.hcl"
    if not pkr.exists():
        return None

    content = pkr.read_text(encoding="utf-8")
    m = re.search(
        rf'variable\s+"{re.escape(var_name)}"\s*\{{[\s\S]*?default\s*=\s*"([^"]+)"',
        content,
        re.MULTILINE,
    )
    if not m:
        return None
    return m.group(1).strip()


def _default_int_var_from_packer(repo_root: Path, var_name: str) -> int | None:
    pkr = repo_root / "provisioning/bootstrap/packer/bootstrap.pkr.hcl"
    if not pkr.exists():
        return None

    content = pkr.read_text(encoding="utf-8")
    m = re.search(
        rf'variable\s+"{re.escape(var_name)}"\s*\{{[\s\S]*?default\s*=\s*([0-9]+)\s*',
        content,
        re.MULTILINE,
    )
    if not m:
        return None
    return int(m.group(1))


def _pick_host_ip(network: ipaddress.IPv4Network, *, avoid: set[ipaddress.IPv4Address]) -> ipaddress.IPv4Address:
    # Prefer .254 on /24-like networks, but fall back safely.
    candidates = []
    for off in (254, 253, 250, 10, 1):
        try:
            candidates.append(network.network_address + off)
        except Exception:
            continue
    for ip in candidates:
        if ip in network and ip != network.network_address and ip != network.broadcast_address and ip not in avoid:
            return ip
    for ip in network.hosts():
        if ip not in avoid:
            return ip
    raise ValueError(f"No usable host IPs in network {network}")


def _tcp_port_open(host: str, port: int, timeout_s: float) -> bool:
    try:
        with socket.create_connection((host, port), timeout=timeout_s):
            return True
    except OSError:
        return False


@pytest.mark.integration
def test_appliance_pxe_installs_talos():
    """
    End-to-end smoke test:

    - Boot the bootstrap appliance RAW disk on an isolated libvirt network
    - Wait for it to serve PXE endpoints (HTTP)
    - Boot a second VM that PXE boots (UEFI) with a MAC matching the appliance's dnsmasq pin
    - Wait for Talos to come up and report healthy via talosctl
    """
    _require_bin("virsh")
    _require_bin("virt-install")
    _require_bin("curl")
    _require_bin("qemu-img")
    _require_bin("talosctl")

    # Ensure libvirt is reachable
    cp = _virsh("uri", check=False)
    if cp.returncode != 0:
        pytest.skip(f"libvirt not available to current user (virsh uri failed):\n{cp.stdout}")

    repo_root = Path(__file__).resolve().parents[4]

    talos_version = os.environ.get("IT_TALOS_VERSION", "v1.11.6")

    packer_vm_ip = _default_var_from_packer(repo_root, "vm_ip") or "192.168.2.1"
    packer_minipc_ip = _default_var_from_packer(repo_root, "minipc_ip") or "192.168.2.2"
    packer_vm_prefix = _default_int_var_from_packer(repo_root, "vm_prefix") or 24

    bootstrap_ip = os.environ.get("IT_BOOTSTRAP_IP", packer_vm_ip)
    bootstrap_http = f"http://{bootstrap_ip}"

    # Appliance NIC MACs (must match what the guest netplan expects).
    bootstrap_internal_mac = (
        os.environ.get("IT_BOOTSTRAP_INTERNAL_MAC")
        or _default_bootstrap_internal_mac_from_packer(repo_root)
        or "02:11:32:24:64:5a"
    ).lower()
    bootstrap_uplink_mac = (
        os.environ.get("IT_BOOTSTRAP_UPLINK_MAC")
        or _default_bootstrap_uplink_mac_from_packer(repo_root)
        or "02:11:32:24:64:5c"
    ).lower()

    # Must match the MAC used when building the appliance (packer var minipc_mac).
    # Default matches the packer template default, but your real build may override.
    packer_default_mac = _default_minipc_mac_from_packer(repo_root) or "02:11:32:24:64:5b"
    minipc_mac = os.environ.get("IT_MINIPC_MAC", packer_default_mac).lower()
    minipc_ip = os.environ.get("IT_MINIPC_IP", packer_minipc_ip)

    # Locate appliance disk (built out-of-band, e.g. via `just build`)
    bootstrap_version = os.environ.get("IT_BOOTSTRAP_VERSION", "local-test")
    default_disk = repo_root / "artifacts/bootstrap" / bootstrap_version / "bootstrap-pxe.raw"
    appliance_disk_env = os.environ.get("IT_BOOTSTRAP_DISK")
    appliance_disk = Path(appliance_disk_env) if appliance_disk_env else default_disk
    appliance_disk = _ensure_raw_disk(appliance_disk)

    if not appliance_disk.exists():
        pytest.skip(
            f"Bootstrap appliance disk not found: {appliance_disk}\n"
            f"(run `just build` first, or set IT_BOOTSTRAP_DISK to point at an existing image)"
        )

    # libvirt isolated network that still gives the host an IP (virbrX).
    # Default to the same subnet as packer vm_ip/vm_prefix, but avoid collisions.
    try:
        net = ipaddress.IPv4Network(f"{packer_vm_ip}/{packer_vm_prefix}", strict=False)
        avoid = {ipaddress.IPv4Address(packer_vm_ip), ipaddress.IPv4Address(packer_minipc_ip)}
        default_net_ip = str(_pick_host_ip(net, avoid=avoid))
    except Exception:
        default_net_ip = "192.168.2.254"

    net_ip = os.environ.get("IT_NET_HOST_IP", default_net_ip)
    net_mask = os.environ.get("IT_NET_MASK", "255.255.255.0")

    timeout_s = int(os.environ.get("IT_TIMEOUT_S", "900"))
    deadline = time.time() + timeout_s
    keep_on_fail = _keep_artifacts_on_failure()
    keep_always = _keep_artifacts_always()
    gui = _env_bool("IT_GUI", False)
    if gui:
        print("[info] IT_GUI=true: VMs will be launched with a graphical console (SPICE).", flush=True)
        print("[info] If the test fails/stalls, you can inspect in virt-manager (connect to qemu:///system).", flush=True)

    with tempfile.TemporaryDirectory(prefix="bootstrap-it-") as td:
        td_path = Path(td)
        uniq = uuid4().hex[:8]
        net_name = f"bootstrap-it-{os.getpid()}-{uniq}"
        uplink_name = f"bootstrap-uplink-{os.getpid()}-{uniq}"
        appliance_name = f"bootstrap-appliance-{os.getpid()}-{uniq}"
        client_name = f"bootstrap-client-{os.getpid()}-{uniq}"
        pool = os.environ.get("IT_LIBVIRT_POOL", "default")
        keep_volume = _env_bool("IT_LIBVIRT_KEEP_VOLUME", False)
        # Allow reusing the same imported volume across reruns to avoid re-uploading a large disk.
        vol_name = os.environ.get("IT_LIBVIRT_APPLIANCE_VOL", f"bootstrap-pxe-{bootstrap_version}.raw")
        appliance_disk_path = str(appliance_disk)
        client_vol_name = f"bootstrap-client-{uniq}.qcow2"
        client_vol_path: str | None = None

        # Best-effort cleanup from previous crashes (same PID reuse is unlikely but cheap to guard).
        _destroy_domain(client_name)
        _destroy_domain(appliance_name)
        _destroy_network(net_name)
        _destroy_network(uplink_name)

        network_xml = textwrap.dedent(f"""\
        <network>
          <name>{net_name}</name>
          <forward mode='none'/>
          <!-- Let libvirt auto-allocate a unique bridge name (avoids collisions if old bridges exist) -->
          <bridge stp='on' delay='0'/>
          <ip address='{net_ip}' netmask='{net_mask}'>
          </ip>
        </network>
        """)
        net_xml_path = td_path / "net.xml"
        _write_file(net_xml_path, network_xml)

        # Uplink/NAT network for the appliance (DHCP + internet egress via host).
        # This avoids relying on a pre-existing "default" libvirt network, which is
        # often missing/disabled in CI.
        uplink_gateway = os.environ.get("IT_UPLINK_GW", "192.168.123.1")
        uplink_netmask = os.environ.get("IT_UPLINK_MASK", "255.255.255.0")
        uplink_dhcp_start = os.environ.get("IT_UPLINK_DHCP_START", "192.168.123.100")
        uplink_dhcp_end = os.environ.get("IT_UPLINK_DHCP_END", "192.168.123.254")

        uplink_xml = textwrap.dedent(f"""\
        <network>
          <name>{uplink_name}</name>
          <forward mode='nat'/>
          <bridge stp='on' delay='0'/>
          <ip address='{uplink_gateway}' netmask='{uplink_netmask}'>
            <dhcp>
              <range start='{uplink_dhcp_start}' end='{uplink_dhcp_end}'/>
            </dhcp>
          </ip>
        </network>
        """)
        uplink_xml_path = td_path / "uplink.xml"
        _write_file(uplink_xml_path, uplink_xml)

        failed = False
        try:
            _virsh("net-define", str(net_xml_path))
            _virsh("net-start", net_name)
            _virsh("net-autostart", net_name, check=False)

            _virsh("net-define", str(uplink_xml_path))
            _virsh("net-start", uplink_name)
            _virsh("net-autostart", uplink_name, check=False)

            # When using system libvirt, QEMU often can't read disk images from /home (DAC/SELinux).
            # Import into a libvirt storage pool and use the pool-backed path instead.
            if _libvirt_uri() == "qemu:///system":
                appliance_disk_path = _ensure_libvirt_volume_from_file(pool=pool, src_path=appliance_disk, vol_name=vol_name)

            # Boot appliance from the existing raw disk.
            _run([
                "virt-install",
                "--connect", _libvirt_uri(),
                "--name", appliance_name,
                "--memory", "2048",
                "--vcpus", "2",
                "--import",
                "--os-variant", "ubuntu24.04",
                "--disk", f"path={appliance_disk_path},format=raw,bus=virtio",
                # Internal/PXE network (dnsmasq binds to this interface in the guest).
                "--network", f"network={net_name},model=virtio,mac={bootstrap_internal_mac}",
                # Uplink network (guest uses DHCP; appliance NATs PXE traffic out via this interface).
                "--network", f"network={uplink_name},model=virtio,mac={bootstrap_uplink_mac}",
                "--boot", "uefi",
                "--wait", "0",
                *_virt_install_console_args(),
            ], check=True, capture=True)

            # Wait for appliance HTTP to be reachable.
            _wait_until(
                deadline,
                interval_s=2.0,
                what=f"bootstrap appliance HTTP ({bootstrap_http})",
                fn=lambda: _curl_ok(f"{bootstrap_http}/boot.ipxe"),
            )

            # Extra assertions that catch common breakages early.
            assert _curl_ok(f"{bootstrap_http}/configs/minipc.yaml")
            assert _curl_ok(f"{bootstrap_http}/talos/{talos_version}/vmlinuz")
            assert _curl_ok(f"{bootstrap_http}/talos/{talos_version}/initramfs.xz")

            # Create a blank disk for the PXE client inside the libvirt pool (avoids /tmp SELinux/DAC issues).
            if _libvirt_uri() == "qemu:///system":
                client_vol_path = _create_empty_volume(
                    pool=pool,
                    vol_name=client_vol_name,
                    capacity_bytes=20 * 1024 * 1024 * 1024,
                    fmt="qcow2",
                )
            else:
                # Session libvirt can generally access temp paths.
                client_disk = td_path / "client.qcow2"
                _run(["qemu-img", "create", "-f", "qcow2", str(client_disk), "20G"], check=True, capture=True)
                client_vol_path = str(client_disk)

            # Boot a PXE client VM on the same isolated L2 network.
            #
            # Important: for UEFI PXE, the NIC model must be supported by the firmware.
            # OVMF typically supports e1000e out of the box; virtio-net may not PXE boot.
            _run([
                "virt-install",
                "--connect", _libvirt_uri(),
                "--name", client_name,
                "--memory", "2048",
                "--vcpus", "2",
                "--osinfo", "detect=on,require=off",
                "--pxe",
                "--boot", "uefi,network,hd",
                "--disk", f"path={client_vol_path},format=qcow2,bus=sata",
                "--network", f"network={net_name},model=e1000e,mac={minipc_mac}",
                "--wait", "0",
                *_virt_install_console_args(),
            ], check=True, capture=True)

            # Wait for Talos API port to come up (gRPC on 50000/tcp) on the expected IP.
            def talos_port_open() -> bool:
                return _tcp_port_open(minipc_ip, 50000, timeout_s=1.0)

            try:
                _wait_until(
                    deadline,
                    interval_s=3.0,
                    what=f"Talos API on {minipc_ip}:50000",
                    fn=talos_port_open,
                )
            except Exception:
                _debug_dump(net_name=net_name, appliance_name=appliance_name, client_name=client_name)
                failed = True
                raise

            # Prefer using the talosconfig baked/served by the appliance; allow override.
            talosconfig_path = os.environ.get("IT_TALOSCONFIG_PATH")
            if talosconfig_path:
                tc_path = Path(talosconfig_path)
            else:
                tc_path = td_path / "talosconfig"
                # If this fails (not served), the user can provide IT_TALOSCONFIG_PATH.
                _run(["curl", "-fsS", "-o", str(tc_path), f"{bootstrap_http}/configs/talosconfig"], check=True, capture=True)

            env = {"TALOSCONFIG": str(tc_path)}

            # Health check: wait until Talos reports healthy.
            # Note: talosctl 'health' already has internal retries; keep outer timeout generous.
            cp = _run([
                "talosctl",
                "-n", minipc_ip,
                "health",
                "--wait-timeout", "10m",
            ], check=True, capture=True, env=env)
            assert cp.returncode == 0

            # Sanity check: can query version.
            _run(["talosctl", "-n", minipc_ip, "version"], check=True, capture=True, env=env)

        finally:
            # Cleanup (optionally keep artifacts for debugging).
            if keep_always or (keep_on_fail and failed):
                print(f"\n[info] Keeping libvirt artifacts for debugging:", flush=True)
                print(f"[info]   net={net_name}", flush=True)
                print(f"[info]   appliance={appliance_name}", flush=True)
                print(f"[info]   client={client_name}", flush=True)
                print("[info] To clean up later:", flush=True)
                print(f"[info]   virsh -c {_libvirt_uri()} destroy {client_name} || true", flush=True)
                print(f"[info]   virsh -c {_libvirt_uri()} undefine {client_name} --nvram || true", flush=True)
                print(f"[info]   virsh -c {_libvirt_uri()} destroy {appliance_name} || true", flush=True)
                print(f"[info]   virsh -c {_libvirt_uri()} undefine {appliance_name} --nvram || true", flush=True)
                print(f"[info]   virsh -c {_libvirt_uri()} net-destroy {net_name} || true", flush=True)
                print(f"[info]   virsh -c {_libvirt_uri()} net-undefine {net_name} || true", flush=True)
                print(f"[info]   (and optionally) just -f {repo_root/'provisioning/bootstrap/justfile'} cleanup", flush=True)
                return

            _destroy_domain(client_name)
            _destroy_domain(appliance_name)
            if _libvirt_uri() == "qemu:///system":
                _delete_libvirt_volume(pool=pool, vol_name=client_vol_name)
                if not keep_volume:
                    _delete_libvirt_volume(pool=pool, vol_name=vol_name)
            _destroy_network(net_name)
            _destroy_network(uplink_name)


