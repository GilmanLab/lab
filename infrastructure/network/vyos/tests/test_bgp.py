"""
BGP configuration tests for the VyOS gateway.
"""


def test_bgp_local_as(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set protocols bgp system-as {test_topology.bgp_local_as}"],
        context="BGP local AS",
    )


def test_bgp_router_id(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set protocols bgp parameters router-id {test_topology.bgp_router_id}"],
        context="BGP router ID",
    )


def test_bgp_neighbors_configured(config_commands, test_topology, assert_contains):
    expected = [
        f"set protocols bgp neighbor {neighbor} remote-as {test_topology.bgp_remote_as}"
        for neighbor in test_topology.bgp_neighbors
    ]
    assert_contains(config_commands, expected, context="BGP neighbors")


def test_bgp_neighbors_shutdown(config_commands, test_topology, assert_contains):
    expected = [
        f"set protocols bgp neighbor {neighbor} shutdown"
        for neighbor in test_topology.bgp_neighbors
    ]
    assert_contains(config_commands, expected, context="BGP neighbor shutdown")


def test_bgp_network_advertisement(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [
            "set protocols bgp address-family ipv4-unicast",
            f"set protocols bgp address-family ipv4-unicast network {test_topology.bgp_service_network}",
        ],
        context="BGP network advertisement",
    )
