"""
NAT configuration tests for the VyOS gateway.
"""


def test_source_nat_rule_exists(config_commands, assert_contains):
    assert_contains(
        config_commands,
        ["set nat source rule 100"],
        context="NAT rule",
    )


def test_masquerade_configured(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [
            "set nat source rule 100 translation address masquerade",
            f"set nat source rule 100 source address {test_topology.lab_cidr}",
        ],
        context="NAT masquerade",
    )


def test_nat_outbound_interface(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set nat source rule 100 outbound-interface name {test_topology.wan_iface}"],
        context="NAT outbound interface",
    )
