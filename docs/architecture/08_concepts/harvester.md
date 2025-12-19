# 08. Concepts - Harvester HCI

## Overview
**Rancher Harvester** serves as the Hyper-Converged Infrastructure (HCI) layer for the lab, providing unified compute and storage capabilities. Unlike the Platform and Tenant clusters, Harvester is a **standalone Kubernetes cluster** managed via raw Harvester CRDs rather than Crossplane XRs.

It acts as the virtualization foundation that enables CAPI to provision downstream clusters, while also hosting special-purpose VMs that fall outside the Kubernetes orchestration model.

> [!NOTE]
> Harvester is unique in the architecture: it is both a Kubernetes cluster itself AND the infrastructure layer that hosts other Kubernetes clusters (Platform VMs, Tenant VMs).

---

## Harvester as a Managed Cluster

Harvester is registered with Argo CD as a managed cluster, but it differs from Platform and Tenant clusters in key ways:

| Aspect | Harvester | Platform/Tenant Clusters |
|:---|:---|:---|
| **Purpose** | HCI layer (VMs, storage) | Workload orchestration |
| **API Type** | Raw Harvester CRDs | Crossplane XRs |
| **VMs Provisioned Via** | VirtualMachine CRDs | CAPI (TenantCluster XR) |
| **Managed By** | Argo CD (direct) | Argo CD → Crossplane |
| **Provisioned By** | Tinkerbell (PXE bare-metal) | CAPI + Harvester provider |

### Registration Process

During bootstrap (Phase 3), Harvester is manually registered with Argo CD:

```yaml
# Applied during genesis step 12
apiVersion: v1
kind: Secret
metadata:
  name: harvester
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: cluster
    lab.gilman.io/cluster-name: harvester
type: Opaque
stringData:
  name: harvester
  server: https://harvester.lab.local:6443
  config: |
    {
      "tlsClientConfig": {
        "caData": "...",
        "certData": "...",
        "keyData": "..."
      }
    }
```

Once registered, Argo CD can sync resources to Harvester via the `cluster-apps` ApplicationSet.

---

## Architecture Position

```
┌─────────────────────────────────────────────────────────────────┐
│                    Physical Layer (Bare Metal)                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │   MS-02 #1   │  │   MS-02 #2   │  │   MS-02 #3   │           │
│  │ i9-12900H    │  │ i9-12900H    │  │ i9-12900H    │           │
│  │ 64GB RAM     │  │ 64GB RAM     │  │ 64GB RAM     │           │
│  │ 2x NVMe SSD  │  │ 2x NVMe SSD  │  │ 2x NVMe SSD  │           │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘           │
└─────────┼──────────────────┼──────────────────┼──────────────────┘
          │                  │                  │
          │    ┌─────────────┴──────────┐       │
          │    │    LACP Bond (20Gbps)  │       │
          └────┼────────────────────────┼───────┘
               │                        │
          ┌────▼────────────────────────▼────┐
          │      Harvester Cluster (HCI)     │
          │  ┌────────────────────────────┐  │
          │  │    RKE2 Kubernetes         │  │
          │  │  ┌──────────┐ ┌──────────┐ │  │
          │  │  │ KubeVirt │ │ Longhorn │ │  │
          │  │  │   (VMs)  │ │ (Storage)│ │  │
          │  │  └────┬─────┘ └──────────┘ │  │
          │  └───────┼────────────────────┘  │
          └──────────┼───────────────────────┘
                     │
     ┌───────────────┼───────────────────────────┐
     │               │                           │
     ▼               ▼                           ▼
┌─────────┐    ┌──────────────┐         ┌────────────┐
│Platform │    │Tenant Clusters│         │Standalone  │
│VMs      │    │(CAPI-managed) │         │VMs         │
│ CP-2    │    │ media, dev... │         │Gaming, NAS │
│ CP-3    │    │               │         │            │
└─────────┘    └───────────────┘         └────────────┘
```

---

## Network Configuration

Harvester uses **VLAN-aware networking** to provide native L2 connectivity to guest VMs.

### VLAN Mapping

| VLAN | Network Name | Subnet | Purpose | Harvester CRD |
|:---|:---|:---|:---|:---|
| **10** | `mgmt` | `10.10.10.0/24` | Harvester host management | `ClusterNetwork` |
| **30** | `platform` | `10.10.30.0/24` | Platform Cluster VMs | `ClusterNetwork` |
| **40** | `cluster` | `10.10.40.0/24` | Tenant Cluster VMs | `ClusterNetwork` |
| **60** | `storage` | `10.10.60.0/24` | Longhorn replication (L2 only) | `ClusterNetwork` |

