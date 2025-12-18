"""
Configuration management for integration tests.

Centralizes environment variable handling, packer defaults, and test configuration.
"""

from __future__ import annotations

import ipaddress
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import TypeVar

T = TypeVar("T", str, int)


# --- Core Configuration ---
# Talos version string used in the built appliance’s HTTP paths (default: v1.11.6)
ENV_TALOS_VERSION = "IT_TALOS_VERSION"
DEFAULT_TALOS_VERSION = "v1.11.6"

# Build/output directory version under `artifacts/bootstrap/<version>/` (default: local-test)
ENV_BOOTSTRAP_VERSION = "IT_BOOTSTRAP_VERSION"
DEFAULT_BOOTSTRAP_VERSION = "local-test"

# Path to the appliance raw disk
ENV_BOOTSTRAP_DISK = "IT_BOOTSTRAP_DISK"

# Overall timeout in seconds (default: 900)
ENV_TIMEOUT_S = "IT_TIMEOUT_S"
DEFAULT_TIMEOUT_S = 900


# --- Network Configuration ---
# Expected appliance IP (default: read from packer/bootstrap.pkr.hcl `vm_ip` or 192.168.2.1)
ENV_BOOTSTRAP_IP = "IT_BOOTSTRAP_IP"
DEFAULT_VM_IP = "192.168.2.1"

# Expected Talos node IP (default: read from packer/bootstrap.pkr.hcl `minipc_ip` or 192.168.2.2)
ENV_MINIPC_IP = "IT_MINIPC_IP"
DEFAULT_MINIPC_IP = "192.168.2.2"

# Host IP for the isolated network
ENV_NET_HOST_IP = "IT_NET_HOST_IP"

# Netmask for the isolated network
ENV_NET_MASK = "IT_NET_MASK"
DEFAULT_NET_MASK = "255.255.255.0"

# VM network prefix (only used to calculate host IP default)
DEFAULT_VM_PREFIX = 24


# --- Uplink (NAT) Configuration ---
ENV_UPLINK_GW = "IT_UPLINK_GW"
DEFAULT_UPLINK_GW = "192.168.123.1"

ENV_UPLINK_MASK = "IT_UPLINK_MASK"
DEFAULT_UPLINK_MASK = "255.255.255.0"

ENV_UPLINK_DHCP_START = "IT_UPLINK_DHCP_START"
DEFAULT_UPLINK_DHCP_START = "192.168.123.100"

ENV_UPLINK_DHCP_END = "IT_UPLINK_DHCP_END"
DEFAULT_UPLINK_DHCP_END = "192.168.123.254"


# --- MAC Addresses ---
# Internal NIC MAC (DHCP/TFTP/HTTP)
ENV_BOOTSTRAP_INTERNAL_MAC = "IT_BOOTSTRAP_INTERNAL_MAC"
DEFAULT_BOOTSTRAP_INTERNAL_MAC = "02:11:32:24:64:5a"

# Uplink NIC MAC (NAT/Internet)
ENV_BOOTSTRAP_UPLINK_MAC = "IT_BOOTSTRAP_UPLINK_MAC"
DEFAULT_BOOTSTRAP_UPLINK_MAC = "02:11:32:24:64:5c"

# Client VM MAC (pinned for static lease in dnsmasq)
ENV_MINIPC_MAC = "IT_MINIPC_MAC"
DEFAULT_MINIPC_MAC = "02:11:32:24:64:5b"


# --- Libvirt Configuration ---
ENV_LIBVIRT_URI = "IT_LIBVIRT_URI"
DEFAULT_LIBVIRT_URI = "qemu:///system"

ENV_LIBVIRT_POOL = "IT_LIBVIRT_POOL"
DEFAULT_LIBVIRT_POOL = "default"

ENV_LIBVIRT_APPLIANCE_VOL = "IT_LIBVIRT_APPLIANCE_VOL"
ENV_LIBVIRT_KEEP_VOLUME = "IT_LIBVIRT_KEEP_VOLUME"
ENV_LIBVIRT_REFRESH_VOLUME = "IT_LIBVIRT_REFRESH_VOLUME"


# --- Behavior Flags ---
# Use sudo for libvirt commands
ENV_SUDO = "IT_SUDO"

# Launch VMs with graphical console
ENV_GUI = "IT_GUI"

# Keep VMs/networks around on success
ENV_KEEP = "IT_KEEP"

# Keep VMs/networks around on failure
ENV_KEEP_ON_FAIL = "IT_KEEP_ON_FAIL"

# Path to a local talosconfig
ENV_TALOSCONFIG_PATH = "IT_TALOSCONFIG_PATH"


# --- Constants ---
TALOS_API_PORT = 50000
VM_MEMORY_MB = 2048
VM_VCPUS = 2
CLIENT_DISK_SIZE_GB = 20


def env_bool(name: str, default: bool = False) -> bool:
    """Parse a boolean from an environment variable."""
    v = os.environ.get(name)
    if v is None:
        return default
    return v.strip().lower() in {"1", "true", "yes", "y", "on"}


