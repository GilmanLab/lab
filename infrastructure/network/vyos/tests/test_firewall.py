"""
Firewall configuration tests for the VyOS gateway.
"""


def test_firewall_groups_exist(config_commands, assert_contains):
    assert_contains(
        config_commands,
        [
            "set firewall group network-group HOME_NETWORK",
            "set firewall group network-group LAB_NETWORKS",
            "set firewall group network-group RFC1918",
        ],
        context="firewall groups",
    )


def test_home_network_group_content(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set firewall group network-group HOME_NETWORK network {test_topology.home_cidr}"],
        context="HOME_NETWORK group",
    )


def test_lab_networks_group_content(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [f"set firewall group network-group LAB_NETWORKS network {test_topology.lab_cidr}"],
        context="LAB_NETWORKS group",
    )


def test_rfc1918_group_content(config_commands, assert_contains):
    assert_contains(
        config_commands,
        [
            "set firewall group network-group RFC1918 network 10.0.0.0/8",
            "set firewall group network-group RFC1918 network 172.16.0.0/12",
            "set firewall group network-group RFC1918 network 192.168.0.0/16",
        ],
        context="RFC1918 group",
    )


def test_firewall_interface_binding(config_commands, test_topology, assert_contains):
    assert_contains(
        config_commands,
        [
            f"set firewall interface {test_topology.wan_iface} in name WAN_TO_LAB",
            f"set firewall interface {test_topology.wan_iface} local name LOCAL",
            f"set firewall interface {test_topology.wan_iface} out name LAB_TO_WAN",
        ],
        context="firewall interface binding",
    )


def test_wan_to_lab_rules(config_commands, assert_contains):
    assert_contains(
        config_commands,
        [
            "set firewall ipv4 name WAN_TO_LAB default-action drop",
            "set firewall ipv4 name WAN_TO_LAB rule 10 state established",
            "set firewall ipv4 name WAN_TO_LAB rule 10 state related",
            "set firewall ipv4 name WAN_TO_LAB rule 20 source group network-group HOME_NETWORK",
        ],
        context="WAN_TO_LAB rules",
    )


def test_lab_to_wan_rules(config_commands, assert_contains):
    assert_contains(
        config_commands,
        [
            "set firewall ipv4 name LAB_TO_WAN default-action accept",
            "set firewall ipv4 name LAB_TO_WAN rule 10 state established",
            "set firewall ipv4 name LAB_TO_WAN rule 10 state related",
            "set firewall ipv4 name LAB_TO_WAN rule 20 destination group network-group HOME_NETWORK",
        ],
        context="LAB_TO_WAN rules",
    )


def test_local_firewall_rules(config_commands, assert_contains):
    assert_contains(
        config_commands,
        [
            "set firewall ipv4 name LOCAL default-action drop",
            "set firewall ipv4 name LOCAL rule 30 destination port 22",
            "set firewall ipv4 name LOCAL rule 40 destination port 53",
            "set firewall ipv4 name LOCAL rule 50 destination port 67",
            "set firewall ipv4 name LOCAL rule 60 destination port 179",
        ],
        context="LOCAL rules",
    )
