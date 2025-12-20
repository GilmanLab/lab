"""
System configuration tests for the VyOS gateway.
"""


def test_hostname_configured(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set system host-name {test_topology.hostname}"],
        context="hostname",
    )


def test_domain_name_configured(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set system domain-name {test_topology.domain_name}"],
        context="domain name",
    )


def test_name_servers_configured(config_commands, test_topology, assert_contains):
    expected = [
        f"set system name-server {server}" for server in test_topology.name_servers
    ]
    assert_contains(config_commands, expected, context="name servers")


def test_timezone_configured(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set system time-zone {test_topology.time_zone}"],
        context="time zone",
    )