def env_str(name: str, default: str) -> str:
    """Get a string from an environment variable with a default."""
    return os.environ.get(name, default)


def env_int(name: str, default: int) -> int:
    """Get an integer from an environment variable with a default."""
    v = os.environ.get(name)
    if v is None:
        return default
    return int(v)


def packer_var(repo_root: Path, var_name: str, *, as_int: bool = False) -> str | int | None:
    """
    Extract default value for a packer variable from bootstrap.pkr.hcl.

    Args:
        repo_root: Path to the repository root
        var_name: Name of the packer variable to extract
        as_int: If True, parse the value as an integer

    Returns:
        The default value, or None if not found
    """
    pkr = repo_root / "provisioning/bootstrap/packer/bootstrap.pkr.hcl"
    if not pkr.exists():
        return None

    content = pkr.read_text(encoding="utf-8")

    # Build regex pattern based on expected value type
    base_pattern = rf'variable\s+"{re.escape(var_name)}"\s*\{{[\s\S]*?default\s*=\s*'
    if as_int:
        pattern = base_pattern + r"([0-9]+)"
    else:
        pattern = base_pattern + r'"([^"]+)"'

    if m := re.search(pattern, content, re.MULTILINE):
        value = m.group(1).strip()
        return int(value) if as_int else value
    return None


def pick_host_ip(
    network: ipaddress.IPv4Network,
    *,
    avoid: set[ipaddress.IPv4Address],
) -> ipaddress.IPv4Address:
    """
    Pick a host IP from a network, avoiding specified addresses.

    Prefers common host IP offsets (.254, .253, etc.) but falls back to any available.
    """
    # Preferred offsets in order of preference
    preferred_offsets = (254, 253, 250, 10, 1)
    candidates = []

    for offset in preferred_offsets:
        try:
            candidates.append(network.network_address + offset)
        except Exception:
            continue

    # Try preferred candidates first
    for ip in candidates:
        if (
            ip in network
            and ip != network.network_address
            and ip != network.broadcast_address
            and ip not in avoid
        ):
            return ip

    # Fall back to any available host
    for ip in network.hosts():
        if ip not in avoid:
            return ip

    raise ValueError(f"No usable host IPs in network {network}")


@dataclass(frozen=True)
class UplinkNetworkConfig:
    """Configuration for the uplink NAT network."""

    gateway: str
    netmask: str
    dhcp_start: str
    dhcp_end: str

    @classmethod
    def from_env(cls) -> UplinkNetworkConfig:
        return cls(
            gateway=env_str(ENV_UPLINK_GW, DEFAULT_UPLINK_GW),
            netmask=env_str(ENV_UPLINK_MASK, DEFAULT_UPLINK_MASK),
            dhcp_start=env_str(ENV_UPLINK_DHCP_START, DEFAULT_UPLINK_DHCP_START),
            dhcp_end=env_str(ENV_UPLINK_DHCP_END, DEFAULT_UPLINK_DHCP_END),
        )


@dataclass(frozen=True)
class LibvirtConfig:
    """Libvirt-specific configuration."""

    uri: str
    pool: str
    keep_volume: bool
    refresh_volume: bool
    use_sudo: bool

    @classmethod
    def from_env(cls) -> LibvirtConfig:
        return cls(
            uri=env_str(ENV_LIBVIRT_URI, DEFAULT_LIBVIRT_URI),
            pool=env_str(ENV_LIBVIRT_POOL, DEFAULT_LIBVIRT_POOL),
            keep_volume=env_bool(ENV_LIBVIRT_KEEP_VOLUME),
            refresh_volume=env_bool(ENV_LIBVIRT_REFRESH_VOLUME),
            use_sudo=env_bool(ENV_SUDO),
        )

    @property
    def is_system(self) -> bool:
        """Check if using system libvirt (vs session)."""
        return self.uri == "qemu:///system"

    @property
    def sudo_prefix(self) -> list[str]:
        """Get sudo prefix for commands if enabled."""
        return ["sudo", "-n"] if self.use_sudo else []


