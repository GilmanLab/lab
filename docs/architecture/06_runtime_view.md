# 06. Runtime View

This section describes key runtime scenarios — how the system's building blocks interact during critical operations.

---

## 1. Genesis Bootstrap

The "Genesis" sequence bootstraps the entire infrastructure from bare metal to a fully operational Platform Cluster.

### Prerequisites
- Physical hardware cabled and powered
- VyOS image built with Packer (baked-in configuration)
- Synology NAS available with Talos VM capability

### Sequence

```mermaid
sequenceDiagram
    participant NAS as Synology NAS
    participant Seed as Seed Cluster
    participant Tink as Tinkerbell
    participant VyOS as VP6630 (VyOS)
    participant UM as UM760
    participant MS as MS-02 (x3)
    participant Harv as Harvester
    participant Argo as Argo CD
    participant Plat as Platform Cluster

    Note over NAS: Phase 1: Seed
    NAS->>Seed: Bootstrap single-node Talos VM
    Seed->>Argo: Deploy Argo CD (Helm)
    Seed->>Tink: Deploy Tinkerbell stack
    VyOS->>Tink: PXE boot request
    Tink->>VyOS: Provision VyOS (lab networking)
    Note over VyOS: VLANs + DHCP relay active

    Note over UM: Phase 2: Single-Node Platform
    UM->>Tink: PXE boot request (via VyOS DHCP relay)
    Tink->>UM: Provision Talos
    UM->>Seed: Join cluster
    Seed->>UM: Migrate workloads (Tinkerbell, Argo)
    NAS->>NAS: Shutdown Seed VM
    UM->>UM: Deploy Crossplane + XRDs

    Note over MS: Phase 3: Harvester Online
    MS->>Tink: PXE boot (x3)
    Tink->>MS: Provision Harvester
    MS->>Harv: Form HA cluster
    Argo->>Harv: Register as managed cluster
    Argo->>Harv: Sync clusters/harvester/

    Note over Plat: Phase 4: Full Platform
    Harv->>Harv: Create CP-2, CP-3 VMs
    Harv-->>UM: VMs PXE boot and join
    UM->>Plat: 3-node Platform Cluster formed
    Plat->>Plat: Deploy remaining services
```

### Phase Summary

| Phase | Action | Result |
|:---|:---|:---|
| **1. Seed** | Bootstrap temporary Talos on NAS, provision VyOS | Tinkerbell + Argo CD + VyOS networking operational |
| **2. Single-Node Platform** | Provision UM760, migrate from NAS | Single-node platform with Crossplane |
| **3. Harvester Online** | Provision 3x MS-02, register with Argo CD | HCI cluster managed by Argo CD |
| **4. Full Platform** | Add 2 Harvester VMs to UM760 | 3-node HA Platform Cluster |

---

## 2. Cluster Lifecycle

Downstream clusters are created, scaled, and destroyed declaratively via Git.

### Create Cluster

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant Git as GitHub
    participant Argo as Argo CD
    participant CAPI as Cluster API
    participant Harv as Harvester
    participant Talos as Talos API

    Dev->>Git: Commit Cluster manifest
    Git->>Argo: Webhook / Poll
    Argo->>CAPI: Sync Cluster CRD
    CAPI->>Harv: Create VMs (CP + Workers)
    Harv-->>CAPI: VMs running
    CAPI->>Talos: Apply machine configs
    Talos-->>CAPI: Nodes joined
    CAPI-->>Argo: Cluster Ready
    Argo->>Argo: Sync workloads to new cluster
```

### Scale Cluster

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant Git as GitHub
    participant Argo as Argo CD
    participant CAPI as Cluster API
    participant Harv as Harvester

    Dev->>Git: Update MachineDeployment replicas
    Git->>Argo: Sync
    Argo->>CAPI: Update MachineDeployment
    alt Scale Up
        CAPI->>Harv: Create new VM(s)
        Harv-->>CAPI: VMs joined cluster
    else Scale Down
        CAPI->>CAPI: Cordon & drain node
        CAPI->>Harv: Delete VM
    end
```

### Delete Cluster

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant Git as GitHub
    participant Argo as Argo CD
    participant CAPI as Cluster API
    participant Harv as Harvester

    Dev->>Git: Remove Cluster manifest
    Git->>Argo: Sync (prune enabled)
    Argo->>CAPI: Delete Cluster CRD
    CAPI->>CAPI: Delete Machines
    CAPI->>Harv: Delete VMs
    Harv-->>CAPI: Resources cleaned up
```

---

## 3. GitOps Sync Flow

All configuration changes flow through Git. This is the standard path for deploying or updating workloads.

### Application Deployment

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant Git as GitHub
    participant Argo as Argo CD (Platform)
    participant K8s as Downstream Cluster

    Dev->>Git: Commit application manifest
    Git->>Argo: Webhook notification
    Argo->>Argo: ApplicationSet detects new app
    Argo->>K8s: Apply manifests (via kubeconfig)
    K8s-->>Argo: Resources created
    Argo->>Argo: Report sync status: Healthy
```

### Sync Modes

| Mode | Description |
|:---|:---|
| **Auto-Sync** | Argo automatically applies changes on Git commit |
| **Manual Sync** | Operator triggers sync via UI/CLI (for sensitive changes) |
| **Prune** | Argo deletes resources removed from Git |
| **Self-Heal** | Argo reverts manual cluster changes to match Git |

### Drift Detection

```
┌─────────────────┐       ┌─────────────────┐
│     GitHub      │       │    Argo CD      │
│  (Desired State)│◀─────▶│ (Reconciler)    │
└─────────────────┘       └────────┬────────┘
                                   │ Compare
                                   ▼
                          ┌─────────────────┐
                          │   Cluster       │
                          │ (Actual State)  │
                          └─────────────────┘
                                   │
                          Drift? ──┼── Yes → Auto-correct
                                   └── No  → Healthy
```

If drift is detected (manual `kubectl` changes), Argo CD can:
- **Alert**: Notify operator of out-of-sync state
- **Self-Heal**: Automatically revert to Git state (if enabled)
