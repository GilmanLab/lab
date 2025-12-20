"""
Service configuration tests for the VyOS gateway.
"""


def test_dhcp_server_configured(config_commands, assert_contains):
    assert_contains(
        config_commands,
        ["set service dhcp-server", "set service dhcp-server shared-network-name LAB_MGMT"],
        context="DHCP server config",
    )


def test_dhcp_range_configured(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [test_topology.dhcp_range_start, test_topology.dhcp_range_stop],
        context="DHCP range",
    )


def test_dns_forwarding_configured(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        ["set service dns forwarding", f"set service dns forwarding allow-from {test_topology.lab_cidr}"],
        context="DNS forwarding",
    )


def test_dns_listen_addresses(config_commands, test_topology, assert_contains):
    expected = [
        f"set service dns forwarding listen-address {address}"
        for address in test_topology.dns_listen_addresses
    ]
    assert_contains(config_commands, expected, context="DNS listen addresses")


def test_ssh_service_enabled(config_commands, assert_contains):
    assert_contains(
        config_commands,
        ["set service ssh", "set service ssh port 22"],
        context="SSH service",
    )
