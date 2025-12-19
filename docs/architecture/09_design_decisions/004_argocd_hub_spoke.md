# ADR 004: Argo CD Hub-and-Spoke Strategy

**Status**: Accepted
**Date**: 2025-12-19

## Context
The lab architecture includes multiple Kubernetes clusters that need declarative management:
*   **Platform Cluster**: Hosts shared services (Crossplane, CAPI, Argo CD, identity, observability)
*   **Harvester Cluster**: HCI layer for VM management and storage
*   **Tenant Clusters**: Application workload clusters (media, dev, prod)

We need a multi-cluster GitOps strategy that enables centralized management while maintaining simplicity for a single-administrator homelab environment.

## Options

### Option A: Hub-and-Spoke (Direct Apply)
A single Argo CD instance on the platform cluster directly manages all clusters using kubeconfig-based cluster Secrets.
*   **Mechanism**: Argo CD maintains cluster Secrets for each managed cluster. Applications sync directly to target clusters via kubeconfig credentials.
*   **Pros**:
    *   **Single pane of glass**: One Argo CD UI for all clusters.
    *   **Simple architecture**: No additional components or agents.
    *   **Network accessibility**: All clusters on same lab network with direct connectivity.
    *   **CAPI integration**: CAPI already generates kubeconfigs for tenant clusters.
    *   **Mature pattern**: Well-documented, battle-tested approach.
*   **Cons**:
    *   **Credential management**: Hub must store and rotate cluster credentials.
    *   **Hub as single point of failure**: If platform cluster fails, no cluster management.

### Option B: Agent per Cluster (Pull Model)
Deploy Argo CD ApplicationSet Controller or Argo CD Agent on each managed cluster to pull configurations.
*   **Mechanism**: Each cluster runs an agent that pulls from Git and applies locally.
*   **Pros**:
    *   **Reduced hub privileges**: Hub doesn't need cluster admin credentials.
    *   **Works across NAT/firewalls**: Pull model doesn't require inbound connectivity.
*   **Cons**:
    *   **Complexity**: Additional component on every cluster.
    *   **Fragmented visibility**: Harder to get unified view of all clusters.
    *   **Unnecessary for lab**: No NAT/firewall restrictions in homelab network.

### Option C: Federated (Separate Instances)
Run independent Argo CD instances on each cluster managing only themselves.
*   **Mechanism**: Each cluster has its own Argo CD managing its own resources.
*   **Pros**:
    *   **Full isolation**: Cluster failures don't affect others.
    *   **No credential sharing**: Each Argo CD uses in-cluster access.
*   **Cons**:
    *   **Operational overhead**: Multiple Argo CD instances to maintain.
    *   **No centralized view**: Must access each cluster separately.
    *   **Redundant for lab**: Overkill for single-administrator environment.

## Decision
**Use Option A: Hub-and-Spoke.**

## Rationale
1.  **Simplicity**: Single Argo CD instance provides unified management interface aligned with homelab operational model.
2.  **Network Topology**: All clusters on flat lab network (VLANs with routing) with no NAT or firewall restrictions.
3.  **CAPI Integration**: TenantCluster XR composition already generates kubeconfigs; automatically creating Argo CD cluster Secrets leverages existing infrastructure.
4.  **Automatic Registration**: Crossplane compositions create Argo CD cluster Secrets when tenant clusters are provisioned, enabling zero-touch cluster onboarding.
5.  **Bus Factor**: Single administrator makes distributed model unnecessary; centralized management reduces complexity.
6.  **Maturity**: Hub-and-spoke is the most widely deployed pattern with extensive documentation and tooling support.
