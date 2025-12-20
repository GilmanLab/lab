"""
Pytest fixtures for VyOS Gateway integration tests.

This module provides fixtures for connecting to the VyOS gateway container
running in Containerlab.
"""

import os
import subprocess
import time
from dataclasses import dataclass
from typing import Callable, Iterable

import pytest
from scrapli import Scrapli


def wait_for_vyos_ready(host: str, timeout: int = 240, interval: int = 5) -> bool:
    """Wait for VyOS to be ready for SSH connections."""
    import socket

    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            sock.settimeout(5)
            result = sock.connect_ex((host, 22))
            sock.close()
            if result == 0:
                # SSH port is open, wait a bit more for VyOS to fully initialize
                time.sleep(10)
                return True
        except socket.error:
            pass
        time.sleep(interval)
    return False


@dataclass(frozen=True)
class TestTopology:
    """Expected values for the Containerlab test topology."""

    wan_iface: str = "eth4"
    wan_ip: str = "192.168.0.2/24"
    wan_gateway: str = "192.168.0.1"
    trunk_iface: str = "eth5"
    mgmt_vif: str = "10"
    mgmt_ip: str = "10.10.10.1/24"
    prov_vif: str = "20"
    prov_ip: str = "10.10.20.1/24"
    platform_vif: str = "30"
    platform_ip: str = "10.10.30.1/24"
    cluster_vif: str = "40"
    cluster_ip: str = "10.10.40.1/24"
    service_vif: str = "50"
    service_ip: str = "10.10.50.1/24"
    storage_vif: str = "60"
    storage_ip: str = "10.10.60.1/24"
    home_cidr: str = "192.168.0.0/24"
    lab_cidr: str = "10.10.0.0/16"
    dhcp_subnet: str = "10.10.10.0/24"
    dhcp_range_start: str = "10.10.10.200"
    dhcp_range_stop: str = "10.10.10.250"
    dns_listen_addresses: tuple[str, ...] = ("10.10.10.1", "10.10.30.1")
    bgp_neighbors: tuple[str, ...] = ("10.10.30.10", "10.10.30.11", "10.10.30.12")
    bgp_remote_as: str = "64513"
    bgp_local_as: str = "64512"
    bgp_router_id: str = "10.10.50.1"
    bgp_service_network: str = "10.10.50.0/24"
    domain_name: str = "lab.gilman.io"
    hostname: str = "gateway"
    name_servers: tuple[str, ...] = ("1.1.1.1", "8.8.8.8")
    time_zone: str = "America/Los_Angeles"


@pytest.fixture(scope="session")
def vyos_host() -> str:
    """Get the VyOS gateway hostname from environment or use default."""
    return os.environ.get("VYOS_HOST", "clab-vyos-gateway-test-gateway")


@pytest.fixture(scope="session")
def vyos_container(vyos_host: str) -> str:
    """Get the VyOS container name for docker exec."""
    return os.environ.get("VYOS_CONTAINER", vyos_host)


@pytest.fixture(scope="session")
def vyos_username() -> str:
    """Get the VyOS username from environment or use default."""
    return os.environ.get("VYOS_USER", "vyos")


@pytest.fixture(scope="session")
def vyos_password() -> str:
    """Get the VyOS password from environment or use default."""
    return os.environ.get("VYOS_PASS", "vyos")


@pytest.fixture(scope="session")
def vyos_private_key() -> str | None:
    """Get the path to the VyOS SSH private key, if provided."""
    env_key = os.environ.get("VYOS_SSH_KEY")
    if env_key:
        return env_key
    default_key = os.path.join(os.path.dirname(__file__), ".vyos-test-key")
    return default_key if os.path.exists(default_key) else None


@pytest.fixture(scope="session")
def test_topology() -> TestTopology:
    """Return the expected topology values for assertions."""
    return TestTopology()


@pytest.fixture(scope="session")
def vyos(
    vyos_host: str,
    vyos_username: str,
    vyos_password: str,
    vyos_private_key: str | None,
) -> Scrapli:
    """
    Create a Scrapli connection to the VyOS gateway.

    This fixture uses session scope so the connection is reused across all tests.
    """
    # Wait for VyOS to be ready
    if not wait_for_vyos_ready(vyos_host):
        pytest.fail(f"VyOS at {vyos_host} not ready after timeout")

    conn_args = {
        "host": vyos_host,
        "auth_username": vyos_username,
        "auth_strict_key": False,
        "transport": "system",
        "platform": "vyos_vyos",
        "transport_options": {"open_cmd": ["-tt"]},
    }
    if vyos_private_key:
        conn_args["auth_private_key"] = vyos_private_key
    else:
        conn_args["auth_password"] = vyos_password

    conn = Scrapli(**conn_args)
    conn.open()
    yield conn
    conn.close()


@pytest.fixture(scope="session")
def vyos_show(vyos: Scrapli) -> Callable[[str], str]:
    """Return a helper to run show commands and return output."""

    def _show(command: str) -> str:
        result = vyos.send_command(command)
        if result.failed:
            pytest.fail(f"Command failed: {command}")
        return normalize_output(result.result)

    return _show


@pytest.fixture(scope="session")
def config_commands(vyos_container: str) -> str:
    """Return the rendered config as VyOS set-style commands."""
    result = subprocess.run(
        [
            "docker",
            "exec",
            vyos_container,
            "vyos-config-to-commands",
            "/opt/vyatta/etc/config/config.boot",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        stderr = result.stderr.strip() or result.stdout.strip()
        pytest.fail(f"Failed to render config commands via docker exec: {stderr}")
    return normalize_output(result.stdout)


@pytest.fixture(scope="session")
def assert_contains() -> Callable[[str, Iterable[str], str], None]:
    """Return an assertion helper for checking output content."""

    def _assert(output: str, items: Iterable[str], context: str = "") -> None:
        missing = [item for item in items if item not in output]
        if missing:
            prefix = f"{context}: " if context else ""
            raise AssertionError(f"{prefix}missing {', '.join(missing)}")

    return _assert


def normalize_output(output: str) -> str:
    """Normalize VyOS CLI output for stable assertions."""
    warning_lines = {
        "WARNING: terminal is not fully functional",
        "Press RETURN to continue",
    }
    filtered = [line for line in output.splitlines() if line.strip() not in warning_lines]
    return "\n".join(filtered).replace("'", "")
