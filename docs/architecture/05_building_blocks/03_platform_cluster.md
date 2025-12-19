# 05. Building Block: Platform Cluster

## Overview
The **Platform Cluster** is the permanent, privileged Kubernetes cluster that acts as the "Brain" of the lab. It hosts the control plane services that manage all other infrastructure — from provisioning bare metal to spawning downstream Kubernetes clusters.

> This cluster is the **single point of orchestration**. If it fails, new clusters cannot be created, but existing downstream clusters continue to operate independently.

## Role in the Architecture

| Function | Description |
|:---|:---|
| **Cluster Factory** | Runs Cluster API (CAPI) and **Crossplane** to provision and lifecycle-manage downstream clusters |
| **GitOps Engine** | Hosts Argo CD, the source of truth for all declarative configurations |
| **Identity Provider** | Runs Zitadel for centralized authentication (OIDC) |
| **Secrets Management** | Runs OpenBAO (Vault fork) for secrets, PKI, and dynamic credentials |
| **Provisioning** | Hosts Tinkerbell after the Genesis pivot (for disaster recovery re-provisioning) |

## Cluster Topology

The Platform Cluster is a **3-node Talos** cluster with a hybrid physical/virtual topology:

| Node | Type | Hardware | Purpose |
|:---|:---|:---|:---|
| **platform-cp-1** | Physical | UM760 (Ryzen 7, 32GB) | Permanent control plane anchor |
| **platform-cp-2** | VM | Harvester (4 vCPU, 8GB) | HA control plane member |
| **platform-cp-3** | VM | Harvester (4 vCPU, 8GB) | HA control plane member |

### Why Hybrid?
- **UM760 (Physical)**: Survives Harvester failures. If Harvester is down, the Platform Cluster retains quorum (1 of 3) and can orchestrate recovery
- **Harvester VMs**: Provide HA without requiring additional physical hardware

```
┌─────────────────────────────────────────────────────────────────┐
│                      Platform Cluster                           │
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │  platform-cp-1  │  │  platform-cp-2  │  │  platform-cp-3  │  │
│  │    (UM760)      │  │  (Harvester VM) │  │  (Harvester VM) │  │
│  │   [Physical]    │  │    [Virtual]    │  │    [Virtual]    │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│            │                   │                   │            │
│            └───────────────────┴───────────────────┘            │
│                         etcd quorum                             │
└─────────────────────────────────────────────────────────────────┘
```

## Networking

The Platform Cluster operates on **VLAN 30 (LAB_PLATFORM)**:

| Attribute | Value |
|:---|:---|
| **Subnet** | `10.10.30.0/24` |
| **Gateway** | `10.10.30.1` (VyOS) |
| **DHCP** | VyOS (with static reservations for control plane nodes) |
| **CNI** | Cilium (with BGP peering to VyOS) |
| **Service VIPs** | Allocated from `10.10.50.0/24` (VLAN 50) via BGP |

### UM760 Network Configuration
The UM760 uses a **Hybrid Trunk** on its single 2.5GbE NIC:
- **Native VLAN 20**: For PXE boot during initial provisioning
- **Tagged VLAN 30**: For Platform Cluster traffic post-bootstrap

Talos configures VLAN sub-interfaces to separate traffic logically.

## Hosted Services

### Core Platform Services

| Service | Purpose | Access |
|:---|:---|:---|
| **Argo CD** | GitOps continuous delivery | `https://argocd.lab.local` |
| **Zitadel** | Identity provider (OIDC/SAML) | `https://idm.lab.local` |
| **OpenBAO** | Secrets, PKI, dynamic credentials | `https://vault.lab.local` |

### Infrastructure Controllers

| Controller | Purpose |
|:---|:---|
| **Cluster API (CAPI)** | Declarative cluster lifecycle management |
| **CAPI Harvester Provider** | Creates VMs on Harvester for downstream clusters |
| **CAPI Talos Provider** | Bootstraps Talos on provisioned VMs |
| **Crossplane** | Hosts the `TenantCluster` XRD to abstract CAPI operations |
| **Tinkerbell** | Bare metal provisioning (post-Genesis) |

## CAPI Integration

The Platform Cluster is the **Management Cluster** in CAPI terminology. It holds:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Platform Cluster (CAPI)                      │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ CRDs: Cluster, Machine, HarvesterMachineTemplate, etc.    │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ CAPI Core       │  │ Harvester Infra │  │ Talos Bootstrap │  │
│  │ Controller      │  │ Provider        │  │ Provider        │  │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘  │
│           │                    │                    │           │
└───────────┼────────────────────┼────────────────────┼───────────┘
            │                    │                    │
            ▼                    ▼                    ▼
      Reconcile            Create VMs on         Bootstrap Talos
      Cluster state        Harvester             on new VMs
```

### Workflow: Creating a Downstream Cluster
1. **GitOps Trigger**: Argo CD syncs a new `Cluster` manifest from Git
2. **CAPI Reconciles**: Core controller creates `Machine` objects
3. **Harvester Provider**: Provisions VMs on Harvester HCI
4. **Talos Provider**: Generates machine configs, bootstraps Talos
5. **Cluster Ready**: Downstream cluster API becomes available
6. **Argo CD (Downstream)**: Syncs workloads to the new cluster

## Operational Notes

### High Availability
- **etcd**: Distributed across 3 nodes (quorum = 2)
- **Control Plane**: All 3 nodes run API server, controller-manager, scheduler
- **Failure Tolerance**: Survives loss of 1 node (including total Harvester failure if UM760 remains)

### Backup Strategy
- **etcd Snapshots**: Scheduled backups to NFS (Synology NAS)
- **Argo CD State**: Stored in Git (inherently backed up)
- **Secrets**: OpenBAO uses Raft storage with snapshots to NFS

### Access
- **Kubernetes API**: `https://platform.lab.local:6443`
- **kubeconfig**: Generated via Talos `talosctl kubeconfig` command
