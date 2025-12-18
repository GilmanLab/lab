"""
Verification logic for integration tests.

Contains assertion assertions and wait loops to keep test functions clean.
"""

from __future__ import annotations

from pathlib import Path

from .config import TALOS_API_PORT, IntegrationTestConfig
from .infrastructure import curl_ok, run_cmd, tcp_port_open, wait_until


def wait_for_appliance_http(
    config: IntegrationTestConfig,
    deadline: float,
) -> None:
    """Wait for the appliance HTTP server to be reachable."""
    wait_until(
        deadline,
        interval_s=2.0,
        what=f"bootstrap appliance HTTP ({config.bootstrap_http})",
        fn=lambda: curl_ok(f"{config.bootstrap_http}/boot.ipxe"),
    )


def assert_appliance_endpoints(config: IntegrationTestConfig) -> None:
    """Verify expected HTTP endpoints are accessible."""
    endpoints = [
        "/configs/minipc.yaml",
        f"/talos/{config.talos_version}/vmlinuz",
        f"/talos/{config.talos_version}/initramfs.xz",
    ]

    for endpoint in endpoints:
        url = f"{config.bootstrap_http}{endpoint}"
        assert curl_ok(url), f"Expected endpoint not accessible: {url}"


def wait_for_talos_api(
    config: IntegrationTestConfig,
    deadline: float,
) -> None:
    """Wait for the Talos API port to be reachable."""
    wait_until(
        deadline,
        interval_s=3.0,
        what=f"Talos API on {config.minipc_ip}:{TALOS_API_PORT}",
        fn=lambda: tcp_port_open(config.minipc_ip, TALOS_API_PORT),
    )


def fetch_talosconfig(
    config: IntegrationTestConfig,
    temp_dir: Path,
) -> Path:
    """
    Get the talosconfig for the test.

    Uses IT_TALOSCONFIG_PATH if set, otherwise fetches from appliance.
    """
    if config.talosconfig_path:
        return config.talosconfig_path

    tc_path = temp_dir / "talosconfig"
    run_cmd(
        [
            "curl",
            "-fsS",
            "-o",
            str(tc_path),
            f"{config.bootstrap_http}/configs/talosconfig",
        ],
        check=True,
        capture=True,
    )
    return tc_path


def bootstrap_talos(
    config: IntegrationTestConfig,
    talosconfig_path: Path,
) -> None:
    """
    Bootstrap the Talos cluster (initialize etcd).

    This must be run exactly once on the first control plane node before
    the cluster becomes healthy. Until bootstrap is run, etcd stays in
    the "Preparing" state.
    """
    env = {"TALOSCONFIG": str(talosconfig_path)}
    print(f"[info] Bootstrapping Talos cluster on {config.minipc_ip}...", flush=True)
    run_cmd(
        ["talosctl", "-n", config.minipc_ip, "bootstrap"],
        check=True,
        capture=True,
        env=env,
    )


def assert_talos_healthy(
    config: IntegrationTestConfig,
    talosconfig_path: Path,
) -> None:
    """
    Verify Talos is healthy via talosctl.

    Note: talosctl 'health' has internal retries; we use a generous timeout.
    """
    env = {"TALOSCONFIG": str(talosconfig_path)}

    # Health check
    cp = run_cmd(
        [
            "talosctl",
            "-n",
            config.minipc_ip,
            "health",
            "--wait-timeout",
            "10m",
        ],
        check=True,
        capture=True,
        env=env,
    )
    assert cp.returncode == 0, "Talos health check failed"

    # Sanity check: can query version
    run_cmd(
        ["talosctl", "-n", config.minipc_ip, "version"],
        check=True,
        capture=True,
        env=env,
    )
