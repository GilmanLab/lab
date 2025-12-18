"""
Pytest fixtures for bootstrap integration tests.

Provides fixtures for libvirt resources with automatic cleanup.
"""

from __future__ import annotations

import os
import tempfile
import time
from pathlib import Path
from typing import TYPE_CHECKING, Generator
from uuid import uuid4

import pytest

from .config import IntegrationTestConfig
from .infrastructure import (
    VirshClient,
    ensure_raw_disk,
    require_bin,
    LibvirtResourceTracker,
)

if TYPE_CHECKING:
    from _pytest.fixtures import FixtureRequest


@pytest.fixture(scope="session")
def repo_root() -> Path:
    """Return the repository root path."""
    return Path(__file__).resolve().parents[4]


@pytest.fixture(scope="session")
def test_config(repo_root: Path) -> IntegrationTestConfig:
    """Load test configuration from environment with packer defaults."""
    return IntegrationTestConfig.from_env(repo_root)


@pytest.fixture(scope="session")
def virsh_client(test_config: IntegrationTestConfig) -> VirshClient:
    """Create a VirshClient instance."""
    return VirshClient(test_config.libvirt)


def _unique_suffix() -> str:
    """Generate a unique suffix for resource names."""
    return f"{os.getpid()}-{uuid4().hex[:8]}"


@pytest.fixture(scope="session")
def resource_suffix() -> str:
    """Unique suffix for all resources in this test session."""
    return _unique_suffix()


@pytest.fixture(scope="session")
def appliance_disk_path(
    test_config: IntegrationTestConfig,
    virsh_client: VirshClient,
) -> Generator[str, None, None]:
    """
    Ensure appliance disk is available and return its path.

    For system libvirt, imports the disk into a storage pool.
    """
    disk = ensure_raw_disk(test_config.appliance_disk)

    if not disk.exists():
        pytest.skip(
            f"Bootstrap appliance disk not found: {disk}\n"
            "(run `just build` first, or set IT_BOOTSTRAP_DISK to point at an existing image)"
        )

    if test_config.libvirt.is_system:
        # Import into libvirt pool to avoid SELinux/DAC issues
        disk_path = virsh_client.ensure_volume_from_file(
            pool=test_config.libvirt.pool,
            src_path=disk,
            vol_name=test_config.appliance_volume_name,
        )
        yield disk_path

        # Cleanup: only delete if not keeping
        if not test_config.libvirt.keep_volume:
            virsh_client.delete_volume(
                test_config.libvirt.pool,
                test_config.appliance_volume_name,
            )
    else:
        yield str(disk)


@pytest.fixture(scope="session")
def temp_dir() -> Generator[Path, None, None]:
    """Create a temporary directory for test artifacts."""
    with tempfile.TemporaryDirectory(prefix="bootstrap-it-") as td:
        yield Path(td)


@pytest.fixture(scope="session")
def libvirt_resources(
    test_config: IntegrationTestConfig,
    virsh_client: VirshClient,
    temp_dir: Path,
    resource_suffix: str,
    appliance_disk_path: str,
    request: FixtureRequest,
) -> Generator[LibvirtResourceTracker, None, None]:
    """
    Set up all libvirt resources for the test.

    This fixture:
    - Creates internal and uplink networks
    - Boots the appliance VM
    - Boots the PXE client VM
    - Cleans up all resources on teardown (unless keep flags are set)
    """
    # Check prerequisites
    require_bin("virsh")
    require_bin("virt-install")
    require_bin("curl")
    require_bin("qemu-img")
    require_bin("talosctl")

    # Verify libvirt is reachable
    cp = virsh_client.run("uri", check=False)
    if cp.returncode != 0:
        pytest.skip(f"libvirt not available to current user (virsh uri failed):\n{cp.stdout}")

    # Print GUI info if enabled
    if test_config.gui_enabled:
        print("[info] IT_GUI=true: VMs will be launched with a graphical console (SPICE).", flush=True)
        print("[info] If the test fails/stalls, you can inspect in virt-manager.", flush=True)

    tracker = LibvirtResourceTracker(
        virsh=virsh_client,
        config=test_config,
        temp_dir=temp_dir,
        suffix=resource_suffix,
    )

    try:
        # Setup in order: networks -> appliance -> client
        tracker.setup_networks()
        tracker.setup_appliance(appliance_disk_path)
        tracker.setup_client()

        yield tracker

    except Exception:
        tracker.failed = True
        raise

    finally:
        tracker.cleanup()


@pytest.fixture(scope="session")
def deadline(test_config: IntegrationTestConfig) -> float:
    """Calculate the test deadline as an absolute time."""
    return time.time() + test_config.timeout_s

