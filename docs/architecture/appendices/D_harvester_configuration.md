# Appendix D: Harvester Configuration Reference

> **Document Type**: Technical Reference
> **Date**: 2025-12-19
> **Related Concepts**: See main architecture documentation for high-level understanding

This appendix provides detailed technical reference for Harvester HCI cluster configuration, including network setup, image management, virtual machine definitions, storage configuration, and integration with Cluster API.

---

## Table of Contents

- [Network Configuration](#network-configuration)
- [Image Management](#image-management)
- [Virtual Machine Definitions](#virtual-machine-definitions)
- [Storage Configuration](#storage-configuration)
- [Integration with CAPI](#integration-with-capi)

---

## Network Configuration

Harvester uses ClusterNetwork and VlanConfig CRDs to configure VLAN-backed networks for VM connectivity. These networks map to the physical VLANs configured on the VyOS gateway.

### Network Architecture Overview

| VLAN ID | Network Name | Subnet | Purpose | Used By |
|:---:|:---|:---|:---|:---|
| 10 | `mgmt` | 10.10.10.0/24 | Management | Harvester nodes, BMC, switches |
| 30 | `platform` | 10.10.30.0/24 | Platform cluster | Platform CP-2, CP-3 VMs |
| 40 | `cluster` | 10.10.40.0/24 | Tenant clusters | CAPI-provisioned tenant cluster VMs |
| 60 | `storage` | 10.10.60.0/24 | Storage replication | Longhorn inter-node replication |

### ClusterNetwork CRDs

ClusterNetwork CRDs define the physical network configuration in Harvester.

#### Management Network (VLAN 10)

```yaml
# File: clusters/harvester/config/networks/mgmt.yaml
# Purpose: Management network for Harvester nodes and infrastructure

apiVersion: network.harvesterhci.io/v1beta1
kind: ClusterNetwork
metadata:
  name: mgmt
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
    network.harvesterhci.io/description: "Management network for infrastructure"
spec:
  description: "VLAN 10 - Management Network"

  # Enable VLAN support
  enable: true

  # Default network for Harvester management traffic
  defaultNetwork: true

---
apiVersion: network.harvesterhci.io/v1beta1
kind: VlanConfig
metadata:
  name: mgmt-vlan
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
spec:
  clusterNetwork: mgmt
  vlan: 10
  uplink:
    bondOptions:
      mode: active-backup
      miimon: 100
    linkAttributes:
      mtu: 1500
      txQueueLen: 1000
    nics:
      - enp2s0  # Primary NIC on MS-02 nodes
```

#### Platform Cluster Network (VLAN 30)

```yaml
# File: clusters/harvester/config/networks/platform.yaml
# Purpose: Network for platform cluster control plane VMs

apiVersion: network.harvesterhci.io/v1beta1
kind: ClusterNetwork
metadata:
  name: platform
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
    network.harvesterhci.io/description: "Platform cluster control plane network"
spec:
  description: "VLAN 30 - Platform Cluster Network"
  enable: true
  defaultNetwork: false

---
apiVersion: network.harvesterhci.io/v1beta1
kind: VlanConfig
metadata:
  name: platform-vlan
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
spec:
  clusterNetwork: platform
  vlan: 30
  uplink:
    bondOptions:
      mode: active-backup
      miimon: 100
    linkAttributes:
      mtu: 1500
      txQueueLen: 1000
    nics:
      - enp2s0
```

#### Tenant Cluster Network (VLAN 40)

```yaml
# File: clusters/harvester/config/networks/cluster.yaml
# Purpose: Network for CAPI-provisioned tenant cluster VMs

apiVersion: network.harvesterhci.io/v1beta1
kind: ClusterNetwork
metadata:
  name: cluster
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
    network.harvesterhci.io/description: "Tenant cluster VM network"
spec:
  description: "VLAN 40 - Tenant Cluster Network"
  enable: true
  defaultNetwork: false

---
apiVersion: network.harvesterhci.io/v1beta1
kind: VlanConfig
metadata:
  name: cluster-vlan
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
spec:
  clusterNetwork: cluster
  vlan: 40
  uplink:
    bondOptions:
      mode: active-backup
      miimon: 100
    linkAttributes:
      mtu: 9000  # Jumbo frames for better performance
      txQueueLen: 1000
    nics:
      - enp2s0
```

#### Storage Replication Network (VLAN 60)

```yaml
# File: clusters/harvester/config/networks/storage.yaml
# Purpose: Dedicated network for Longhorn storage replication

apiVersion: network.harvesterhci.io/v1beta1
kind: ClusterNetwork
metadata:
  name: storage
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
    network.harvesterhci.io/description: "Longhorn storage replication network"
spec:
  description: "VLAN 60 - Storage Replication Network"
  enable: true
  defaultNetwork: false

---
apiVersion: network.harvesterhci.io/v1beta1
kind: VlanConfig
metadata:
  name: storage-vlan
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
spec:
  clusterNetwork: storage
  vlan: 60
  uplink:
    bondOptions:
      mode: active-backup
      miimon: 100
    linkAttributes:
      mtu: 9000  # Jumbo frames for storage traffic
      txQueueLen: 1000
    nics:
      - enp3s0  # Dedicated NIC for storage (if available)
```

### VLAN to VM Network Mapping

When creating VirtualMachine CRDs, networks are referenced by ClusterNetwork name:

| VM Type | Primary Network | Secondary Networks | IP Assignment |
|:---|:---|:---|:---|
| Harvester nodes | `mgmt` (VLAN 10) | `storage` (VLAN 60) | DHCP (management), Static (storage) |
| Platform CP-2, CP-3 | `platform` (VLAN 30) | - | Static via cloud-init |
| Tenant cluster VMs (CAPI) | `cluster` (VLAN 40) | - | Static via CAPI |
| Standalone VMs | `mgmt` (VLAN 10) | As needed | DHCP or Static |

---

## Image Management

Harvester uses VirtualMachineImage CRDs to manage VM disk images. These images can be imported from URLs, uploaded directly, or built from ISOs.

### Talos VM Image

The Talos image is used for both platform cluster VMs and CAPI-provisioned tenant cluster VMs.

```yaml
# File: clusters/harvester/config/images/talos-1.9.yaml
# Purpose: Talos Linux disk image for Kubernetes nodes

apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineImage
metadata:
  name: talos-1.9.0
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "-2"
    harvesterhci.io/image-type: "os"
  labels:
    harvesterhci.io/os: linux
    harvesterhci.io/os-version: talos-1.9.0
spec:
  displayName: "Talos Linux 1.9.0"
  description: "Talos Linux v1.9.0 - Immutable Kubernetes OS"

  # Source URL for image download
  sourceType: download
  url: "https://github.com/siderolabs/talos/releases/download/v1.9.0/nocloud-amd64.raw.xz"

  # Image will be stored in Longhorn
  storageClassName: longhorn

  # Optional: Checksum verification
  checksum: "sha256:a1b2c3d4e5f6..."

  # PVC settings
  pvcName: talos-1.9.0-image
  pvcNamespace: default

  # Size will be determined from image
  # (Talos images are typically ~150MB compressed, ~1GB uncompressed)
```

**Image Download Process:**
1. Harvester creates a PVC with the specified StorageClass
2. Harvester launches an importer Pod to download the image
3. Image is decompressed and written to PVC
4. PVC becomes available as a disk image for VMs
5. VMs can clone this image for their root disks

### Additional Image Examples

**Ubuntu Cloud Image (for standalone VMs):**

```yaml
# File: clusters/harvester/config/images/ubuntu-22.04.yaml
# Purpose: Ubuntu Server cloud image for general-purpose VMs

apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineImage
metadata:
  name: ubuntu-22.04
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "-2"
  labels:
    harvesterhci.io/os: linux
    harvesterhci.io/os-version: ubuntu-22.04
spec:
  displayName: "Ubuntu 22.04 LTS"
  description: "Ubuntu Server 22.04 LTS (Jammy Jellyfish)"
  sourceType: download
  url: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"
  storageClassName: longhorn
  pvcName: ubuntu-22.04-image
  pvcNamespace: default
```

**Windows Server (for standalone VMs):**

```yaml
# File: clusters/harvester/config/images/windows-server-2022.yaml
# Purpose: Windows Server image (requires manual upload)

apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineImage
metadata:
  name: windows-server-2022
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "-2"
  labels:
    harvesterhci.io/os: windows
    harvesterhci.io/os-version: windows-server-2022
spec:
  displayName: "Windows Server 2022"
  description: "Windows Server 2022 Datacenter"

  # Upload type requires manual web UI upload or PVC creation
  sourceType: upload
  storageClassName: longhorn
  pvcName: windows-server-2022-image
  pvcNamespace: default
```

---

## Virtual Machine Definitions

Harvester uses KubeVirt VirtualMachine CRDs to define virtual machines. These VMs are managed directly via Argo CD for platform bootstrap and standalone workloads.

### VM Categories in Harvester

| Directory | Purpose | Lifecycle | Provisioned By |
|:---|:---|:---|:---|
| `vms/platform/` | Platform cluster CP-2, CP-3 nodes | Bootstrap only, deleted after CAPI migration | Argo CD → Harvester |
| `vms/standalone/` | Non-containerized workloads | Permanent | Argo CD → Harvester |
| (Not in repo) | Tenant cluster VMs | Dynamic, CAPI-managed | Crossplane XR → CAPI → Harvester |

### Platform CP-2 VM Definition

This VM is created during bootstrap phase 3 to expand the platform cluster from single-node (UM760) to multi-node HA.

```yaml
# File: clusters/harvester/vms/platform/cp-2.yaml
# Purpose: Platform cluster control plane node 2 (Harvester VM)

apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: platform-cp-2
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "0"
    harvesterhci.io/description: "Platform cluster control plane node 2"
  labels:
    harvesterhci.io/cluster: platform
    harvesterhci.io/node-role: control-plane
    harvesterhci.io/os: talos
spec:
  running: true

  template:
    metadata:
      labels:
        kubevirt.io/vm: platform-cp-2
        harvesterhci.io/cluster: platform
        harvesterhci.io/node-role: control-plane

    spec:
      # Node affinity - prefer spreading across Harvester nodes
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchExpressions:
                    - key: harvesterhci.io/cluster
                      operator: In
                      values:
                        - platform
                topologyKey: kubernetes.io/hostname

      # Resource allocation
      domain:
        cpu:
          cores: 4
          sockets: 1
          threads: 1

        memory:
          guest: 16Gi

        # Devices
        devices:
          disks:
            - name: rootdisk
              bootOrder: 1
              disk:
                bus: virtio
            - name: cloudinitdisk
              disk:
                bus: virtio

          interfaces:
            - name: default
              bridge: {}
              macAddress: "52:54:00:30:00:02"  # Static MAC for consistent IP

        # Resource overcommit settings
        resources:
          requests:
            memory: 16Gi
            cpu: "4"
          limits:
            memory: 16Gi
            cpu: "4"

        # Machine type
        machine:
          type: q35

        # Features
        features:
          acpi:
            enabled: true
          smm:
            enabled: true

        # Firmware
        firmware:
          bootloader:
            efi:
              secureBoot: false

      # Network configuration
      networks:
        - name: default
          multus:
            networkName: default/platform  # References ClusterNetwork 'platform'

      # Volumes
      volumes:
        - name: rootdisk
          persistentVolumeClaim:
            claimName: platform-cp-2-rootdisk

        - name: cloudinitdisk
          cloudInitNoCloud:
            networkData: |
              version: 2
              ethernets:
                eth0:
                  addresses:
                    - 10.10.30.12/24
                  gateway4: 10.10.30.1
                  nameservers:
                    addresses:
                      - 10.10.10.1
                      - 1.1.1.1

            # Talos machine configuration URL
            # Served by Tinkerbell config-server or NGINX
            userData: |
              #cloud-config
              talos:
                config_url: http://10.10.10.5:8080/talos/platform-cp-2.yaml

---
# PVC for root disk (cloned from Talos image)
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: platform-cp-2-rootdisk
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "-1"  # Create before VM
    harvesterhci.io/imageId: default/talos-1.9.0  # Clone from image
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 50Gi
  storageClassName: longhorn
  volumeMode: Block
```

**Key Configuration Details:**

| Component | Value | Purpose |
|:---|:---|:---|
| `running: true` | VM starts automatically | Ensures VM boots after creation |
| `cores: 4, memory: 16Gi` | Resource allocation | Control plane sizing |
| `macAddress: 52:54:00:30:00:02` | Static MAC | Consistent DHCP/static IP assignment |
| `networkName: default/platform` | VLAN 30 network | Isolates platform traffic |
| `imageId: default/talos-1.9.0` | Clone Talos image | Fast provisioning from template |
| `config_url` | Talos machine config | PXE-style config fetch for cloud-init |

### Platform CP-3 VM Definition

Similar to CP-2, with unique identifiers:

```yaml
# File: clusters/harvester/vms/platform/cp-3.yaml
# Purpose: Platform cluster control plane node 3 (Harvester VM)

apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: platform-cp-3
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "0"
    harvesterhci.io/description: "Platform cluster control plane node 3"
  labels:
    harvesterhci.io/cluster: platform
    harvesterhci.io/node-role: control-plane
    harvesterhci.io/os: talos
spec:
  running: true
  template:
    metadata:
      labels:
        kubevirt.io/vm: platform-cp-3
        harvesterhci.io/cluster: platform
        harvesterhci.io/node-role: control-plane
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchExpressions:
                    - key: harvesterhci.io/cluster
                      operator: In
                      values:
                        - platform
                topologyKey: kubernetes.io/hostname
      domain:
        cpu:
          cores: 4
          sockets: 1
          threads: 1
        memory:
          guest: 16Gi
        devices:
          disks:
            - name: rootdisk
              bootOrder: 1
              disk:
                bus: virtio
            - name: cloudinitdisk
              disk:
                bus: virtio
          interfaces:
            - name: default
              bridge: {}
              macAddress: "52:54:00:30:00:03"  # Unique MAC
        resources:
          requests:
            memory: 16Gi
            cpu: "4"
          limits:
            memory: 16Gi
            cpu: "4"
        machine:
          type: q35
        features:
          acpi:
            enabled: true
          smm:
            enabled: true
        firmware:
          bootloader:
            efi:
              secureBoot: false
      networks:
        - name: default
          multus:
            networkName: default/platform
      volumes:
        - name: rootdisk
          persistentVolumeClaim:
            claimName: platform-cp-3-rootdisk
        - name: cloudinitdisk
          cloudInitNoCloud:
            networkData: |
              version: 2
              ethernets:
                eth0:
                  addresses:
                    - 10.10.30.13/24  # Unique IP
                  gateway4: 10.10.30.1
                  nameservers:
                    addresses:
                      - 10.10.10.1
                      - 1.1.1.1
            userData: |
              #cloud-config
              talos:
                config_url: http://10.10.10.5:8080/talos/platform-cp-3.yaml  # Unique config

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: platform-cp-3-rootdisk
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
    harvesterhci.io/imageId: default/talos-1.9.0
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 50Gi
  storageClassName: longhorn
  volumeMode: Block
```

### Standalone VM Considerations

Standalone VMs (e.g., Windows gaming VM, TrueNAS) follow the same structure but with different requirements:

**Example: Windows Gaming VM (Conceptual)**

```yaml
# File: clusters/harvester/vms/standalone/windows-gaming.yaml
# Note: This is a conceptual example, not deployed in initial lab

apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: windows-gaming
  namespace: default
  annotations:
    argocd.argoproj.io/sync-wave: "1"
    harvesterhci.io/description: "Windows gaming workstation"
spec:
  running: false  # Manual start/stop for gaming sessions
  template:
    spec:
      domain:
        cpu:
          cores: 8
          sockets: 1
          threads: 2
        memory:
          guest: 32Gi
        devices:
          disks:
            - name: rootdisk
              bootOrder: 1
              disk:
                bus: sata
            - name: datadisk
              disk:
                bus: scsi
          interfaces:
            - name: default
              bridge: {}
          # GPU passthrough (if configured)
          hostDevices:
            - name: gpu
              deviceName: nvidia.com/GPU_0f17_2267
        resources:
          requests:
            memory: 32Gi
            cpu: "16"
        machine:
          type: pc-q35-5.2
        features:
          acpi:
            enabled: true
          hyperv:
            relaxed:
              enabled: true
            vapic:
              enabled: true
            spinlocks:
              enabled: true
              spinlocks: 8191
        firmware:
          bootloader:
            efi:
              secureBoot: false
      networks:
        - name: default
          multus:
            networkName: default/mgmt
      volumes:
        - name: rootdisk
          persistentVolumeClaim:
            claimName: windows-gaming-rootdisk
        - name: datadisk
          persistentVolumeClaim:
            claimName: windows-gaming-datadisk
```

**Standalone VM Characteristics:**
- Larger resource allocations (gaming, media processing)
- GPU/hardware passthrough capabilities
- Different OS images (Windows, TrueNAS, etc.)
- Manual start/stop policies
- VLAN 10 (mgmt) network for LAN access

---

## Storage Configuration

Harvester uses Longhorn for VM disk storage. Storage classes define performance characteristics and replication policies.

### Longhorn Storage Classes

#### Default Storage Class

```yaml
# Created by Harvester automatically
# High availability with 3-replica replication

apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: longhorn
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: driver.longhorn.io
allowVolumeExpansion: true
reclaimPolicy: Delete
volumeBindingMode: Immediate
parameters:
  numberOfReplicas: "3"
  staleReplicaTimeout: "2880"
  fromBackup: ""
  fsType: "ext4"
  dataLocality: "disabled"
  replicaAutoBalance: "best-effort"
```

#### Fast Storage Class (SSD-only)

```yaml
# File: clusters/harvester/config/storage/longhorn-fast.yaml
# Purpose: High-performance storage for database VMs

apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: longhorn-fast
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
provisioner: driver.longhorn.io
allowVolumeExpansion: true
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
parameters:
  numberOfReplicas: "2"
  staleReplicaTimeout: "2880"
  dataLocality: "best-effort"
  replicaAutoBalance: "best-effort"
  diskSelector: "ssd"  # Only use nodes with SSD tag
  nodeSelector: ""
```

#### Single-Replica Storage Class (Ephemeral)

```yaml
# File: clusters/harvester/config/storage/longhorn-single.yaml
# Purpose: Non-critical data, faster provisioning

apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: longhorn-single
  annotations:
    argocd.argoproj.io/sync-wave: "-3"
provisioner: driver.longhorn.io
allowVolumeExpansion: true
reclaimPolicy: Delete
volumeBindingMode: Immediate
parameters:
  numberOfReplicas: "1"
  staleReplicaTimeout: "2880"
  dataLocality: "disabled"
  replicaAutoBalance: "disabled"
```

### VM Disk Provisioning

When a VirtualMachine CRD references a PVC, Longhorn provisions the disk:

**Process Flow:**
```
1. VirtualMachine CRD created
   ↓
2. References PVC (platform-cp-2-rootdisk)
   ↓
3. PVC has annotation harvesterhci.io/imageId: default/talos-1.9.0
   ↓
4. Harvester clones VirtualMachineImage PVC to new PVC
   ↓
5. Longhorn provisions replicated volume
   ↓
6. PVC bound, VM can start
```

**Storage Performance Considerations:**

| Use Case | Storage Class | Replicas | Rationale |
|:---|:---|:---:|:---|
| Control plane VMs | `longhorn` (default) | 3 | High availability required |
| Worker node VMs | `longhorn` (default) | 3 | Data durability |
| Database VMs | `longhorn-fast` | 2 | Performance + availability |
| Temp/cache volumes | `longhorn-single` | 1 | Speed, non-critical data |

---

## Integration with CAPI

Harvester integrates with Cluster API (CAPI) to provision tenant cluster VMs dynamically. This section clarifies the distinction between Harvester-managed and CAPI-managed VMs.

### Harvester Provider Overview

The CAPI Harvester provider allows CAPI to create and manage VirtualMachine CRDs in the Harvester cluster.

**Architecture:**
```
Platform Cluster (CAPI)                 Harvester Cluster
┌────────────────────────┐             ┌────────────────────────┐
│ TenantCluster XR       │             │                        │
│   ↓                    │             │                        │
│ CAPI Cluster CRD       │             │                        │
│   ↓                    │             │                        │
│ CAPI Machine CRDs      │─────────────▶│ VirtualMachine CRDs   │
│   ↓                    │   Harvester  │   ↓                    │
│ CAPI Talos Bootstrap   │   Provider   │ KubeVirt creates VMs  │
└────────────────────────┘             └────────────────────────┘
```

### How CAPI Uses Harvester Provider

When a TenantCluster XR is created (e.g., `clusters/media/cluster.yaml`):

1. **Crossplane processes XR** and creates CAPI Cluster resource
2. **CAPI Harvester provider** reads CAPI Machine specs
3. **Provider creates VirtualMachine CRDs** in Harvester cluster
4. **KubeVirt reconciles VMs** on Harvester nodes
5. **CAPI Talos provider** generates machine configs
6. **VMs boot** and join the tenant cluster

**Example CAPI-Generated VirtualMachine (Conceptual):**

```yaml
# Created by CAPI Harvester provider, NOT by Argo CD
# Lives in Harvester cluster

apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: media-control-plane-0
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: media
    cluster.x-k8s.io/role: control-plane
    capi.harvesterhci.io/managed: "true"
spec:
  running: true
  template:
    spec:
      domain:
        cpu:
          cores: 4
        memory:
          guest: 8Gi
        devices:
          disks:
            - name: rootdisk
              disk:
                bus: virtio
          interfaces:
            - name: default
              bridge: {}
      networks:
        - name: default
          multus:
            networkName: default/cluster  # VLAN 40
      volumes:
        - name: rootdisk
          persistentVolumeClaim:
            claimName: media-control-plane-0-rootdisk
```

**Differences from Harvester-Managed VMs:**

| Aspect | Harvester-Managed (Platform CP-2/CP-3) | CAPI-Managed (Tenant Clusters) |
|:---|:---|:---|
| **Defined in** | Git repo (`clusters/harvester/vms/`) | Generated dynamically by CAPI |
| **Lifecycle** | Managed by Argo CD | Managed by CAPI controllers |
| **Network** | VLAN 30 (platform) | VLAN 40 (cluster) |
| **Purpose** | Bootstrap platform HA | Tenant workload clusters |
| **Configuration** | cloud-init with Talos config URL | CAPI Talos provider auto-config |
| **Deletion** | Manual (or Argo CD prune) | Automatic when TenantCluster XR deleted |

### Distinction: Raw VMs vs CAPI-Managed VMs

```
clusters/harvester/vms/
├── platform/              ← Harvester-managed (Argo CD)
│   ├── cp-2.yaml          ← Bootstrap only, manual VirtualMachine CRD
│   └── cp-3.yaml          ← Bootstrap only, manual VirtualMachine CRD
└── standalone/            ← Harvester-managed (Argo CD)
    └── (future VMs)       ← Permanent, manual VirtualMachine CRDs

(Not in repo)
Tenant cluster VMs         ← CAPI-managed (dynamic)
  - Created by CAPI Harvester provider
  - Defined by TenantCluster XR composition
  - Lifecycle tied to CAPI Cluster resource
```

**Why Platform VMs are in Git:**
- Platform cluster exists BEFORE CAPI is available
- Cannot use TenantCluster XR to provision platform itself (chicken-and-egg)
- Explicit VirtualMachine CRDs enable manual bootstrap
- After bootstrap, these VMs could theoretically be migrated to CAPI management

**Why Tenant VMs are NOT in Git:**
- Fully declarative via TenantCluster XR
- CAPI handles VM lifecycle automatically
- Cluster scaling (add/remove nodes) handled by CAPI
- No manual VM definition required

---

## Summary Tables

### Network Summary

| Network | VLAN | Subnet | MTU | Used By |
|:---|:---:|:---|:---:|:---|
| `mgmt` | 10 | 10.10.10.0/24 | 1500 | Harvester, infrastructure |
| `platform` | 30 | 10.10.30.0/24 | 1500 | Platform CP-2, CP-3 |
| `cluster` | 40 | 10.10.40.0/24 | 9000 | Tenant cluster VMs |
| `storage` | 60 | 10.10.60.0/24 | 9000 | Longhorn replication |

### Image Summary

| Image | Version | Size | Used By |
|:---|:---|:---|:---|
| `talos-1.9.0` | 1.9.0 | ~1GB | Platform VMs, tenant VMs |
| `ubuntu-22.04` | 22.04 LTS | ~2GB | Standalone VMs |
| `windows-server-2022` | 2022 | ~20GB | Standalone VMs (if needed) |

### VM Summary

| VM Name | CPU | Memory | Disk | Network | Purpose |
|:---|:---:|:---:|:---:|:---|:---|
| `platform-cp-2` | 4 | 16Gi | 50Gi | VLAN 30 | Platform HA node |
| `platform-cp-3` | 4 | 16Gi | 50Gi | VLAN 30 | Platform HA node |
| (CAPI tenant VMs) | Variable | Variable | Variable | VLAN 40 | Dynamic tenant clusters |

### Storage Class Summary

| Storage Class | Replicas | Use Case | Performance |
|:---|:---:|:---|:---|
| `longhorn` (default) | 3 | HA workloads | Balanced |
| `longhorn-fast` | 2 | Databases | High IOPS |
| `longhorn-single` | 1 | Ephemeral data | Fast provisioning |

---

## References

- See main architecture documentation for high-level concepts
- See Appendix C for Argo CD ApplicationSet configuration
- Harvester documentation: https://docs.harvesterhci.io/
- KubeVirt documentation: https://kubevirt.io/
- Longhorn documentation: https://longhorn.io/
- CAPI Harvester provider: https://github.com/harvester/cluster-api-provider-harvester
