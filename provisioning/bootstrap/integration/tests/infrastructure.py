"""
Libvirt helper utilities for integration tests.

Provides wrappers for virsh, virt-install, and volume management.
"""

from __future__ import annotations

import os
import shutil
import socket
import subprocess
import textwrap
import time
from pathlib import Path
from typing import Callable
from uuid import uuid4

import pytest

from .config import (
    CLIENT_DISK_SIZE_GB,
    VM_MEMORY_MB,
    VM_VCPUS,
    IntegrationTestConfig,
    LibvirtConfig,
    env_bool,
)


def run_cmd(
    cmd: list[str],
    *,
    check: bool = True,
    capture: bool = True,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    """
    Run a command with standard options.

    Args:
        cmd: Command and arguments to run
        check: Raise on non-zero exit code
        capture: Capture stdout/stderr
        env: Additional environment variables to set
    """
    kwargs: dict = {
        "check": check,
        "text": True,
        "env": {**os.environ, **(env or {})},
    }
    if capture:
        kwargs["stdout"] = subprocess.PIPE
        kwargs["stderr"] = subprocess.STDOUT
    return subprocess.run(cmd, **kwargs)


def require_bin(name: str) -> None:
    """Skip the test if a required binary is not available."""
    if shutil.which(name) is None:
        pytest.skip(f"Missing required binary on PATH: {name}")


def wait_until(
    deadline: float,
    *,
    interval_s: float,
    what: str,
    fn: Callable[[], bool],
    log_interval_s: float = 15.0,
) -> None:
    """
    Wait for a condition to become true.

    Args:
        deadline: Absolute time (time.time()) by which condition must be true
        interval_s: Seconds between checks
        what: Human-readable description of what we're waiting for
        fn: Callable that returns True when condition is met
        log_interval_s: Seconds between progress log messages

    Raises:
        AssertionError: If deadline is reached without condition becoming true
    """
    last_exc: Exception | None = None
    start = time.time()
    last_log = 0.0

    while time.time() < deadline:
        try:
            if fn():
                return
        except Exception as e:  # noqa: BLE001 - used for retry loop diagnostics
            last_exc = e

        now = time.time()
        # Emit periodic progress so long-running waits aren't silent under -s
        if now - last_log >= log_interval_s:
            remaining = max(0, int(deadline - now))
            elapsed = int(now - start)
            print(f"[wait] {what} (elapsed={elapsed}s remaining~{remaining}s)", flush=True)
            last_log = now

        time.sleep(interval_s)

    if last_exc:
        raise AssertionError(f"Timed out waiting for: {what}. Last error: {last_exc}") from last_exc
    raise AssertionError(f"Timed out waiting for: {what}")


def curl_ok(url: str) -> bool:
    """Check if a URL is reachable with curl."""
    cp = run_cmd(
        ["curl", "-fsS", "--max-time", "2", "-o", "/dev/null", url],
        check=False,
        capture=True,
    )
    return cp.returncode == 0


def tcp_port_open(host: str, port: int, timeout_s: float = 1.0) -> bool:
    """Check if a TCP port is accepting connections."""
    try:
        with socket.create_connection((host, port), timeout=timeout_s):
            return True
    except OSError:
        return False


def write_file(path: Path, content: str) -> None:
    """Write content to a file, creating parent directories as needed."""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def ensure_raw_disk(disk_path: Path) -> Path:
    """
    Ensure we have a plaintext .raw disk image.

    If only <disk>.gz exists, decompress it with gunzip -k.

    Args:
        disk_path: Expected path to the raw disk

    Returns:
        Path to the raw disk (may be same as input if it already exists)
    """
    if disk_path.exists():
        return disk_path

    gz = disk_path.with_suffix(disk_path.suffix + ".gz")
    if gz.exists():
        require_bin("gunzip")
        print(f"[info] Decompressing {gz} -> {disk_path}", flush=True)
        run_cmd(["gunzip", "-k", "-f", str(gz)], check=True, capture=True)
        return disk_path

    return disk_path


class VirshClient:
    """
    Client for libvirt virsh operations.

    Wraps virsh commands with consistent connection and sudo handling.
    """

    def __init__(self, config: LibvirtConfig):
        self.config = config

    def _cmd(self, *args: str) -> list[str]:
        """Build a virsh command with connection and sudo prefix."""
        return [*self.config.sudo_prefix, "virsh", "-c", self.config.uri, *args]

    def run(self, *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
        """Run a virsh command."""
        return run_cmd(self._cmd(*args), check=check, capture=True)

    def stdout(self, *args: str) -> str:
        """Run a virsh command and return stdout stripped."""
        cp = self.run(*args, check=True)
        return (cp.stdout or "").strip()

    def exists(self, kind: str, name: str) -> bool:
        """
        Check if a domain or network exists.

        Args:
            kind: "dom" for domain, "net" for network
            name: Resource name
        """
        if kind == "dom":
            cp = self.run("dominfo", name, check=False)
            return cp.returncode == 0
        if kind == "net":
            cp = self.run("net-info", name, check=False)
            return cp.returncode == 0
        raise ValueError(f"unknown kind: {kind}")

    def destroy_domain(self, name: str) -> None:
        """Destroy and undefine a domain if it exists."""
        if not self.exists("dom", name):
            return
        self.run("destroy", name, check=False)
        # UEFI nvram cleanup when present
        self.run("undefine", name, "--nvram", check=False)
        self.run("undefine", name, check=False)

    def destroy_network(self, name: str) -> None:
        """Destroy and undefine a network if it exists."""
        if not self.exists("net", name):
            return
        self.run("net-destroy", name, check=False)
        self.run("net-undefine", name, check=False)

    def define_and_start_network(self, xml_path: Path, name: str) -> None:
        """Define, start, and optionally autostart a network."""
        self.run("net-define", str(xml_path))
        self.run("net-start", name)
        self.run("net-autostart", name, check=False)

    def ensure_volume_from_file(
        self,
        pool: str,
        src_path: Path,
        vol_name: str,
        *,
        refresh: bool = False,
    ) -> str:
        """
        Copy a file into a libvirt storage pool volume.

        This avoids SELinux/DAC issues when the source file lives under /home.

        Args:
            pool: Storage pool name
            src_path: Source file path
            vol_name: Target volume name
            refresh: Force re-upload even if volume exists

        Returns:
            Path to the volume in the pool
        """
        # Ensure pool exists/active
        cp = self.run("pool-info", pool, check=False)
        if cp.returncode != 0:
            raise RuntimeError(f"libvirt pool not available: {pool}\n{cp.stdout}")

        should_refresh = refresh or env_bool("IT_LIBVIRT_REFRESH_VOLUME")

        # If volume doesn't exist, create it
        cp = self.run("vol-info", vol_name, pool, check=False)
        if cp.returncode != 0:
            self.run(
                "vol-create-as",
                pool,
                vol_name,
                str(src_path.stat().st_size),
                "--format",
                "raw",
                check=True,
            )
            should_refresh = True

        # Upload if requested or if we just created the volume
        if should_refresh:
            print(f"[info] Refreshing libvirt volume {pool}/{vol_name} from {src_path}", flush=True)
            self.run("vol-upload", vol_name, str(src_path), pool, "--sparse", check=True)

        vol_path = self.stdout("vol-path", vol_name, pool)
        if not vol_path:
            raise RuntimeError(f"Unable to resolve volume path for {pool}/{vol_name}")
        return vol_path

    def create_empty_volume(
        self,
        pool: str,
        vol_name: str,
        capacity_bytes: int,
        fmt: str = "qcow2",
    ) -> str:
        """
        Create an empty volume in a storage pool.

        Args:
            pool: Storage pool name
            vol_name: Volume name
            capacity_bytes: Volume capacity
            fmt: Volume format (e.g., "qcow2", "raw")

        Returns:
            Path to the volume
        """
        cp = self.run("vol-info", vol_name, pool, check=False)
        if cp.returncode != 0:
            self.run(
                "vol-create-as",
                pool,
                vol_name,
                str(capacity_bytes),
                "--format",
                fmt,
                check=True,
            )

        vol_path = self.stdout("vol-path", vol_name, pool)
        if not vol_path:
            raise RuntimeError(f"Unable to resolve volume path for {pool}/{vol_name}")
        return vol_path

    def delete_volume(self, pool: str, vol_name: str) -> None:
        """Delete a volume from a pool (ignores errors)."""
        self.run("vol-delete", vol_name, pool, check=False)

    def debug_dump(
        self,
        *,
        net_name: str,
        appliance_name: str,
        client_name: str,
    ) -> None:
        """
        Print debug information about resources.

        Best-effort output to make failures actionable.
        """
        resources = [
            ("net-info", net_name),
            ("net-dumpxml", net_name),
            ("domstate", appliance_name),
            ("domiflist", appliance_name),
            ("domblklist", appliance_name),
            ("domstate", client_name),
            ("domiflist", client_name),
            ("domblklist", client_name),
        ]

        for cmd_name, resource in resources:
            cmd = self._cmd(cmd_name, resource)
            cp = run_cmd(cmd, check=False, capture=True)
            print(f"\n[debug] $ {' '.join(cmd)}\n{cp.stdout}", flush=True)


def virt_install_console_args(gui_enabled: bool) -> list[str]:
    """
    Get virt-install console/graphics arguments.

    Args:
        gui_enabled: If True, launch with SPICE graphics; otherwise headless
    """
    if gui_enabled:
        return ["--graphics", "spice"]
    return ["--graphics", "none", "--noautoconsole"]


def generate_isolated_network_xml(
    name: str,
    ip: str,
    netmask: str,
) -> str:
    """Generate XML for an isolated libvirt network."""
    return textwrap.dedent(f"""\
        <network>
          <name>{name}</name>
          <forward mode='none'/>
          <!-- Let libvirt auto-allocate a unique bridge name -->
          <bridge stp='on' delay='0'/>
          <ip address='{ip}' netmask='{netmask}'>
          </ip>
        </network>
    """)


def generate_nat_network_xml(
    name: str,
    gateway: str,
    netmask: str,
    dhcp_start: str,
    dhcp_end: str,
) -> str:
    """Generate XML for a NAT libvirt network with DHCP."""
    return textwrap.dedent(f"""\
        <network>
          <name>{name}</name>
          <forward mode='nat'/>
          <bridge stp='on' delay='0'/>
          <ip address='{gateway}' netmask='{netmask}'>
            <dhcp>
              <range start='{dhcp_start}' end='{dhcp_end}'/>
            </dhcp>
          </ip>
        </network>
    """)


def create_appliance_vm(
    *,
    virsh: VirshClient,
    name: str,
    disk_path: str,
    internal_network: str,
    uplink_network: str,
    internal_mac: str,
    uplink_mac: str,
    gui_enabled: bool,
    memory_mb: int = VM_MEMORY_MB,
    vcpus: int = VM_VCPUS,
) -> None:
    """
    Create and start the bootstrap appliance VM.

    Args:
        virsh: VirshClient instance
        name: VM name
        disk_path: Path to the appliance disk image
        internal_network: Name of the internal/PXE network
        uplink_network: Name of the uplink/NAT network
        internal_mac: MAC address for internal NIC
        uplink_mac: MAC address for uplink NIC
        gui_enabled: Enable graphical console
        memory_mb: Memory allocation in MB
        vcpus: Number of virtual CPUs
    """
    run_cmd(
        [
            *virsh.config.sudo_prefix,
            "virt-install",
            "--connect",
            virsh.config.uri,
            "--name",
            name,
            "--memory",
            str(memory_mb),
            "--vcpus",
            str(vcpus),
            "--import",
            "--os-variant",
            "ubuntu24.04",
            "--disk",
            f"path={disk_path},format=raw,bus=virtio",
            "--network",
            f"network={internal_network},model=virtio,mac={internal_mac}",
            "--network",
            f"network={uplink_network},model=virtio,mac={uplink_mac}",
            "--boot",
            "uefi",
            "--wait",
            "0",
            *virt_install_console_args(gui_enabled),
        ],
        check=True,
        capture=True,
    )


def create_pxe_client_vm(
    *,
    virsh: VirshClient,
    name: str,
    disk_path: str,
    network: str,
    mac: str,
    gui_enabled: bool,
    memory_mb: int = VM_MEMORY_MB,
    vcpus: int = VM_VCPUS,
) -> None:
    """
    Create and start a PXE client VM.

    Uses e1000e NIC for UEFI PXE compatibility with OVMF.

    Args:
        virsh: VirshClient instance
        name: VM name
        disk_path: Path to the client disk image
        network: Name of the network to attach to
        mac: MAC address for the NIC
        gui_enabled: Enable graphical console
        memory_mb: Memory allocation in MB
        vcpus: Number of virtual CPUs
    """
    run_cmd(
        [
            *virsh.config.sudo_prefix,
            "virt-install",
            "--connect",
            virsh.config.uri,
            "--name",
            name,
            "--memory",
            str(memory_mb),
            "--vcpus",
            str(vcpus),
            "--osinfo",
            "detect=on,require=off",
            "--pxe",
            "--boot",
            "uefi,network,hd",
            "--disk",
            f"path={disk_path},format=qcow2,bus=sata",
            "--network",
            f"network={network},model=e1000e,mac={mac}",
            "--wait",
            "0",
            *virt_install_console_args(gui_enabled),
        ],
        check=True,
        capture=True,
    )


class LibvirtResourceTracker:
    """Tracks libvirt resources for cleanup."""

    def __init__(
        self,
        virsh: VirshClient,
        config: IntegrationTestConfig,
        temp_dir: Path,
        suffix: str,
    ):
        self.virsh = virsh
        self.config = config
        self.temp_dir = temp_dir
        self.suffix = suffix

        # Resource names
        self.internal_network = f"bootstrap-it-{suffix}"
        self.uplink_network = f"bootstrap-uplink-{suffix}"
        self.appliance_name = f"bootstrap-appliance-{suffix}"
        self.client_name = f"bootstrap-client-{suffix}"
        self.client_vol_name = f"bootstrap-client-{suffix}.qcow2"

        # Track what we've created
        self._networks_created: list[str] = []
        self._domains_created: list[str] = []
        self._volumes_created: list[str] = []

        # State
        self.failed = False
        self.client_disk_path: str | None = None

    def setup_networks(self) -> None:
        """Create the internal and uplink networks."""
        # Pre-cleanup in case of leftover resources
        self.virsh.destroy_network(self.internal_network)
        self.virsh.destroy_network(self.uplink_network)

        # Internal (isolated) network
        internal_xml = generate_isolated_network_xml(
            name=self.internal_network,
            ip=self.config.net_host_ip,
            netmask=self.config.net_mask,
        )
        internal_xml_path = self.temp_dir / "internal-net.xml"
        write_file(internal_xml_path, internal_xml)
        self.virsh.define_and_start_network(internal_xml_path, self.internal_network)
        self._networks_created.append(self.internal_network)

        # Uplink (NAT) network
        uplink_xml = generate_nat_network_xml(
            name=self.uplink_network,
            gateway=self.config.uplink.gateway,
            netmask=self.config.uplink.netmask,
            dhcp_start=self.config.uplink.dhcp_start,
            dhcp_end=self.config.uplink.dhcp_end,
        )
        uplink_xml_path = self.temp_dir / "uplink-net.xml"
        write_file(uplink_xml_path, uplink_xml)
        self.virsh.define_and_start_network(uplink_xml_path, self.uplink_network)
        self._networks_created.append(self.uplink_network)

    def setup_appliance(self, disk_path: str) -> None:
        """Create and start the bootstrap appliance VM."""
        self.virsh.destroy_domain(self.appliance_name)

        create_appliance_vm(
            virsh=self.virsh,
            name=self.appliance_name,
            disk_path=disk_path,
            internal_network=self.internal_network,
            uplink_network=self.uplink_network,
            internal_mac=self.config.bootstrap_internal_mac,
            uplink_mac=self.config.bootstrap_uplink_mac,
            gui_enabled=self.config.gui_enabled,
        )
        self._domains_created.append(self.appliance_name)

    def setup_client(self) -> str:
        """Create and start the PXE client VM. Returns the disk path."""
        self.virsh.destroy_domain(self.client_name)

        # Create client disk
        if self.config.libvirt.is_system:
            self.client_disk_path = self.virsh.create_empty_volume(
                pool=self.config.libvirt.pool,
                vol_name=self.client_vol_name,
                capacity_bytes=CLIENT_DISK_SIZE_GB * 1024 * 1024 * 1024,
                fmt="qcow2",
            )
            self._volumes_created.append(self.client_vol_name)
        else:
            client_disk = self.temp_dir / "client.qcow2"
            run_cmd(
                ["qemu-img", "create", "-f", "qcow2", str(client_disk), "20G"],
                check=True,
                capture=True,
            )
            self.client_disk_path = str(client_disk)

        create_pxe_client_vm(
            virsh=self.virsh,
            name=self.client_name,
            disk_path=self.client_disk_path,
            network=self.internal_network,
            mac=self.config.minipc_mac,
            gui_enabled=self.config.gui_enabled,
        )
        self._domains_created.append(self.client_name)

        return self.client_disk_path

    def cleanup(self) -> None:
        """Clean up all created resources."""
        should_keep = self.config.keep_always or (self.config.keep_on_fail and self.failed)

        if should_keep:
            self._print_keep_message()
            return

        # Destroy domains in reverse order
        for domain in reversed(self._domains_created):
            self.virsh.destroy_domain(domain)

        # Delete volumes
        if self.config.libvirt.is_system:
            for vol in self._volumes_created:
                self.virsh.delete_volume(self.config.libvirt.pool, vol)

        # Destroy networks
        for net in reversed(self._networks_created):
            self.virsh.destroy_network(net)

    def _print_keep_message(self) -> None:
        """Print message about kept resources."""
        uri = self.config.libvirt.uri
        print("\n[info] Keeping libvirt artifacts for debugging:", flush=True)
        print(f"[info]   networks: {', '.join(self._networks_created)}", flush=True)
        print(f"[info]   domains: {', '.join(self._domains_created)}", flush=True)
        print("[info] To clean up later:", flush=True)
        for domain in reversed(self._domains_created):
            print(f"[info]   virsh -c {uri} destroy {domain} || true", flush=True)
            print(f"[info]   virsh -c {uri} undefine {domain} --nvram || true", flush=True)
        for net in reversed(self._networks_created):
            print(f"[info]   virsh -c {uri} net-destroy {net} || true", flush=True)
            print(f"[info]   virsh -c {uri} net-undefine {net} || true", flush=True)
        print(
            f"[info]   (and optionally) just -f {self.config.repo_root / 'provisioning/bootstrap/justfile'} cleanup",
            flush=True,
        )

    def debug_dump(self) -> None:
        """Print debug information about resources."""
        self.virsh.debug_dump(
            net_name=self.internal_network,
            appliance_name=self.appliance_name,
            client_name=self.client_name,
        )