@dataclass(frozen=True)
class IntegrationTestConfig:
    """
    Complete configuration for integration tests.

    Loaded from environment variables with packer-derived defaults where applicable.
    """

    # Talos settings
    talos_version: str

    # Bootstrap appliance settings
    bootstrap_ip: str
    bootstrap_internal_mac: str
    bootstrap_uplink_mac: str
    bootstrap_version: str
    appliance_disk: Path

    # PXE client settings
    minipc_ip: str
    minipc_mac: str

    # Network settings
    net_host_ip: str
    net_mask: str
    uplink: UplinkNetworkConfig

    # Libvirt settings
    libvirt: LibvirtConfig

    # Timing
    timeout_s: int

    # Behavior flags
    gui_enabled: bool
    keep_on_fail: bool
    keep_always: bool

    # Optional overrides
    talosconfig_path: Path | None

    # Computed paths
    repo_root: Path

    @property
    def bootstrap_http(self) -> str:
        """Base HTTP URL for the bootstrap appliance."""
        return f"http://{self.bootstrap_ip}"

    @property
    def appliance_volume_name(self) -> str:
        """Libvirt volume name for the appliance disk."""
        override = os.environ.get(ENV_LIBVIRT_APPLIANCE_VOL)
        if override:
            return override
        return f"bootstrap-pxe-{self.bootstrap_version}.raw"

    @classmethod
    def from_env(cls, repo_root: Path | None = None) -> IntegrationTestConfig:
        """
        Load configuration from environment variables with packer defaults.

        Args:
            repo_root: Repository root path. If None, derived from file location.
        """
        if repo_root is None:
            repo_root = Path(__file__).resolve().parents[4]

        # Load packer defaults
        packer_vm_ip = packer_var(repo_root, "vm_ip") or DEFAULT_VM_IP
        packer_minipc_ip = packer_var(repo_root, "minipc_ip") or DEFAULT_MINIPC_IP
        packer_vm_prefix = packer_var(repo_root, "vm_prefix", as_int=True) or DEFAULT_VM_PREFIX

        # Ensure these are strings for type checking
        if isinstance(packer_vm_ip, int):
            packer_vm_ip = str(packer_vm_ip)
        if isinstance(packer_minipc_ip, int):
            packer_minipc_ip = str(packer_minipc_ip)

        bootstrap_ip = env_str(ENV_BOOTSTRAP_IP, packer_vm_ip)
        minipc_ip = env_str(ENV_MINIPC_IP, packer_minipc_ip)

        # MAC addresses from packer or defaults
        bootstrap_internal_mac = (
            os.environ.get(ENV_BOOTSTRAP_INTERNAL_MAC)
            or packer_var(repo_root, "bootstrap_internal_mac")
            or DEFAULT_BOOTSTRAP_INTERNAL_MAC
        )
        bootstrap_uplink_mac = (
            os.environ.get(ENV_BOOTSTRAP_UPLINK_MAC)
            or packer_var(repo_root, "bootstrap_uplink_mac")
            or DEFAULT_BOOTSTRAP_UPLINK_MAC
        )
        minipc_mac = (
            os.environ.get(ENV_MINIPC_MAC)
            or packer_var(repo_root, "minipc_mac")
            or DEFAULT_MINIPC_MAC
        )

        # Ensure MAC addresses are lowercase strings
        if isinstance(bootstrap_internal_mac, int):
            bootstrap_internal_mac = str(bootstrap_internal_mac)
        if isinstance(bootstrap_uplink_mac, int):
            bootstrap_uplink_mac = str(bootstrap_uplink_mac)
        if isinstance(minipc_mac, int):
            minipc_mac = str(minipc_mac)

        # Compute network host IP
        try:
            net = ipaddress.IPv4Network(f"{packer_vm_ip}/{packer_vm_prefix}", strict=False)
            avoid = {ipaddress.IPv4Address(packer_vm_ip), ipaddress.IPv4Address(packer_minipc_ip)}
            default_net_ip = str(pick_host_ip(net, avoid=avoid))
        except Exception:
            default_net_ip = "192.168.2.254"

        # Locate appliance disk
        bootstrap_version = env_str(ENV_BOOTSTRAP_VERSION, DEFAULT_BOOTSTRAP_VERSION)
        disk_env = os.environ.get(ENV_BOOTSTRAP_DISK)
        if disk_env:
            appliance_disk = Path(disk_env)
        else:
            appliance_disk = repo_root / "artifacts/bootstrap" / bootstrap_version / "bootstrap-pxe.raw"

        # Talosconfig path
        tc_path_env = os.environ.get(ENV_TALOSCONFIG_PATH)
        talosconfig_path = Path(tc_path_env) if tc_path_env else None

        # GUI affects keep_on_fail default
        gui_enabled = env_bool(ENV_GUI)

        return cls(
            talos_version=env_str(ENV_TALOS_VERSION, DEFAULT_TALOS_VERSION),
            bootstrap_ip=bootstrap_ip,
            bootstrap_internal_mac=bootstrap_internal_mac.lower(),
            bootstrap_uplink_mac=bootstrap_uplink_mac.lower(),
            bootstrap_version=bootstrap_version,
            appliance_disk=appliance_disk,
            minipc_ip=minipc_ip,
            minipc_mac=minipc_mac.lower(),
            net_host_ip=env_str(ENV_NET_HOST_IP, default_net_ip),
            net_mask=env_str(ENV_NET_MASK, DEFAULT_NET_MASK),
            uplink=UplinkNetworkConfig.from_env(),
            libvirt=LibvirtConfig.from_env(),
            timeout_s=env_int(ENV_TIMEOUT_S, DEFAULT_TIMEOUT_S),
            gui_enabled=gui_enabled,
            keep_on_fail=env_bool(ENV_KEEP_ON_FAIL, gui_enabled),
            keep_always=env_bool(ENV_KEEP),
            talosconfig_path=talosconfig_path,
            repo_root=repo_root,
        )