### How VMs Connect

VMs attach to these networks via Harvester's `VirtualMachineImage` and `VirtualMachine` resources:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: cp-2
  namespace: default
spec:
  template:
    spec:
      networks:
        - name: platform
          multus:
            networkName: default/platform  # References ClusterNetwork
```

VMs receive IPs via:
- **DHCP (VyOS)**: For Platform and Tenant VMs (VLANs 30/40)
- **Static Assignment**: For special-purpose VMs (gaming, NAS)

> [!IMPORTANT]
> VLAN 60 (storage) is **non-routed**. It exists solely for Longhorn replication between Harvester nodes and is not accessible to VMs.

---

## VM Management

Harvester hosts three categories of VMs:

### 1. Platform Cluster VMs (Bootstrap Phase)

Created during bootstrap (Phase 3) to expand the Platform Cluster from single-node (UM760) to 3-node HA.

| VM | Purpose | Created Via | Lifecycle |
|:---|:---|:---|:---|
| **CP-2** | Platform control plane node 2 | Raw `VirtualMachine` CRD | Permanent |
| **CP-3** | Platform control plane node 3 | Raw `VirtualMachine` CRD | Permanent |

These VMs are defined in `clusters/harvester/vms/platform/` and synced by Argo CD.

```yaml
# clusters/harvester/vms/platform/cp-2.yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: cp-2
  namespace: default
spec:
  running: true
  template:
    spec:
      domain:
        cpu:
          cores: 4
        memory:
          guest: 16Gi
        devices:
          disks:
            - name: root
              disk:
                bus: virtio
      volumes:
        - name: root
          persistentVolumeClaim:
            claimName: cp-2-root
      networks:
        - name: platform
          multus:
            networkName: default/platform
```

> [!NOTE]
> These VMs are created **before CAPI is available** (chicken-and-egg). Once the Platform Cluster is fully operational, CAPI takes over for Tenant Clusters.

### 2. Tenant Cluster VMs (CAPI-Managed)

Provisioned dynamically via the `TenantCluster` XR, which uses CAPI's Harvester provider.

| Attribute | Value |
|:---|:---|
| **Created By** | CAPI `HarvesterMachine` resources |
| **Lifecycle** | Ephemeral (destroyed when cluster is deleted) |
| **Storage** | Longhorn-backed PVCs |
| **Network** | VLAN 40 (`cluster` network) |

These VMs **do NOT live in** `clusters/harvester/` — they are managed entirely by CAPI.

```
TenantCluster XR (clusters/media/cluster.yaml)
  ↓
CAPI Cluster + Machines
  ↓
HarvesterMachine resources
  ↓
VirtualMachine CRDs (created by CAPI provider)
  ↓
