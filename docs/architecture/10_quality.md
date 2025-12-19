# 10. Quality Requirements

This section defines the quality goals and requirements that guide architectural decisions.

---

## Quality Tree

```
                        Quality
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
   Reliability        Operability        Security
        │                  │                  │
   ┌────┴────┐        ┌────┴────┐        ┌────┴────┐
   │         │        │         │        │         │
Availability Resilience Simplicity Automation Isolation Auditability
```

---

## Quality Goals

| Priority | Quality Goal | Motivation |
|:---:|:---|:---|
| 1 | **Reproducibility** | If the house burns down, `git clone` + bootstrap restores everything |
| 2 | **Resilience** | Survive single component failures without manual intervention |
| 3 | **Simplicity** | Bus factor of 1; complexity is the enemy of reliability |
| 4 | **Automation** | GitOps everything; manual intervention is a failure mode |
| 5 | **Security** | Lab is a "hostile entity" to the home network |

---

## Quality Scenarios

### Reliability

| ID | Scenario | Expected Response |
|:---|:---|:---|
| **R-1** | Single Harvester node fails | VMs live-migrate; service continues. Longhorn maintains quorum. |
| **R-2** | Platform Cluster VM fails | etcd quorum maintained (2/3). CAPI operations continue. |
| **R-3** | Total Harvester failure | Platform Cluster retains 1 physical node (UM760). Can orchestrate recovery. |
| **R-4** | Downstream cluster destroyed | Recreated from Git in < 30 minutes via CAPI. |

### Availability Targets

| Component | Target | Justification |
|:---|:---|:---|
| **Harvester HCI** | 99.5% | Underlying compute must be highly available |
| **Platform Cluster** | 99.5% | Factory must be available to manage infrastructure |
| **Downstream Clusters** | 99% | Workloads are non-critical (homelab) |
| **Individual Services** | Best-effort | Media streaming outages are tolerable |

> [!NOTE]
> These are aspirational targets for a homelab. There is no SLA enforcement. They guide design decisions (e.g., 3-node HA vs single-node).

### Recovery Objectives

| Metric | Target | Notes |
|:---|:---|:---|
| **RPO (Recovery Point)** | 24 hours | etcd and OpenBAO snapshots daily to NFS |
| **RTO (Recovery Time)** | 4 hours | Full Genesis rebuild from bare metal |
| **MTTR (Mean Time to Repair)** | 1 hour | Automated node replacement via CAPI MachineHealthCheck |

---

### Operability

| ID | Scenario | Expected Response |
|:---|:---|:---|
| **O-1** | Configuration change needed | Push to Git → Argo CD syncs automatically |
| **O-2** | New cluster required | Commit Cluster manifest → Provisioned in < 15 minutes |
| **O-3** | Security patch released | Update manifest in Git → Rolling upgrade with zero downtime |
| **O-4** | Administrator unavailable | System self-heals; drift auto-corrected; no degradation |

---

### Security

| ID | Scenario | Expected Response |
|:---|:---|:---|
| **S-1** | Lab node compromised | Cannot access home network (firewall DROP outbound) |
| **S-2** | Unauthorized cluster access attempt | OIDC authentication required; no anonymous access |
| **S-3** | Secret exposure | Secrets stored in OpenBAO, not in Git; dynamic credentials rotate |
| **S-4** | TLS certificate expiry | cert-manager auto-renews from OpenBAO PKI |

---

## Quality Metrics

| Metric | Collection Method | Target |
|:---|:---|:---|
| **Cluster Uptime** | Prometheus `up` metric | > 99% monthly |
| **GitOps Sync Status** | Argo CD metrics | 100% synced, < 5 min drift detection |
| **Node Health** | Prometheus node exporter | < 80% CPU/memory sustained |
| **Certificate Validity** | cert-manager metrics | > 7 days remaining |
| **Backup Success** | Custom metrics / alerts | 100% daily backups successful |
