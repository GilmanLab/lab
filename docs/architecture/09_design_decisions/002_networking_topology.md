# ADR 002: Networking Topology - Bonding vs. Segregation

**Status**: Accepted
**Date**: 2025-12-18


## Context
The MS-02 nodes have 2x 25GbE capable SFP+ ports (downgraded to 10GbE due to switch limitations).
We need to decide how to utilize these two physical links for Data Plane traffic (Workload + Storage).

## Options

### Option A: LACP Bonding (802.3ad)
Combine both links into a single logical `bond0` interface (20Gbps).
*   **Pros**:
    *   **Simplicity**: One logical interface to manage in Harvester/OS.
    *   **Redundancy**: If one cable/port fails, traffic continues (at reduced speed).
    *   **Burst**: Single flows are capped at 10G, but aggregate traffic can hit 20G.
*   **Cons**:
    *   **Shared Pipe**: Heavy storage replication could theoretically choke workload traffic (though 20Gbps is a high ceiling).

### Option B: Physical Segregation
Dedicate `eth0` to "Cluster Traffic" and `eth1` to "Storage Traffic".
*   **Pros**:
    *   **QoS**: Guaranteed 10Gbps dedicated pipe for Longhorn. Storage storms cannot impact workloads.
*   **Cons**:
    *   **Complexity**: Requires careful VLAN mapping and Harvester Network config.
    *   **No Failover**: If the storage cable dies, storage dies (and the node crashes).
    *   **Waste**: If storage is idle, that 10Gbps bandwidth sits unused.

## Decision
**Use Option A: LACP Bonding.**

## Rationale
1.  **Alignment with Constraints**: "Simplicity > Complexity" (Bus Factor of 1). Bonding is "set and forget".
2.  **Redundancy**: In a physical lab, loose cables or bad transceivers are real failures. Losing an entire node because one cable failed (Option B) is unacceptable.
3.  **Performance**: 20Gbps is ample headroom for a 3-node homelab. Contentions are unlikely to be the bottleneck before CPU/Disk IO.
