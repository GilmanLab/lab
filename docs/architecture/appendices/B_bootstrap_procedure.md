# Appendix B: Bootstrap Procedure Reference

> **Status**: Authoritative Reference
> **Date**: 2025-12-19

This document defines the architectural design of the lab bootstrap process. It describes WHAT happens at each phase and WHY, not HOW to execute commands (see `bootstrap/genesis/` runbooks for step-by-step instructions).

---

## Table of Contents

1. [Overview](#overview)
2. [Design Principles](#design-principles)
3. [Bootstrap Phases](#bootstrap-phases)
4. [Phase 1: Seed (NAS)](#phase-1-seed-nas)
5. [Phase 2: Single-Node Platform (UM760)](#phase-2-single-node-platform-um760)
6. [Phase 3: Harvester Online](#phase-3-harvester-online)
7. [Phase 4: Full Platform (3-Node HA)](#phase-4-full-platform-3-node-ha)
8. [Bootstrap Progression Diagram](#bootstrap-progression-diagram)
9. [Complete Step Reference](#complete-step-reference)
10. [Prerequisites](#prerequisites)
11. [Post-Bootstrap State](#post-bootstrap-state)

---

## Overview

The lab infrastructure bootstrap is a carefully orchestrated 4-phase process that progressively builds the platform from a minimal seed cluster to a fully operational, highly available control plane.

### The Chicken-and-Egg Problem

The platform cluster hosts the very tools needed to provision infrastructure:
- **Crossplane** processes XR Claims to provision resources
- **CAPI** provisions tenant Kubernetes clusters
- **Tinkerbell** provisions bare-metal hardware via PXE

This creates a dependency loop: we cannot use these tools to bootstrap the platform that hosts them.

### The Solution: Progressive Bootstrap

1. **Start minimal** - Seed cluster on NAS with only Tinkerbell (no Crossplane)
2. **Provision foundation** - Use Tinkerbell to PXE boot UM760 (first platform node)
3. **Migrate and upgrade** - Move to UM760, deploy full platform with Crossplane/CAPI
4. **Scale out** - Use Tinkerbell to provision Harvester, then expand platform to 3 nodes

By the end, the platform is self-managing via Crossplane XRs.

---

## Design Principles

| Principle | Rationale |
|:----------|:----------|
| **Resource efficiency** | Seed cluster uses minimal RAM (NAS has only 32GB shared with Synology) |
| **Progressive complexity** | Each phase adds capability only when needed |
| **GitOps from the start** | Even seed cluster uses Argo CD (though with raw manifests) |
| **Reproducibility** | Entire process documented and scriptable |
| **Self-healing target** | Final state is fully declarative and self-managing |

---

## Bootstrap Phases

The bootstrap is divided into 4 phases, spanning 20 discrete steps.

```
Phase 1: Seed (NAS)
  Steps 1-8: Build images, minimal cluster with Tinkerbell, provision VyOS and UM760
  Duration: ~45 minutes
  Result: VyOS router online, UM760 joins cluster via PXE

Phase 2: Single-Node Platform (UM760)
  Steps 9-13: Migrate to UM760, deploy full platform
  Duration: ~1 hour
  Result: Platform cluster with Crossplane, CAPI, Harvester provisioned

Phase 3: Harvester Online
  Steps 14-17: Register Harvester, create platform VMs
  Duration: ~2 hours (Harvester install is slow)
  Result: CP-2 and CP-3 join platform cluster

Phase 4: Full Platform (3-Node HA)
  Steps 18-20: Deploy remaining services, steady state
  Duration: ~30 minutes
  Result: Full platform operational, ready for tenant clusters
```

**Total Duration:** ~4.5 hours (mostly waiting for Harvester installation)

---

## Phase 1: Seed (NAS)

**Goal:** Create a minimal Kubernetes cluster on the NAS that can provision VyOS (lab networking) and UM760 (first platform node) via PXE.

**Resource Constraints:**
- NAS has 32GB RAM shared with Synology DSM
- Talos VM allocated: 4 vCPU, 8GB RAM, 40GB disk
- Single-node cluster (no HA required for temporary seed)

**What Runs on the Seed:**
- Argo CD (GitOps controller)
- Tinkerbell (PXE/DHCP/TFTP services)
- NGINX (serves Talos machine configs and VyOS image)

**What Does NOT Run on the Seed:**
- Crossplane (too heavyweight, not needed yet)
- CAPI (requires Crossplane)
- CoreServices (Istio, cert-manager, etc.)
- PlatformServices (Zitadel, OpenBAO, etc.)

### Step 1: Build VyOS Image

**Purpose:** Create a bootable VyOS disk image with the initial lab configuration baked in.

**Mechanism:**
- Run Packer against `infrastructure/network/vyos/packer/vyos.pkr.hcl`
- Packer downloads VyOS ISO, installs to virtual disk, applies initial config
- Configuration sourced from `infrastructure/network/vyos/configs/gateway.conf`
- Output: Raw disk image stored on NAS for Tinkerbell to serve

**Why Now:**
- VyOS image must exist before Tinkerbell can serve it
- Baking config into image avoids manual configuration during bootstrap
- Packer runs on admin workstation (not in cluster)

**Image Contents:**
- VyOS LTS release
- Pre-configured VLANs (10, 20, 30, 40, 50, 60)
- DHCP relay for VLANs 30 and 40 (points to Tinkerbell)
- BGP peering configuration for service VIPs
- Firewall rules for lab isolation
- SSH keys for initial access

**Output:**
```
infrastructure/network/vyos/packer/output/
└── vyos-lab.raw              # Raw disk image (~2GB)
```

### Step 2: Generate Talos Configs

**Purpose:** Create encrypted machine configurations for all platform cluster nodes.

**Mechanism:**
- Run talhelper against `infrastructure/compute/talos/talconfig.yaml`
- Generates configs for: CP-1 (UM760), CP-2 (Harvester VM), CP-3 (Harvester VM)
- Encrypts secrets with SOPS
- Output: `infrastructure/compute/talos/clusterconfig/` (gitignored)

**Why Now:**
- Configs must exist before nodes can boot
- talhelper runs on admin workstation (not in cluster)

**Files Generated:**
```
infrastructure/compute/talos/clusterconfig/
├── platform-cp-1.yaml          # UM760 (MAC: xx:xx:xx:xx:xx:01)
├── platform-cp-2.yaml          # Harvester VM (created in Phase 3)
├── platform-cp-3.yaml          # Harvester VM (created in Phase 3)
└── talosconfig                 # Admin kubeconfig for talosctl
```

### Step 3: Create Seed Talos VM

**Purpose:** Create the initial Kubernetes node on the NAS.

**Mechanism:**
- Use Synology Virtual Machine Manager
- Allocate resources: 4 vCPU, 8GB RAM, 40GB disk
- Attach Talos ISO
- Boot and apply minimal Talos config (single-node cluster)

**Why Manual:**
- Cannot PXE boot on NAS (no Tinkerbell yet)
- One-time operation, not worth automating

**Result:**
- Single-node Talos cluster running on NAS
- No workloads yet (just Kubernetes API)

### Step 4: Deploy Argo CD

**Purpose:** Establish GitOps controller to manage all subsequent deployments.

**Mechanism:**
- Install Argo CD via Helm CLI (manual)
- Use `bootstrap/genesis/scripts/install-argocd.sh`
- Register platform cluster with itself (in-cluster registration)

**Why Argo CD First:**
- All subsequent components deployed via GitOps
- Enables declarative, version-controlled infrastructure
- Argo CD is lightweight enough for seed cluster

**Configuration:**
```yaml
# Created by install-argocd.sh
apiVersion: v1
kind: Secret
metadata:
  name: platform
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: cluster
    lab.gilman.io/cluster-name: platform
stringData:
  name: platform
  server: https://kubernetes.default.svc  # In-cluster
```

### Step 5: Apply Bootstrap Application

**Purpose:** Deploy Tinkerbell and provisioning configurations for VyOS and UM760.

**Mechanism:**
- Create Argo CD Application pointing to `bootstrap/seed/`
- Argo CD syncs raw Kubernetes manifests (NOT XRs)
- Deployed: Tinkerbell stack, Hardware CRDs (VP6630, UM760), Workflow CRDs (VyOS, Talos), NGINX config server

**Why Raw Manifests:**
- Crossplane is not running yet (cannot process XRs)
- Temporary deployment (will be deleted and redeployed as XRs in Phase 2)

**Application Definition:**
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: bootstrap
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/gilmanlab/lab.git
    targetRevision: HEAD
    path: bootstrap/seed
    directory:
      recurse: true
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      selfHeal: true
      prune: false  # Manual cleanup after migration
```

**What Gets Deployed:**

| Component | Purpose |
|:----------|:--------|
| Tinkerbell namespace | Isolation for Tinkerbell services |
| Tinkerbell Helm release | Boots, Hegel, Rufio, Tink server, Tink worker |
| Hardware CRD (VP6630) | Registers VyOS router MAC address, BMC, network config |
| Hardware CRD (UM760) | Registers UM760 MAC address, BMC, network config |
| Workflow CRD (VyOS) | Defines VyOS installation workflow for VP6630 |
| Workflow CRD (Talos) | Defines Talos installation workflow for UM760 |
| NGINX deployment | Serves VyOS image and talhelper-generated Talos configs over HTTP |

### Step 6: Tinkerbell Deploys

**Purpose:** Tinkerbell services come online and begin listening for PXE requests.

**Mechanism:**
- Boots (DHCP server) starts serving VLAN 30 (platform network)
- Hegel (metadata server) provides hardware-specific config
- Tink server stores workflows and hardware definitions
- Tink worker polls for workflow executions

**Services Exposed:**

| Service | Protocol | Port | Purpose |
|:--------|:---------|:-----|:--------|
| Boots | DHCP | 67 | PXE boot DHCP |
| Boots | TFTP | 69 | Kernel/initrd transfer |
| Boots | HTTP | 8080 | iPXE scripts, OS images |
| Hegel | HTTP | 50061 | Metadata API |
| NGINX | HTTP | 8081 | VyOS image and Talos machine configs |

**Result:**
- Tinkerbell ready to provision VyOS and UM760
- Network boot infrastructure operational

### Step 7: VyOS Boots via PXE

**Purpose:** Provision the VyOS router to establish lab networking infrastructure.

**Mechanism:**
1. Power on VP6630 with PXE boot enabled
2. VP6630 sends DHCP request (bootstrap network)
3. Boots responds with PXE boot parameters
4. VP6630 downloads iPXE script, kernel, initrd
5. Tinkerbell Workflow executes:
   - Downloads VyOS raw image from NGINX
   - Writes VyOS image to disk
   - Reboots into VyOS
6. VyOS boots with pre-configured lab networking

**Why VyOS First:**
- Lab VLANs must exist before UM760 can PXE boot on VLAN 30
- DHCP relay configuration routes PXE requests to Tinkerbell
- Without VyOS, UM760 cannot reach Tinkerbell on the provisioning network

**What Gets Configured:**
- VLANs 10, 20, 30, 40, 50, 60 on trunk interfaces
- DHCP relay for VLANs 30 and 40 (points to Tinkerbell IP)
- Inter-VLAN routing enabled
- BGP peering configuration (inactive until Cilium peers)
- Firewall rules for lab isolation from home network

**Timeline:**
- PXE boot: ~2 minutes
- VyOS install: ~3 minutes
- VyOS boot with config: ~1 minute
- **Total: ~6 minutes**

**Result:**
- Lab networking operational
- VLANs available for subsequent provisioning
- DHCP relay active for platform and cluster networks

### Step 8: UM760 Boots via PXE

**Purpose:** Provision the UM760 as the first production platform node.

**Mechanism:**
1. Power on UM760 with PXE boot enabled
2. UM760 sends DHCP request (VLAN 30 via VyOS DHCP relay)
3. Boots responds with PXE boot parameters
4. UM760 downloads iPXE script, kernel, initrd
5. Tinkerbell Workflow executes:
   - Downloads Talos image
   - Fetches machine config from NGINX (cp-1.yaml)
   - Writes Talos to disk
   - Reboots into Talos
6. Talos bootstraps and joins cluster as CP-1

**What Makes UM760 Special:**
- MAC address matches Hardware CRD: `00:68:eb:xx:xx:01`
- Workflow triggered automatically when UM760 PXE boots
- Talos config URL served via Hegel metadata

**Timeline:**
- PXE boot: ~2 minutes
- Talos install: ~5 minutes
- Cluster join: ~2 minutes
- **Total: ~10 minutes**

**Result:**
- Platform cluster now has 2 nodes: NAS VM + UM760
- UM760 is control plane + worker (hybrid)

---

## Phase 2: Single-Node Platform (UM760)

**Goal:** Migrate workloads from NAS to UM760, delete temporary seed manifests, and deploy full platform with Crossplane/CAPI.

**Why Migrate:**
- UM760 has more resources (64GB RAM vs 8GB)
- NAS can be shut down (reclaim 8GB RAM for DSM)
- UM760 is production hardware (UM760 is permanent, NAS was temporary)

### Step 9: Migrate Workloads to UM760

**Purpose:** Move all running pods from NAS to UM760.

**Mechanism:**
- Cordon NAS node: `kubectl cordon platform-cp-0`
- Drain NAS node: `kubectl drain platform-cp-0 --ignore-daemonsets --delete-emptydir-data`
- Wait for pods to reschedule on UM760
- Power off NAS VM (optional, or keep as backup)

**Workloads Migrated:**
- Argo CD (controller, server, repo-server)
- Tinkerbell stack (Boots, Hegel, Rufio, Tink)
- NGINX config server

**Result:**
- All workloads running on UM760
- NAS node idle (can be removed from cluster or powered off)

### Step 10: Delete Bootstrap Application

**Purpose:** Remove temporary seed manifests to make room for XR-based deployment.

**Mechanism:**
- Delete Argo CD Application: `kubectl delete application bootstrap -n argocd`
- Manually delete any remaining resources (if not pruned)
- Tinkerbell namespace and resources removed

**Why Delete:**
- Seed used raw manifests; production uses XRs
- Cannot have both raw Tinkerbell and XR-based Tinkerbell simultaneously
- Clean slate for proper deployment

**Result:**
- No Tinkerbell running (temporarily)
- Argo CD still running
- UM760 still in cluster (Talos is persistent)

### Step 11: Apply Platform Configuration

**Purpose:** Deploy full platform cluster configuration via Crossplane XRs.

**Mechanism:**
- Create ApplicationSet: `kubectl apply -f argocd/applicationsets/cluster-definitions.yaml`
- Argo CD discovers `clusters/platform/` directory
- Syncs `core.yaml` and `platform.yaml` to platform cluster

**What Gets Deployed:**

| File | XR Type | What It Provisions |
|:-----|:--------|:-------------------|
| `clusters/platform/core.yaml` | CoreServices | Crossplane, CAPI, cert-manager, external-dns, Istio |
| `clusters/platform/platform.yaml` | PlatformServices | Zitadel, OpenBAO, Vault Secrets Operator |

**Sync Waves:**
- Wave 0: `core.yaml` (Crossplane must be running first)
- Wave 1: `platform.yaml` (depends on Crossplane)
- Wave 2: `apps/*` (depends on CoreServices)

**Result:**
- Crossplane is running
- CAPI is installed (but no clusters yet)
- cert-manager, external-dns, Istio deployed
- Zitadel and OpenBAO deployed

### Step 12: Crossplane + Tinkerbell (XRD)

**Purpose:** Redeploy Tinkerbell via proper Crossplane abstraction.

**Mechanism:**
- ApplicationSet discovers `clusters/platform/apps/tinkerbell/`
- Argo CD syncs Hardware XRs and Workflow XRs
- Crossplane processes XRs and creates Tinkerbell Hardware/Workflow CRDs

**Files Synced:**
```
clusters/platform/apps/tinkerbell/
├── hardware/
│   ├── ms02-node1.yaml         # Hardware XR for Harvester node 1
│   ├── ms02-node2.yaml         # Hardware XR for Harvester node 2
│   ├── ms02-node3.yaml         # Hardware XR for Harvester node 3
│   └── um760.yaml              # Hardware XR for UM760 (already provisioned)
└── workflows/
    ├── harvester.yaml          # Workflow XR for Harvester installation
    └── talos.yaml              # Workflow XR for Talos installation
```

**Result:**
- Tinkerbell redeployed via XRs (now permanent)
- Hardware definitions registered for MS-02 nodes (Harvester cluster)
- Workflows ready to provision Harvester

### Step 13: Provision Harvester

**Purpose:** Install Harvester OS on MS-02 nodes to create HCI cluster.

**Mechanism:**
1. Power on MS-02 nodes with PXE boot enabled
2. Tinkerbell detects hardware (MAC addresses match Hardware XRs)
3. Executes Harvester installation workflow:
   - Downloads Harvester ISO
   - Writes Harvester to disk
   - Applies Harvester config (cluster token, network config)
   - Reboots into Harvester
4. Harvester cluster self-forms (3-node)

**Harvester Installation:**
- **Duration:** ~1.5 hours (Harvester install is slow)
- **Result:** 3-node Harvester cluster running on MS-02 hardware
- **Services:** Longhorn (storage), Harvester VMs, Harvester networking

**Harvester Cluster Details:**

| Node | Hardware | Role | IP (VLAN 10 - Mgmt) |
|:-----|:---------|:-----|:--------------------|
| ms02-node1 | MS-02 | Control Plane + Compute | 10.10.10.11 |
| ms02-node2 | MS-02 | Control Plane + Compute | 10.10.10.12 |
| ms02-node3 | MS-02 | Control Plane + Compute | 10.10.10.13 |

**Result:**
- Harvester cluster operational
- Ready to host VMs
- Platform cluster still single-node (UM760)

---

## Phase 3: Harvester Online

**Goal:** Register Harvester with Argo CD, deploy VM configurations, and create CP-2/CP-3 VMs to expand platform cluster.

**Why This Phase:**
- Platform cluster is still single-node (no HA)
- Need VMs on Harvester to add CP-2 and CP-3
- Harvester must be registered with Argo CD before it can be managed

### Step 14: Register Harvester with Argo CD

**Purpose:** Allow Argo CD to deploy resources to Harvester cluster.

**Mechanism:**
- Extract Harvester kubeconfig (from Harvester UI or API)
- Create Argo CD cluster Secret with Harvester endpoint and credentials
- Label Secret with `lab.gilman.io/cluster-name: harvester`

**Cluster Secret:**
```yaml
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
  server: https://10.10.10.11:6443  # Harvester API endpoint
  config: |
    {
      "tlsClientConfig": {
        "caData": "...",
        "certData": "...",
        "keyData": "..."
      }
    }
```

**Result:**
- Harvester appears in Argo CD cluster list
- ApplicationSet can now route apps to Harvester

### Step 15: Argo CD Syncs `clusters/harvester/`

**Purpose:** Deploy Harvester network configuration, VM images, and VM definitions.

**Mechanism:**
- ApplicationSet matrix generator discovers Harvester cluster + `clusters/harvester/` directory
- Argo CD syncs to Harvester cluster (not platform)
- Deploys raw Harvester CRDs (ClusterNetwork, VlanConfig, VirtualMachineImage, VirtualMachine)

**What Gets Deployed:**

**Networks (clusters/harvester/config/networks/):**

| File | VLAN | Purpose | Subnet |
|:-----|:----:|:--------|:-------|
| `mgmt.yaml` | 10 | Management network | 10.10.10.0/24 |
| `platform.yaml` | 30 | Platform cluster network | 10.10.30.0/24 |
| `cluster.yaml` | 40 | Tenant cluster network | 10.10.40.0/24 |
| `storage.yaml` | 60 | Ceph replication network | 10.10.60.0/24 |

**Images (clusters/harvester/config/images/):**

| File | Image | Purpose |
|:-----|:------|:--------|
| `talos-1.9.yaml` | Talos 1.9 VM image | OS for platform cluster VMs |

**VMs (clusters/harvester/vms/platform/):**

| File | VM Name | vCPU | RAM | Disk | Network | MAC Address |
|:-----|:--------|:-----|:----|:-----|:--------|:------------|
| `cp-2.yaml` | platform-cp-2 | 8 | 16GB | 100GB | VLAN 30 | 52:54:00:xx:xx:02 |
| `cp-3.yaml` | platform-cp-3 | 8 | 16GB | 100GB | VLAN 30 | 52:54:00:xx:xx:03 |

**Result:**
- Harvester networks configured
- Talos VM image available
- CP-2 and CP-3 VMs created (powered off initially)

### Step 16: CP-2, CP-3 VMs Created

**Purpose:** Create VirtualMachine resources on Harvester for platform cluster nodes.

**Mechanism:**
- Harvester processes VirtualMachine CRDs
- Allocates resources from Longhorn storage
- Attaches to VLAN 30 (platform network)
- VMs created in powered-off state

**VM Specifications:**

| Parameter | CP-2 | CP-3 |
|:----------|:-----|:-----|
| CPU | 8 vCPU | 8 vCPU |
| RAM | 16GB | 16GB |
| Disk | 100GB (Longhorn) | 100GB (Longhorn) |
| Network | VLAN 30 | VLAN 30 |
| MAC | 52:54:00:xx:xx:02 | 52:54:00:xx:xx:03 |
| Boot Order | Network (PXE) → Disk | Network (PXE) → Disk |

**Result:**
- VMs exist but not yet running
- Ready for PXE boot

### Step 17: CP-2, CP-3 PXE Boot

**Purpose:** Provision CP-2 and CP-3 with Talos OS and join platform cluster.

**Mechanism:**
1. Power on CP-2 and CP-3 VMs
2. VMs PXE boot (first boot device is network)
3. Tinkerbell detects VMs (MAC addresses match Hardware XRs)
4. Executes Talos installation workflow:
   - Downloads Talos image
   - Fetches machine config from NGINX (cp-2.yaml, cp-3.yaml)
   - Writes Talos to disk
   - Reboots into Talos
5. Talos bootstraps and joins platform cluster as CP-2, CP-3

**Timeline:**
- PXE boot (per VM): ~2 minutes
- Talos install (per VM): ~5 minutes
- Cluster join (per VM): ~2 minutes
- **Total: ~20 minutes** (VMs can boot in parallel)

**Result:**
- Platform cluster now has 3 control plane nodes: CP-1 (UM760), CP-2 (VM), CP-3 (VM)
- High availability achieved (etcd quorum: 2/3)

---

## Phase 4: Full Platform (3-Node HA)

**Goal:** Deploy remaining platform services and reach steady state.

**Why This Phase:**
- Platform cluster is now HA (3 control plane nodes)
- Safe to deploy production workloads
- Infrastructure complete, ready for tenant clusters

### Step 18: Deploy Remaining Platform Services

**Purpose:** Activate all platform capabilities (observability, policy, etc.).

**Mechanism:**
- ApplicationSet discovers `clusters/platform/apps/*`
- Argo CD syncs Application XRs to platform cluster
- Crossplane processes XRs and deploys Helm releases

**Services Deployed:**

**Observability (clusters/platform/apps/observability/):**

| File | Application | Purpose |
|:-----|:------------|:--------|
| `prometheus.yaml` | Prometheus | Metrics collection and alerting |
| `grafana.yaml` | Grafana | Metrics visualization |
| `loki.yaml` | Loki | Log aggregation |

**CAPI (clusters/platform/apps/capi/):**

| File | Purpose |
|:-----|:--------|
| `providers.yaml` | Install CAPI providers (Harvester, Talos) |
| `harvester-config.yaml` | Configure Harvester provider with credentials |

**Result:**
- Full observability stack operational
- CAPI ready to provision tenant clusters
- Platform services fully deployed

### Step 19: Steady State

**Purpose:** Validate that all platform components are healthy and operational.

**Validation Checklist:**

| Component | Check | Expected Result |
|:----------|:------|:----------------|
| Platform cluster | `kubectl get nodes` | 3 nodes (CP-1, CP-2, CP-3) all Ready |
| Harvester cluster | `kubectl get nodes --kubeconfig harvester` | 3 nodes (ms02-1, ms02-2, ms02-3) all Ready |
| Crossplane | `kubectl get xrd` | All XRDs Established |
| CAPI | `kubectl get providers -A` | Harvester and Talos providers Installed |
| Argo CD | UI / `argocd app list` | All apps Healthy, Synced |
| Tinkerbell | `kubectl get hardware -n tinkerbell` | All hardware Ready |
| Istio | `kubectl get pods -n istio-system` | All pods Running |
| Zitadel | `curl https://auth.lab.local` | 200 OK |
| OpenBAO | `curl https://vault.lab.local` | 200 OK |

**Result:**
- Platform cluster is fully operational
- All services healthy
- Ready for tenant clusters

### Step 20: Tenant Clusters

**Purpose:** Begin provisioning application workload clusters.

**Mechanism:**
1. Create TenantCluster XR in `clusters/media/cluster.yaml`
2. Commit and push to Git
3. Argo CD syncs XR to platform cluster
4. Crossplane processes TenantCluster XR:
   - Creates CAPI Cluster resource
   - CAPI Harvester provider creates VMs on Harvester
   - CAPI Talos provider generates machine configs
   - VMs PXE boot, install Talos, join cluster
   - CAPI generates kubeconfig
   - Crossplane creates Argo CD cluster Secret
5. Argo CD discovers new cluster
6. ApplicationSet syncs `clusters/media/apps/*` to media cluster

**Example TenantCluster XR:**
```yaml
apiVersion: infrastructure.lab.gilman.io/v1alpha1
kind: TenantCluster
metadata:
  name: media
  namespace: default
spec:
  controlPlane:
    count: 3
    cpu: 4
    memory: 8Gi
    disk: 100Gi
  workers:
    count: 3
    cpu: 8
    memory: 32Gi
    disk: 500Gi
  network:
    vlan: 40
    subnet: 10.10.40.0/24
```

**Timeline:**
- VM creation: ~5 minutes
- Talos install: ~10 minutes
- Cluster ready: ~15 minutes
- **Total: ~30 minutes per cluster**

**Result:**
- Tenant cluster operational
- Automatically registered with Argo CD
- Applications deployed via GitOps

---

## Bootstrap Progression Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         PHASE 1: SEED (NAS)                             │
│                                                                         │
│  Step 1: Build VyOS Image (Packer)                                     │
│           ↓                                                             │
│  Step 2: Generate Talos Configs (talhelper)                            │
│           ↓                                                             │
│  Step 3: Create Seed Talos VM (NAS - 8GB RAM)                          │
│           ↓                                                             │
│  Step 4: Deploy Argo CD (Helm CLI)                                     │
│           ↓                                                             │
│  Step 5: Apply bootstrap Application → bootstrap/seed/                 │
│           ↓                                                             │
│  Step 6: Tinkerbell Deploys (Boots, Hegel, Rufio, Tink, NGINX)        │
│           ↓                                                             │
│  Step 7: VyOS Boots via PXE → Lab networking established              │
│           ↓                                                             │
│  Step 8: UM760 Boots via PXE → Talos installed → Joins cluster        │
│                                                                         │
│  Result: VyOS router online, 2-node cluster (NAS VM + UM760)           │
└─────────────────────────────────────────────────────────────────────────┘
                               ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                  PHASE 2: SINGLE-NODE PLATFORM (UM760)                  │
│                                                                         │
│  Step 9: Migrate Workloads to UM760 (drain NAS)                        │
│           ↓                                                             │
│  Step 10: Delete bootstrap Application (remove seed manifests)         │
│           ↓                                                             │
│  Step 11: Apply Platform Configuration (clusters/platform/)            │
│           → core.yaml (CoreServices XR)                                 │
│           → platform.yaml (PlatformServices XR)                         │
│           ↓                                                             │
│  Step 12: Crossplane + Tinkerbell (XRD-based, permanent)               │
│           ↓                                                             │
│  Step 13: Provision Harvester (Tinkerbell PXE boots MS-02 nodes)       │
│           → 3-node Harvester cluster online (~1.5 hours)                │
│                                                                         │
│  Result: Platform cluster (1 node), Harvester cluster (3 nodes)        │
└─────────────────────────────────────────────────────────────────────────┘
                               ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                      PHASE 3: HARVESTER ONLINE                          │
│                                                                         │
│  Step 14: Register Harvester with Argo CD (cluster Secret)             │
│           ↓                                                             │
│  Step 15: Argo CD Syncs clusters/harvester/                            │
│           → Networks (VLANs 10, 30, 40, 60)                             │
│           → Images (Talos 1.9)                                          │
│           → VMs (CP-2, CP-3)                                            │
│           ↓                                                             │
│  Step 16: CP-2, CP-3 VMs Created (powered off)                         │
│           ↓                                                             │
│  Step 17: CP-2, CP-3 PXE Boot → Talos installed → Join cluster        │
│                                                                         │
│  Result: Platform cluster (3 nodes HA), Harvester cluster (3 nodes)    │
└─────────────────────────────────────────────────────────────────────────┘
                               ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                  PHASE 4: FULL PLATFORM (3-NODE HA)                     │
│                                                                         │
│  Step 18: Deploy Remaining Platform Services                           │
│           → Observability (Prometheus, Grafana, Loki)                   │
│           → CAPI providers (Harvester, Talos)                           │
│           ↓                                                             │
│  Step 19: Steady State - Validate all components healthy               │
│           ↓                                                             │
│  Step 20: Tenant Clusters - Provision via TenantCluster XR             │
│           → media cluster (3 CP + 3 workers)                            │
│           → dev cluster (1 CP + 2 workers)                              │
│           → prod cluster (3 CP + 5 workers)                             │
│                                                                         │
│  Result: Full lab operational, ready for application workloads         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Complete Step Reference

| Phase | Step | Name | Duration | Purpose |
|:------|:-----|:-----|:---------|:--------|
| 1 | 1 | Build VyOS Image | 10 min | Create VyOS disk image with Packer |
| 1 | 2 | Generate Talos Configs | 2 min | Create machine configs for all platform nodes |
| 1 | 3 | Create Seed Talos VM | 15 min | Bootstrap initial Kubernetes cluster on NAS |
| 1 | 4 | Deploy Argo CD | 5 min | Install GitOps controller |
| 1 | 5 | Apply Bootstrap Application | 2 min | Deploy seed manifests (Tinkerbell, VyOS, UM760) |
| 1 | 6 | Tinkerbell Deploys | 5 min | PXE services come online |
| 1 | 7 | VyOS Boots via PXE | 6 min | Provision VyOS router, establish lab networking |
| 1 | 8 | UM760 Boots via PXE | 10 min | Provision UM760 as first platform node |
| 2 | 9 | Migrate Workloads to UM760 | 10 min | Move pods from NAS to UM760 |
| 2 | 10 | Delete Bootstrap Application | 2 min | Remove temporary seed manifests |
| 2 | 11 | Apply Platform Configuration | 5 min | Deploy CoreServices and PlatformServices XRs |
| 2 | 12 | Crossplane + Tinkerbell (XRD) | 10 min | Redeploy Tinkerbell via XRs |
| 2 | 13 | Provision Harvester | 90 min | PXE boot MS-02 nodes with Harvester OS |
| 3 | 14 | Register Harvester with Argo CD | 5 min | Create cluster Secret for Harvester |
| 3 | 15 | Argo CD Syncs clusters/harvester/ | 5 min | Deploy networks, images, VM definitions |
| 3 | 16 | CP-2, CP-3 VMs Created | 5 min | Harvester creates VM resources |
| 3 | 17 | CP-2, CP-3 PXE Boot | 20 min | Provision VMs with Talos, join platform cluster |
| 4 | 18 | Deploy Remaining Platform Services | 15 min | Observability, CAPI providers |
| 4 | 19 | Steady State | 10 min | Validate all components healthy |
| 4 | 20 | Tenant Clusters | 30 min/cluster | Provision application workload clusters |

**Total Duration (Phases 1-3):** ~3.5 hours
**Phase 4:** Ongoing (as tenant clusters are added)

---

## Prerequisites

Before beginning the bootstrap, ensure the following are in place:

### Hardware

| Component | Requirement | Purpose |
|:----------|:------------|:--------|
| **NAS** | Synology DS920+ or equivalent | Host seed Talos VM, store VyOS image |
| **VP6630** | Minisforum VP6630 (VyOS router) | Lab gateway, VLAN routing, DHCP relay |
| **UM760** | Minisforum UM760 (64GB RAM, 1TB SSD) | First platform node (bare-metal) |
| **MS-02 (×3)** | Minisforum MS-02 (64GB RAM, 1TB NVMe each) | Harvester HCI cluster |
| **Network** | Managed switch with VLAN support | Layer 2 switching, trunk ports |

### Network

| VLAN | Subnet | Purpose | DHCP |
|:-----|:-------|:--------|:-----|
| 10 | 10.10.10.0/24 | Management | Static IPs |
| 20 | 10.10.20.0/24 | Services (NAS, DNS) | Static IPs |
| 30 | 10.10.30.0/24 | Platform cluster | DHCP (Tinkerbell) |
| 40 | 10.10.40.0/24 | Tenant clusters | DHCP (Tinkerbell) |
| 60 | 10.10.60.0/24 | Storage replication | Static IPs |

**Note:** VyOS is provisioned via Tinkerbell during bootstrap (Step 7). The Packer-built image includes:
- VLANs configured and routing enabled
- DHCP relay enabled for VLANs 30 and 40 (points to Tinkerbell)
- DNS forwarding configured
- NTP server configured

### Software

| Tool | Version | Purpose |
|:-----|:--------|:--------|
| Packer | v1.11.0+ | Build VyOS disk image |
| talhelper | v3.0.0+ | Generate Talos machine configs |
| SOPS | v3.9.0+ | Encrypt Talos secrets |
| kubectl | v1.31.0+ | Kubernetes CLI |
| Helm | v3.16.0+ | Install Argo CD |
| talosctl | v1.9.0+ | Talos cluster management |

### Credentials

- **GitHub** - Personal access token with repo access (for Argo CD)
- **SOPS** - Age key for encrypting/decrypting secrets
- **NAS** - Admin credentials for VM creation
- **Harvester** - Cluster token (generated during install)

### Git Repository

- Clone `https://github.com/gilmanlab/lab.git`
- Checkout branch: `main` (or feature branch for testing)
- Update `infrastructure/compute/talos/talconfig.yaml` with actual MAC addresses
- Commit and push changes

---

## Post-Bootstrap State

After completing all 18 steps, the lab infrastructure is in the following state:

### Clusters

| Cluster | Nodes | Purpose | State |
|:--------|:------|:--------|:------|
| **Platform** | 3 (CP-1, CP-2, CP-3) | Control plane, shared services | Operational |
| **Harvester** | 3 (ms02-1, ms02-2, ms02-3) | HCI, VM hosting | Operational |
| **Tenant** | 0 (ready for provisioning) | Application workloads | Ready |

### Platform Cluster Details

**Nodes:**

| Node | Type | Hardware | IP (VLAN 30) | Role |
|:-----|:-----|:---------|:-------------|:-----|
| CP-1 | Bare-metal | UM760 | 10.10.30.10 | Control Plane + Worker |
| CP-2 | VM | Harvester VM | 10.10.30.11 | Control Plane |
| CP-3 | VM | Harvester VM | 10.10.30.12 | Control Plane |

**Services Running:**

| Service | Namespace | Purpose |
|:--------|:----------|:--------|
| Argo CD | argocd | GitOps controller |
| Crossplane | crossplane-system | Infrastructure provisioning |
| CAPI | capi-system | Cluster provisioning |
| Tinkerbell | tinkerbell | PXE provisioning |
| Istio | istio-system | Service mesh |
| cert-manager | cert-manager | Certificate management |
| external-dns | external-dns | DNS automation |
| Zitadel | zitadel | Identity provider |
| OpenBAO | openbao | Secrets management |
| Vault Secrets Operator | vault-secrets-operator | Secret injection |
| Prometheus | observability | Metrics collection |
| Grafana | observability | Metrics visualization |
| Loki | observability | Log aggregation |

### Harvester Cluster Details

**Nodes:**

| Node | Hardware | IP (VLAN 10) | Storage | Role |
|:-----|:---------|:-------------|:--------|:-----|
| ms02-1 | MS-02 | 10.10.10.11 | 1TB NVMe | Control Plane + Compute |
| ms02-2 | MS-02 | 10.10.10.12 | 1TB NVMe | Control Plane + Compute |
| ms02-3 | MS-02 | 10.10.10.13 | 1TB NVMe | Control Plane + Compute |

**Storage:**
- Longhorn replicated storage (3 replicas across nodes)
- Total capacity: ~2.7TB (3×1TB - overhead)

**VMs Running:**

| VM | vCPU | RAM | Disk | Network | Purpose |
|:---|:-----|:----|:-----|:--------|:--------|
| platform-cp-2 | 8 | 16GB | 100GB | VLAN 30 | Platform control plane node 2 |
| platform-cp-3 | 8 | 16GB | 100GB | VLAN 30 | Platform control plane node 3 |

### GitOps State

**Argo CD Applications:**

| Application | Path | Destination | Status |
|:------------|:-----|:------------|:-------|
| cluster-platform | clusters/platform/ | platform | Synced, Healthy |
| platform-tinkerbell | clusters/platform/apps/tinkerbell/ | platform | Synced, Healthy |
| platform-observability | clusters/platform/apps/observability/ | platform | Synced, Healthy |
| platform-capi | clusters/platform/apps/capi/ | platform | Synced, Healthy |
| cluster-harvester | clusters/harvester/ | harvester | Synced, Healthy |

**Registered Clusters:**

| Cluster | Server | Namespace | Label |
|:--------|:-------|:----------|:------|
| platform | https://kubernetes.default.svc | argocd | lab.gilman.io/cluster-name: platform |
| harvester | https://10.10.10.11:6443 | argocd | lab.gilman.io/cluster-name: harvester |

### Crossplane State

**Installed Packages:**

| Package | Version | Purpose |
|:--------|:--------|:--------|
| xrp-infrastructure | v1.0.0 | TenantCluster, Hardware, Workflow XRDs |
| xrp-platform | v1.0.0 | CoreServices, PlatformServices, Application, Database XRDs |

**XRDs Established:**

| XRD | API Group | Kind |
|:----|:----------|:-----|
| TenantCluster | infrastructure.lab.gilman.io | TenantCluster |
| Hardware | infrastructure.lab.gilman.io | Hardware |
| Workflow | infrastructure.lab.gilman.io | Workflow |
| CoreServices | platform.lab.gilman.io | CoreServices |
| PlatformServices | platform.lab.gilman.io | PlatformServices |
| Application | platform.lab.gilman.io | Application |
| Database | platform.lab.gilman.io | Database |

### Next Steps

The platform is now ready for tenant cluster provisioning:

1. Define tenant cluster in `clusters/<name>/cluster.yaml`
2. Define core services in `clusters/<name>/core.yaml`
3. Define applications in `clusters/<name>/apps/*/`
4. Commit and push to Git
5. Argo CD automatically syncs and provisions

**Example workflow:**
```bash
# Create media cluster
mkdir -p clusters/media/apps/plex
cat > clusters/media/cluster.yaml <<EOF
apiVersion: infrastructure.lab.gilman.io/v1alpha1
kind: TenantCluster
metadata:
  name: media
spec:
  controlPlane:
    count: 3
    cpu: 4
    memory: 8Gi
  workers:
    count: 3
    cpu: 8
    memory: 32Gi
EOF

git add clusters/media/
git commit -m "Add media cluster"
git push

# Wait ~30 minutes for cluster to provision
# Argo CD automatically registers cluster and deploys apps
```

---

## Cross-References

- [Appendix A: Repository Structure](A_repository_structure.md) - Directory layout and organization
- [ADR-004: Platform Cluster Deployment](../09_design_decisions/ADR-004-platform-deployment.md) - Platform cluster architecture decisions
- [Concept: Crossplane Abstractions](../08_concepts/crossplane-abstractions.md) - XRD design philosophy
- [Deployment View](../07_deployment_view.md) - Physical and logical deployment architecture
- [Building Block: Tinkerbell](../05_building_blocks/tinkerbell.md) - PXE provisioning system
- [Building Block: Harvester](../05_building_blocks/harvester.md) - HCI platform
- [Genesis Runbooks](../../bootstrap/genesis/README.md) - Step-by-step command execution guide
