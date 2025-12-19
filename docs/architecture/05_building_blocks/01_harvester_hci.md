# 05. Building Block: Harvester HCI

## Overview
**Rancher Harvester** serves as the Hyper-Converged Infrastructure (HCI) layer, providing unified Compute and Storage on top of bare-metal hardware. It abstracts the physical MS-02 nodes into a flexible virtualization platform that Cluster API (CAPI) consumes to provision Kubernetes clusters.

## Architecture

### Cluster Topology
| Attribute | Value |
|:---|:---|
| **Node Count** | 3 (Full HA) |
| **Hardware** | Minisforum MS-02 (i9-12900H, 64GB RAM) |
| **Rancher Mode** | Embedded (built-in UI and management) |
| **Hypervisor** | KubeVirt (Kubernetes-native VMs) |
| **Storage Backend** | Longhorn (replicated block storage) |

### Component Stack
```
┌─────────────────────────────────────────────────┐
│                   Harvester UI                  │
│               (Embedded Rancher)                │
├─────────────────────────────────────────────────┤
│     KubeVirt (VMs)     │     Longhorn (PVs)     │
├─────────────────────────────────────────────────┤
│              Kubernetes (RKE2)                  │
├─────────────────────────────────────────────────┤
│           Elemental OS (Immutable)              │
├─────────────────────────────────────────────────┤
│       MS-02 Hardware (x3, LACP Bonded)          │
└─────────────────────────────────────────────────┘
```

## Networking

### Physical Connectivity
Each MS-02 node uses the **Split Plane** topology defined in [ADR 002](../09_design_decisions/002_networking_topology.md):
- **bond0** (2x 10GbE LACP): Data Plane — VM traffic, storage replication, Harvester management
- **enp* 2.5GbE**: OOB Plane — vPro/AMT, PXE boot, Tinkerbell provisioning

### VM Networks (VLAN Bridge)
Harvester uses **VLAN-aware bridges** to provide native L2 connectivity to guest VMs:

| Network Name | VLAN | Subnet | Purpose |
|:---|:---|:---|:---|
| `mgmt` | 10 | `10.10.10.0/24` | Harvester host management |
| `platform` | 30 | `10.10.30.0/24` | Platform Cluster VMs |
| `cluster` | 40 | `10.10.40.0/24` | Downstream Cluster VMs |
| `storage` | 60 | `10.10.60.0/24` | Longhorn replication (L2 only) |

VMs attach to these networks and receive IPs from VyOS DHCP (VLANs 30/40) or static assignment.

## Storage

### Longhorn (Primary)
- **Role**: VM root disks and PersistentVolumes
- **Replica Count**: 3 (data survives any single node failure)
- **Performance**: Leverages NVMe SSDs in each MS-02

### NFS (Synology NAS)
- **Role**: ISO library, VM image templates, backup targets
- **Integration**: Mounted as external storage in Harvester for image management

> [!NOTE]
> VM backups target NFS, not Longhorn. This offloads backup storage from the cluster's performance-critical NVMe pool.

## CAPI Integration

Harvester integrates with Cluster API to act as an **Infrastructure Provider** for downstream Talos clusters.

### Components
| Component | Runs On | Purpose |
|:---|:---|:---|
| **CAPI Harvester Provider** | Platform Cluster | Creates/deletes VMs on Harvester via API |
| **Harvester CCM** | Guest Clusters | Provides CSI storage (PVs backed by Longhorn) |

### What We Use vs. Skip
| Feature | Status | Notes |
|:---|:---|:---|
| **CSI Storage** | ✅ Used | PersistentVolumes for stateful workloads |
| **LoadBalancer** | ❌ Skipped | Cilium BGP handles this (see [ADR 001](../09_design_decisions/001_use_bgp_loadbalancing.md)) |

## Operational Notes

### Maintenance Strategy
- **Node Drain**: Harvester supports live migration. VMs are moved before node maintenance.
- **Upgrades**: Performed via the Harvester UI. Nodes are upgraded sequentially (rolling).
- **Failure**: Loss of 1 node is tolerated (HA quorum intact, Longhorn replicas available).

### Access
- **UI**: `https://harvester.lab.local` (via Ingress on VLAN 10)
- **API**: Kubernetes API exposed for CAPI communication
