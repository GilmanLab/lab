# 08. Concepts - Storage

## Overview
Storage in the lab is divided by purpose and performance characteristics. The architecture uses a tiered approach:

| Tier | Technology | Purpose |
|:---|:---|:---|
| **High Performance** | Longhorn (NVMe) | VM disks, PersistentVolumes |
| **Bulk / Archive** | NFS (Synology NAS) | ISOs, backups, media files |

---

## Longhorn (Primary Storage)

**Longhorn** is the distributed block storage system running on Harvester. It provides replicated storage for VMs and Kubernetes PersistentVolumes.

### Characteristics

| Attribute | Value |
|:---|:---|
| **Backend** | NVMe SSDs in each MS-02 |
| **Replication** | 3 replicas (survives any single node failure) |
| **Network** | VLAN 60 (`LAB_STORAGE`) — L2 only |
| **Interface** | CSI (Container Storage Interface) |

### Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                     Harvester Cluster                          │
│                                                                │
│  ┌────────────┐     ┌────────────┐     ┌────────────┐          │
│  │  MS-02 #1  │     │  MS-02 #2  │     │  MS-02 #3  │          │
│  │  ┌──────┐  │     │  ┌──────┐  │     │  ┌──────┐  │          │
│  │  │ NVMe │  │     │  │ NVMe │  │     │  │ NVMe │  │          │
│  │  └──┬───┘  │     │  └──┬───┘  │     │  └──┬───┘  │          │
│  │     │      │     │     │      │     │     │      │          │
│  │  ┌──▼───┐  │     │  ┌──▼───┐  │     │  ┌──▼───┐  │          │
│  │  │Replica│◀──────┼──│Replica│◀──────┼──│Replica│  │          │
│  │  └──────┘  │     │  └──────┘  │     │  └──────┘  │          │
│  └────────────┘     └────────────┘     └────────────┘          │
│         │                 │                  │                 │
│         └─────────────────┴──────────────────┘                 │
│                   VLAN 60 (Storage Replication)                │
└────────────────────────────────────────────────────────────────┘
```

### Storage Classes

| Class | Replicas | Use Case |
|:---|:---|:---|
| `longhorn` (default) | 3 | Production VMs, databases |
| `longhorn-single` | 1 | Ephemeral workloads, CI runners |

### Consumption

**Harvester VMs**:
- VM root disks are Longhorn volumes
- Created automatically by KubeVirt

**Downstream Clusters (via Harvester CSI)**:
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data
spec:
  storageClassName: longhorn
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
```

---

## NFS (Bulk Storage)

The **Synology NAS** provides NFS for bulk data that doesn't require high IOPS.

### Use Cases

| Use Case | NFS Path | Consumer |
|:---|:---|:---|
| **OS Images** | `/volume1/images` | Tinkerbell HTTP server |
| **Backups** | `/volume1/backups` | Harvester VM backups, etcd snapshots |
| **Media Files** | `/volume1/media` | Plex, Jellyfin |
| **ISO Library** | `/volume1/iso` | Harvester image library |

### Architecture

```
┌─────────────────┐         ┌────────────────────────────────────┐
│  Synology NAS   │         │        Lab Clusters                │
│                 │  NFS    │                                    │
│  ┌───────────┐  │◀───────▶│  ┌────────────┐  ┌──────────────┐  │
│  │ /volume1  │  │         │  │  Platform  │  │  Downstream  │  │
│  │  /images  │  │         │  │  Cluster   │  │  Clusters    │  │
│  │  /backups │  │         │  └────────────┘  └──────────────┘  │
│  │  /media   │  │         │                                    │
│  └───────────┘  │         │  ┌────────────────────────────────┐│
└─────────────────┘         │  │         Harvester              ││
                            │  │  (VM backups, ISO library)     ││
                            │  └────────────────────────────────┘│
                            └────────────────────────────────────┘
```

### NFS Storage Class (Optional)
For workloads needing shared storage (ReadWriteMany):

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: nfs-media
provisioner: nfs.csi.k8s.io
parameters:
  server: nas.lab.local
  share: /volume1/media
```

---

## Storage Decision Matrix

| Requirement | Use Longhorn | Use NFS |
|:---|:---|:---|
| High IOPS (databases) | ✅ | ❌ |
| VM root disks | ✅ | ❌ |
| Large files (media) | ❌ | ✅ |
| Shared access (RWX) | ❌ | ✅ |
| Backup target | ❌ | ✅ |
| Ephemeral/replaceable | ✅ (single replica) | ✅ |

---

## Backup Strategy

### What Gets Backed Up

| Data | Method | Target |
|:---|:---|:---|
| **Harvester VMs** | Harvester backup feature | NFS (`/volume1/backups/harvester`) |
| **etcd (Platform)** | Scheduled Talos snapshots | NFS (`/volume1/backups/etcd`) |
| **OpenBAO** | Raft snapshots | NFS (`/volume1/backups/vault`) |
| **Application Data** | Velero / App-specific | NFS (`/volume1/backups/apps`) |

### What Doesn't Need Backup
- **Downstream cluster state**: Recreated from Git via CAPI
- **Application configs**: Stored in Git (GitOps)
- **Container images**: Pulled from registries

> [!NOTE]
> The lab follows "Reproducibility as Law" — most state can be regenerated from Git. Backups focus on **persistent application data** and **bootstrap acceleration** (avoiding full reprovision).
