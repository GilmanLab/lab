# 08. Concepts - Argo CD GitOps Engine

## Overview
**Argo CD** serves as the GitOps engine for the entire lab infrastructure. It runs as a single instance on the Platform Cluster and manages deployments across all clusters using a **Hub-and-Spoke** architecture.

Its primary responsibility is to sync declarative configurations from Git to Kubernetes clusters, ensuring that the desired state matches the actual state. Argo CD syncs **Claims** (simplified YAML), and Crossplane handles the complex reconciliation.

> [!IMPORTANT]
> **Design Decision**: We use a single centralized Argo CD instance rather than distributed instances. This simplifies credentials, provides a unified dashboard, and leverages the fact that all clusters exist on the same lab network (See [Multi-Cluster Argo CD Strategy](../PROPOSED_STRUCTURE.md#multi-cluster-argo-cd-strategy)).

---

## Hub-and-Spoke Architecture

The Platform Cluster runs the central Argo CD instance that manages all clusters, including itself.

```
┌───────────────────────────────────────────────────────────────┐
│                      Platform Cluster                          │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │                       Argo CD                             │ │
│  │  ┌────────────────────────────────────────────────────┐  │ │
│  │  │           Registered Cluster Secrets               │  │ │
│  │  │  • in-cluster (platform)       [Manual]            │  │ │
│  │  │  • harvester                   [Manual]            │  │ │
│  │  │  • media   ←─────────────┐                         │  │ │
│  │  │  • dev     ←─────────────┤ Created by TenantCluster│  │ │
│  │  │  • prod    ←─────────────┘ XR (Automatic)          │  │ │
│  │  └────────────────────────────────────────────────────┘  │ │
│  └─────────────────────────┬────────────────────────────────┘ │
└────────────────────────────┼──────────────────────────────────┘
                             │ Apply manifests via kubeconfig
         ┌───────────────────┼───────────────────┐
         ▼                   ▼                   ▼
    ┌─────────┐         ┌─────────┐         ┌─────────┐
    │Harvester│         │  media  │         │   dev   │
    │   HCI   │         │ (Tenant)│         │ (Tenant)│
    └─────────┘         └─────────┘         └─────────┘
```

### Cluster Registration

| Cluster Type | Registration Method | Managed By |
|:---|:---|:---|
| **Platform** | Manual Secret during bootstrap | Genesis scripts |
| **Harvester** | Manual Secret after Harvester boots | Genesis runbook |
| **Tenant** | Automatic via TenantCluster XR | Crossplane Composition |

When a `TenantCluster` XR is created, the Crossplane composition automatically generates an Argo CD cluster Secret containing the CAPI-generated kubeconfig. This enables immediate discovery and deployment.

---

## ApplicationSet Strategy

We use **two primary ApplicationSets** to manage all cluster resources:

### 1. Cluster Definitions (`cluster-definitions`)

Syncs the **root-level Claims** (`cluster.yaml`, `core.yaml`, `platform.yaml`) to the **Platform Cluster** where Crossplane processes them.

| Field | Value | Notes |
|:---|:---|:---|
| **Generator** | Git (directories) | Matches `clusters/*` |
| **Destination** | `https://kubernetes.default.svc` | Always platform (in-cluster) |
| **Recurse** | `false` | Only root YAML files |
| **Include** | `*.yaml` | Excludes subdirectories |

```yaml
# Syncs clusters/media/cluster.yaml → Platform Cluster
# Crossplane then creates the actual Kubernetes cluster
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: cluster-definitions
spec:
  generators:
    - git:
        directories:
          - path: clusters/*
  template:
    spec:
      destination:
        server: https://kubernetes.default.svc
      source:
        directory:
          recurse: false
          include: '*.yaml'
```

### 2. Cluster Apps (`cluster-apps`)

Uses a **matrix generator** to deploy applications to their target clusters.

| Generator | Purpose |
|:---|:---|
| **Clusters** | Discover registered Argo CD clusters via label selector |
| **Git** | Find `apps/*` directories for each cluster |

```yaml
# Syncs clusters/media/apps/plex/ → Media Cluster
# Syncs clusters/platform/apps/observability/ → Platform Cluster
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: cluster-apps
spec:
  generators:
    - matrix:
        generators:
          - clusters:
              selector:
                matchExpressions:
                  - key: lab.gilman.io/cluster-name
                    operator: Exists
          - git:
              directories:
                - path: 'clusters/{{name}}/apps/*'
  template:
    spec:
      destination:
        server: '{{server}}'  # Actual cluster endpoint from cluster Secret
```

This pattern ensures apps are deployed to the **correct cluster** (not always the platform).

---

## Sync Waves

Argo CD supports **sync waves** to control the order of resource deployment within an Application.

### Standard Wave Order

| Wave | Resources | Purpose |
|:---|:---|:---|
| **0** | Namespaces, CRDs, XRDs | Foundation layer |
| **1** | Crossplane Configurations | Install API definitions |
| **2** | Core Services (XR Claims) | Base layer (Cilium, cert-manager) |
| **3** | Platform Services (XR Claims) | Shared services (Zitadel, OpenBAO) |
| **4** | Applications (XR Claims) | Tenant workloads |

### Example Annotation

```yaml
apiVersion: lab.gilman.io/v1alpha1
kind: CoreServices
metadata:
  name: platform
  annotations:
    argocd.argoproj.io/sync-wave: "2"  # Deploy after XRDs
spec:
  version: "1.2.0"
```

> [!NOTE]
> Sync waves are **relative** within a single Application. Use them to order resources that have dependencies (e.g., Namespace before Deployment).

---

## Integration with Crossplane

Argo CD and Crossplane operate in a **division of labor**:

| Layer | Responsibility | Tool |
|:---|:---|:---|
| **Sync** | Ensure Git → Cluster consistency | Argo CD |
| **Composition** | Expand Claims → Low-level resources | Crossplane |
| **Actuation** | Apply low-level resources | Kubernetes controllers (Helm, CAPI, etc.) |

### Workflow Example

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Git Commit  │────▶│  Argo CD     │────▶│  Crossplane  │────▶│   CAPI       │
│              │     │  (Syncs)     │     │  (Composes)  │     │  (Actuates)  │
│ cluster.yaml │     │              │     │              │     │              │
│ (TenantCluster)    │ TenantCluster│     │ → Cluster    │     │ → Harvester  │
│              │     │   XR Claim   │     │ → Machine    │     │   VMs        │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
```

1. Developer commits `clusters/media/cluster.yaml` (TenantCluster XR Claim)
2. Argo CD detects change and syncs to Platform Cluster
3. Crossplane sees the Claim and creates CAPI resources
4. CAPI Harvester Provider provisions VMs on Harvester
5. Cluster becomes ready; Crossplane creates Argo CD cluster Secret
6. Argo CD automatically discovers new cluster and deploys apps

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                           Git Repository                             │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │  clusters/                                                      │ │
│  │  ├── platform/                                                  │ │
│  │  │   ├── core.yaml         ← Crossplane, CAPI, cert-manager    │ │
│  │  │   ├── platform.yaml     ← Zitadel, OpenBAO                  │ │
│  │  │   └── apps/             ← Observability, Tinkerbell         │ │
│  │  ├── harvester/                                                 │ │
│  │  │   ├── config/           ← Networks, Images (Harvester CRDs) │ │
│  │  │   └── vms/              ← CP-2, CP-3 VMs                     │ │
│  │  ├── media/                                                     │ │
│  │  │   ├── cluster.yaml      ← TenantCluster XR                  │ │
│  │  │   ├── core.yaml         ← CoreServices XR                   │ │
│  │  │   └── apps/             ← Plex, Jellyfin                    │ │
│  │  └── dev/                                                       │ │
│  │      ├── cluster.yaml                                           │ │
│  │      └── apps/                                                  │ │
│  └────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────┬────────────────────────────────────────┘
                              │
                              ▼
          ┌───────────────────────────────────────────┐
          │        Platform Cluster (Hub)             │
          │  ┌─────────────────────────────────────┐  │
          │  │          Argo CD                    │  │
          │  │  ┌────────────────────────────────┐ │  │
          │  │  │  ApplicationSets               │ │  │
          │  │  │  • cluster-definitions         │ │  │
          │  │  │  • cluster-apps                │ │  │
          │  │  └────────────────────────────────┘ │  │
          │  └─────────────────────────────────────┘  │
          │                                            │
          │  ┌─────────────────────────────────────┐  │
          │  │         Crossplane                  │  │
          │  │  • TenantCluster XRD                │  │
          │  │  • CoreServices XRD                 │  │
          │  │  • Application XRD                  │  │
          │  └─────────────────────────────────────┘  │
          └───────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          ▼                   ▼                   ▼
    ┌──────────┐        ┌──────────┐        ┌──────────┐
    │Harvester │        │  media   │        │   dev    │
    │Raw CRDs  │        │XR Claims │        │XR Claims │
    └──────────┘        └──────────┘        └──────────┘
```

---

## Access and Monitoring

| Feature | Endpoint |
|:---|:---|
| **UI** | `https://argocd.lab.local` |
| **CLI** | `argocd` (installed via package manager) |
| **Authentication** | OIDC via Zitadel |
| **RBAC** | Admin-only (single operator) |

### Key Metrics (Prometheus)

| Metric | Purpose |
|:---|:---|
| `argocd_app_sync_status` | Application sync state (Synced/OutOfSync) |
| `argocd_app_health_status` | Application health (Healthy/Degraded) |
| `argocd_app_sync_total` | Sync operation count |
| `argocd_app_reconcile_count` | Reconciliation frequency |

> [!TIP]
> Use the Argo CD UI to visualize the dependency graph of Crossplane compositions. This helps debug XR expansion issues.

---

## Operational Notes

### Manual vs Automatic Sync

| Mode | When to Use |
|:---|:---|
| **Automatic** | Production clusters (platform, tenant apps) |
| **Manual** | Bootstrap phase, risky changes |

All production ApplicationSets use `syncPolicy.automated.selfHeal: true` to prevent configuration drift.

### Secrets Management

Argo CD does **not** manage secrets directly. Secrets are injected via:
- **Vault Secrets Operator (VSO)**: Syncs secrets from OpenBAO into Kubernetes
- **External Secrets Operator (ESO)**: Alternative to VSO (not currently used)

### Disaster Recovery

In a full platform rebuild:
1. Restore etcd backup (contains Argo CD state)
2. Argo CD Applications automatically re-sync from Git
3. Crossplane reprocesses all XR Claims
4. Clusters and apps return to desired state

> [!IMPORTANT]
> Git is the **source of truth**. Argo CD is stateless; all state lives in Git or Kubernetes etcd.
