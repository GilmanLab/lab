"""
Integration tests for the bootstrap PXE appliance.

End-to-end smoke test that:
- Boots the bootstrap appliance on an isolated libvirt network
- Waits for it to serve PXE endpoints (HTTP)
- Boots a PXE client VM that boots via the appliance
- Verifies Talos comes up healthy via talosctl
"""

from __future__ import annotations

from pathlib import Path

import pytest

from .config import IntegrationTestConfig
from .infrastructure import LibvirtResourceTracker
from .verification import (
    assert_appliance_endpoints,
    assert_talos_healthy,
    bootstrap_talos,
    fetch_talosconfig,
    wait_for_appliance_http,
    wait_for_talos_api,
)


@pytest.mark.integration
def test_appliance_pxe_installs_talos(
    test_config: IntegrationTestConfig,
    libvirt_resources: LibvirtResourceTracker,
    deadline: float,
    temp_dir: Path,
) -> None:
    """
    End-to-end smoke test for PXE installation of Talos.

    This test verifies that:
    1. The bootstrap appliance boots and serves HTTP endpoints
    2. A PXE client can boot from the appliance
    3. Talos can be bootstrapped (etcd initialized)
    4. Talos comes up healthy on the client
    """
    # Wait for appliance HTTP to be reachable
    wait_for_appliance_http(test_config, deadline)

    # Verify expected endpoints are accessible
    assert_appliance_endpoints(test_config)

    # Wait for Talos API to come up on the client
    try:
        wait_for_talos_api(test_config, deadline)
    except Exception:
        libvirt_resources.debug_dump()
        libvirt_resources.failed = True
        raise

    # Fetch talosconfig
    talosconfig = fetch_talosconfig(test_config, temp_dir)

    # Bootstrap the cluster (initialize etcd)
    bootstrap_talos(test_config, talosconfig)

    # Verify Talos is healthy
    assert_talos_healthy(test_config, talosconfig)
