# 08. Concepts - Observability

## Overview
Centralized **Observability** provides visibility into the health, performance, and behavior of all clusters and workloads. The architecture follows a hub-and-spoke model where downstream clusters report to the Platform Cluster.

---

## Architecture

### Hub-and-Spoke Model

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Platform Cluster (Hub)                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                        Observability Stack                         │ │
│  │   ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐   │ │
│  │   │ Prometheus │  │   Grafana  │  │   Loki     │  │Alertmanager│   │ │
│  │   │  (metrics) │  │   (viz)    │  │   (logs)   │  │  (alerts)  │   │ │
│  │   └──────▲─────┘  └────────────┘  └──────▲─────┘  └────────────┘   │ │
│  └──────────┼────────────────────────────────┼────────────────────────┘ │
└─────────────┼────────────────────────────────┼──────────────────────────┘
              │ Remote Write                    │ Push (Promtail)
     ┌────────┴────────┬───────────────────────┴────────┐
     │                 │                                 │
┌────▼─────┐     ┌─────▼────┐                     ┌──────▼─────┐
│ Tenant 1 │     │ Tenant 2 │                     │  Harvester │
│ (media)  │     │  (dev)   │                     │    HCI     │
└──────────┘     └──────────┘                     └────────────┘
```

---

## Components

### Metrics: Prometheus

| Deployment | Location | Purpose |
|:---|:---|:---|
| **Central Prometheus** | Platform Cluster | Aggregates metrics from all sources |
| **Cluster Agents** | Each Tenant Cluster | Collects local metrics, remote-writes to central |

#### Collection Targets

| Source | Exporter | Key Metrics |
|:---|:---|:---|
| **Kubernetes** | kube-state-metrics | Pod/deployment status, resource usage |
| **Nodes** | node_exporter | CPU, memory, disk, network |
| **Cilium** | Built-in `/metrics` | Network flows, policy decisions |
| **cert-manager** | Built-in `/metrics` | Certificate expiry, issuance status |
| **Argo CD** | Built-in `/metrics` | Sync status, application health |
| **Harvester** | Built-in | VM status, storage capacity |

### Visualization: Grafana

| Access | URL |
|:---|:---|
| **Platform Grafana** | `https://grafana.lab.local` |
| **Authentication** | OIDC via Zitadel |

#### Standard Dashboards

| Dashboard | Purpose |
|:---|:---|
| **Cluster Overview** | Multi-cluster health summary |
| **Node Health** | CPU, memory, disk per node |
| **Kubernetes Workloads** | Deployment, pod, container metrics |
| **Networking** | Cilium flow logs, bandwidth |
| **Storage** | Longhorn volume health, IOPS |
| **GitOps** | Argo CD sync status, drift detection |

### Logs: Loki (Optional)

| Component | Role |
|:---|:---|
| **Loki** | Log aggregation and query engine |
| **Promtail** | Log collector deployed as DaemonSet on each cluster |

> [!NOTE]
> Loki is optional for the initial deployment. Talos Linux has limited logging (no traditional syslog), and Kubernetes logs can be queried via `kubectl logs`. Loki becomes valuable for long-term log retention and cross-cluster queries.

### Alerting: Alertmanager

| Feature | Configuration |
|:---|:---|
| **Receivers** | Discord webhook, email (optional) |
| **Silencing** | Maintenance windows via UI |
| **Grouping** | Alerts grouped by cluster, severity |

---

## Metric Flow

### Remote Write Pattern

Prometheus instances in tenant clusters **remote-write** to the central Prometheus:

```yaml
# Prometheus in tenant cluster
remoteWrite:
  - url: https://prometheus.platform.lab.local/api/v1/write
    headers:
      Authorization: Bearer <token>
```

### Why Remote Write?

| Approach | Pros | Cons |
|:---|:---|:---|
| **Federation** | Simple query aggregation | Central Prometheus must scrape all clusters |
| **Remote Write** | Push-based; works through NAT/firewall | Requires central storage capacity |

We use **Remote Write** because tenant clusters push to the Platform, avoiding complex network routing for pull-based scraping.

---

## Access

| Service | URL | Auth |
|:---|:---|:---|
| **Grafana** | `https://grafana.lab.local` | OIDC (Zitadel) |
| **Prometheus UI** | `https://prometheus.lab.local` | OIDC (Zitadel) |
| **Alertmanager** | `https://alertmanager.lab.local` | OIDC (Zitadel) |

---

## Alerting Strategy

### Severity Levels

| Level | Response | Example |
|:---|:---|:---|
| **Critical** | Immediate (page) | etcd quorum lost, Platform Cluster down |
| **Warning** | Same-day review | Disk 80% full, certificate expires in 7 days |
| **Info** | Weekly review | Deployment scaled, node rebooted |

### Key Alerts

| Alert | Condition | Severity |
|:---|:---|:---|
| **ClusterDown** | Prometheus target unreachable for 5m | Critical |
| **NodeNotReady** | Kubernetes node NotReady for 10m | Warning |
| **PodCrashLooping** | Pod restart count > 5 in 10m | Warning |
| **PVCAlmostFull** | PVC usage > 85% | Warning |
| **CertificateExpiring** | cert-manager cert expires in < 7 days | Warning |
| **ArgoCDOutOfSync** | Application out of sync for > 30m | Warning |
| **BackupFailed** | Backup job failed | Critical |

---

## Integration with Platform Services

| Service | Observability Integration |
|:---|:---|
| **OpenBAO** | Audit logs, seal status metrics |
| **Zitadel** | Authentication metrics, login failures |
| **cert-manager** | Certificate lifecycle metrics |
| **Argo CD** | Sync status, health metrics |
| **CAPI** | Cluster/machine provisioning metrics |
