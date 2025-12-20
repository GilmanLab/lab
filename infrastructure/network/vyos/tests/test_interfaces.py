"""
Interface configuration tests for the VyOS gateway.
"""


def test_interface_addresses(config_commands, test_topology, assert_contains):
    expected = [
        f"set interfaces ethernet {test_topology.wan_iface} address {test_topology.wan_ip}",
        f"set interfaces ethernet {test_topology.trunk_iface} vif {test_topology.mgmt_vif} address {test_topology.mgmt_ip}",
        f"set interfaces ethernet {test_topology.trunk_iface} vif {test_topology.prov_vif} address {test_topology.prov_ip}",
        f"set interfaces ethernet {test_topology.trunk_iface} vif {test_topology.platform_vif} address {test_topology.platform_ip}",
        f"set interfaces ethernet {test_topology.trunk_iface} vif {test_topology.cluster_vif} address {test_topology.cluster_ip}",
        f"set interfaces ethernet {test_topology.trunk_iface} vif {test_topology.service_vif} address {test_topology.service_ip}",
        f"set interfaces ethernet {test_topology.trunk_iface} vif {test_topology.storage_vif} address {test_topology.storage_ip}",
    ]
    assert_contains(config_commands, expected, context="interface addresses")
