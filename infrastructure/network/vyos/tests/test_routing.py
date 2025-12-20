"""
Routing configuration tests for the VyOS gateway.
"""


def test_default_route_configured(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [
            "set protocols static route 0.0.0.0/0",
            f"set protocols static route 0.0.0.0/0 next-hop {test_topology.wan_gateway}",
        ],
        context="default route",
    )
