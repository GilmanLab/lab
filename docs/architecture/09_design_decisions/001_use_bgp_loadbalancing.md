# ADR 001: Load Balancing Strategy - BGP vs. Layer 2

**Status**: Accepted
**Date**: 2025-12-18

## Context
Kubernetes clusters in the lab need to expose services (Ingress Controllers, API Servers) to the wider network on stable IPs ("VIPs"). We need a mechanism to announce these IPs from the nodes to the gateway.

## Options

### Option A: Layer 2 (ARP/NDP)
Use MetalLB (or Cilium) in Layer 2 mode.
*   **Mechanism**: One node is elected "Leader" and responds to ARP requests for the VIP.
*   **Pros**: Works on dumb switches. Zero router config required.
*   **Cons**:
    *   **Bottleneck**: All traffic for a VIP goes through one node.
    *   **Slow Failover**: Relies on K8s Leader Election + ARP cache expiry (seconds to tens of seconds).

### Option B: BGP (Border Gateway Protocol)
Use Cilium (or MetalLB) to peer with the Upstream Gateway (VyOS).
*   **Mechanism**: All nodes announce `/32` routes for the VIPs to VyOS. VyOS uses ECMP (Equal-Cost Multi-Path) to distribute traffic.
*   **Pros**:
    *   **True Load Balancing**: Traffic is spread across all healthy nodes.
    *   **Instant Failover**: BGP session teardown removes routes immediately.
    *   **Scalability**: Works across subnets (L3).
*   **Cons**:
    *   **Complexity**: Requires configuring BGP on VyOS and the K8s CNI.

## Decision
**Use Option B: BGP.**

## Rationale
1.  **Capability**: We have a BGP-capable gateway (VyOS on VP6630) and a sophisticated CNI (Cilium).
2.  **Performance**: "Platform Engineering" implies production-grade. BGP is the production standard; L2 is a concession for poor hardware.
3.  **Resilience**: The instant failover of BGP aligns with our "Resilience" goal.