KubeVirt VMs on Harvester
```

### 3. Standalone VMs

VMs for non-containerized workloads that do not fit the Kubernetes model.

| Example | Use Case |
|:---|:---|
| **windows-gaming.yaml** | GPU passthrough gaming VM |
| **truenas.yaml** | FreeBSD-based NAS appliance |
| **vyos-test.yaml** | Router testing environment |

These live in `clusters/harvester/vms/standalone/` and are managed like Platform VMs (raw CRDs).

---

## Storage

Harvester provides two storage mechanisms:

### Longhorn (Primary)

| Feature | Value |
|:---|:---|
| **Backend** | NVMe SSDs in each MS-02 node |
| **Replication** | 3 replicas (survives 1 node failure) |
| **Network** | VLAN 60 (dedicated replication) |
| **Use Cases** | VM root disks, PVCs for guest clusters |

Longhorn is integrated with Harvester and managed automatically. Guest clusters access Longhorn via the **Harvester CSI Driver**.

### NFS (Synology NAS)

| Feature | Value |
|:---|:---|
| **Purpose** | VM image library, backups |
| **Integration** | Mounted as external storage in Harvester UI |
| **Performance** | Lower than Longhorn; suitable for cold data |

Harvester can import VM images (e.g., Talos ISO) from NFS for faster deployments.

> [!TIP]
> VM backups target NFS, not Longhorn. This offloads backup I/O from the performance-critical NVMe pool.

---

## Integration with Platform

Harvester integrates with the Platform Cluster in multiple ways:

### 1. CAPI Infrastructure Provider

The Platform Cluster runs the **CAPI Harvester Provider**, which:
- Creates `VirtualMachine` resources on Harvester
- Provisions Longhorn-backed PVCs for VM disks
- Attaches VMs to the correct VLAN network

### 2. Harvester Cloud Controller Manager (CCM)

Guest clusters (Platform and Tenant) run the **Harvester CCM**, which provides:
- **CSI Storage**: PersistentVolumes backed by Longhorn
- **LoadBalancer**: ❌ **Not used** — Cilium BGP handles this (See [ADR 001](../09_design_decisions/001_use_bgp_loadbalancing.md))

### 3. Bootstrap Dependency

The Platform Cluster has a **bootstrapping dependency** on Harvester:

```
┌──────────────┐       ┌──────────────┐       ┌──────────────┐
│  Phase 2:    │       │  Phase 3:    │       │  Phase 4:    │
│  Single-Node │  ───▶ │  Harvester   │  ───▶ │  3-Node HA   │
│  Platform    │       │  Provisioned │       │  Platform    │
│  (UM760)     │       │  (MS-02s)    │       │  (+CP-2,CP-3)│
└──────────────┘       └──────────────┘       └──────────────┘
       │                      │                      ▲
       │ Tinkerbell           │ Argo CD syncs        │
       │ PXE boots            │ VirtualMachine CRDs  │
       └──────────────────────┴──────────────────────┘
```

The Platform Cluster starts on bare-metal (UM760), provisions Harvester via Tinkerbell, then uses Harvester to create its own additional control plane nodes (CP-2, CP-3).

---

## What Lives in `clusters/harvester/`

```
clusters/harvester/
├── config/
│   ├── networks/
│   │   ├── mgmt.yaml         # ClusterNetwork for VLAN 10
│   │   ├── platform.yaml     # ClusterNetwork for VLAN 30
│   │   ├── cluster.yaml      # ClusterNetwork for VLAN 40
│   │   └── storage.yaml      # ClusterNetwork for VLAN 60
│   └── images/
│       └── talos-1.9.yaml    # VirtualMachineImage (Talos ISO)
└── vms/
    ├── platform/
    │   ├── cp-2.yaml         # Platform control plane VM
    │   └── cp-3.yaml         # Platform control plane VM
    └── standalone/
        └── .gitkeep          # (gaming, NAS VMs when created)
```

### Why Raw CRDs?

Harvester has its own native API (`kubevirt.io`, `harvesterhci.io`). We use raw CRDs because:
1. **No abstraction needed**: Harvester's API is already high-level
2. **CAPI handles tenant VMs**: No need for Crossplane XRs
3. **Simplicity**: Direct CRD management is clearer than XR indirection

---

## Operational Notes

### Maintenance

| Task | Procedure |
|:---|:---|
| **Node Drain** | Harvester live-migrates VMs automatically |
| **Upgrades** | Performed via Harvester UI, rolling upgrade |
| **Backups** | Harvester VM backups → NFS (Synology NAS) |

### Access

| Interface | URL | Auth |
|:---|:---|:---|
| **Harvester UI** | `https://harvester.lab.local` | Local admin credentials |
| **Kubernetes API** | `https://harvester.lab.local:6443` | Kubeconfig (used by CAPI) |

### Monitoring

Harvester metrics are scraped by the Platform Cluster's Prometheus:

| Metric | Purpose |
|:---|:---|
| `harvester_vm_status` | VM running/stopped state |
| `longhorn_volume_actual_size_bytes` | Storage usage per volume |
| `longhorn_node_status` | Longhorn node health |
| `kubevirt_vmi_phase_count` | VM lifecycle phases |

> [!IMPORTANT]
> Harvester is a **critical dependency**. If Harvester is down, Tenant Clusters cannot be provisioned, but existing VMs continue to run (KubeVirt failover).

---

## Disaster Recovery

In a Harvester failure scenario:

1. **Single Node Failure**: VMs live-migrate to surviving nodes, Longhorn replicates data
2. **Quorum Loss**: Harvester becomes read-only; manual intervention required
3. **Full Rebuild**: Reprovision Harvester via Tinkerbell, restore VM definitions from Git

> [!TIP]
> `clusters/harvester/` is version-controlled. All VM definitions can be reapplied via Argo CD after Harvester is restored.
