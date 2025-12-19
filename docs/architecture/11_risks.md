# 11. Risks and Technical Debt

This section documents known risks, potential issues, and areas of technical debt in the architecture.

---

## Risk Register

### High Impact Risks

| ID | Risk | Probability | Impact | Mitigation |
|:---|:---|:---:|:---:|:---|
| **R-01** | **Harvester Single Cluster** — All VMs depend on one Harvester cluster | Medium | High | 3-node HA; UM760 physical node survives total Harvester loss |
| **R-02** | **NAS Single Point of Failure** — Synology NAS hosts backups, ISOs, bootstrap images | Low | Medium | Critical data on RAID; synced to iDrive for off-premise backup |
| **R-03** | **Genesis Bootstrap Complexity** — Multi-phase bootstrap has many failure points | Medium | Medium | Documented runbook; each phase is idempotent and recoverable |
| **R-04** | **Bus Factor of 1** — Single administrator; no knowledge transfer | High | High | Comprehensive documentation (this repo); all config in Git |

### Medium Impact Risks

| ID | Risk | Probability | Impact | Mitigation |
|:---|:---|:---:|:---:|:---|
| **R-05** | **vPro/AMT Reliability** — OOB management only works on 2.5G ports | Low | Medium | Alternative: manual power cycle; Split Plane design accommodates this |
| **R-06** | **Crossplane Complexity** — XRD abstraction layer adds operational overhead | Medium | Medium | Strong typing (Go functions); comprehensive testing; fallback to raw CAPI |
| **R-07** | **Upstream Dependency Changes** — Harvester/Talos/Cilium breaking changes | Medium | Medium | Pin versions; test upgrades in non-production cluster first |
| **R-08** | **Network Partition** — VLAN misconfiguration isolates components | Low | Medium | Infrastructure as Code (VyOS config in Git); monitoring for connectivity |

### Low Impact Risks

| ID | Risk | Probability | Impact | Mitigation |
|:---|:---|:---:|:---:|:---|
| **R-09** | **Power Outage** — Extended residential power loss | Low | Low | Core networking and UM760 on UPS (critical); MS-02s on UPS non-critical tier (load-shed in extended outages) |
| **R-10** | **Storage Exhaustion** — NVMe fills up with logs/images | Low | Low | Monitoring alerts; automated cleanup policies |

---

## Technical Debt Register

### Current Debt

| ID | Debt Item | Severity | Effort | Notes |
|:---|:---|:---:|:---:|:---|
| **TD-01** | **Manual VyOS Configuration** — Gateway config is not fully GitOps | Medium | Medium | Plan documented in [ADR 003](../09_design_decisions/003_vyos_gitops.md) |
| **TD-02** | **Missing Disaster Recovery Runbooks** — Backup strategy defined but not recovery procedures | Medium | Low | Create operational runbooks in appendix |
| **TD-03** | **Terminology Inconsistency** — "Downstream"/"Tenant"/"Workload" cluster used interchangeably | Low | Low | Standardize on "Tenant Cluster" |

### Planned Remediation

| Debt ID | Target Date | Approach |
|:---|:---|:---|
| TD-01 | Q2 2025 | Implement GitHub Actions + Ansible + Tailscale per [ADR 003](../09_design_decisions/003_vyos_gitops.md) |
| TD-02 | Q1 2025 | Write runbooks as part of appendix effort |
| TD-03 | Immediate | Search/replace standardization pass |

---

## Failure Mode Analysis

### Bootstrap Failure Modes

| Phase | Failure | Recovery |
|:---|:---|:---|
| **Seed** | NAS VM fails to start | Verify NAS resources; can use alternative host (laptop) |
| **Pivot** | UM760 PXE fails | Check VLAN 20 connectivity; verify Tinkerbell is responding |
| **HCI** | MS-02 won't boot Harvester | Verify vPro/AMT; check Tinkerbell workflow; manual ISO boot fallback |
| **Platform HA** | VM expansion fails | CAPI reconciles automatically; check Harvester resources |

### Runtime Failure Modes

| Component | Failure | Detection | Recovery |
|:---|:---|:---|:---|
| **Harvester Node** | Hardware failure | Prometheus alerts | Live migration; MachineHealthCheck replaces |
| **Platform etcd** | Quorum loss | API server unavailable | Restore from backup; requires 2+ nodes |
| **Argo CD** | Crash/unavailable | Sync stops | Kubernetes restarts pod; manual intervention rare |
| **OpenBAO** | Sealed/unavailable | Auth failures | Auto-unseal configured; Raft leader election |
| **Downstream Cluster** | Total loss | Metrics gap | Delete/recreate from Git |

---

## Security Considerations

| Area | Concern | Current State | Recommendation |
|:---|:---|:---|:---|
| **Network Isolation** | Lab → Home access | Firewalled DROP | ✅ Adequate |
| **Secrets in Git** | Credential exposure | Secrets in OpenBAO only | ✅ Adequate |
| **OIDC Tokens** | Token theft | Short-lived; refresh via Zitadel | ✅ Adequate |
| **Talos API** | Unauthorized access | mTLS required; keys not in Git | ✅ Adequate |
| **Kubernetes RBAC** | Overprivileged users | OIDC groups map to roles | 🟡 Review role bindings |
| **Image Provenance** | Supply chain attack | Public registries used | 🟡 Consider signing/verification |

---

## Monitoring & Alerting Gaps

| Gap | Risk | Recommended Action |
|:---|:---|:---|
| No dashboard for backup status | Silent backup failures | Add Grafana panel for backup metrics |
| No alert for certificate expiry | Service outage | cert-manager exposes metrics; add alert |
| No synthetic monitoring | Undetected service degradation | Consider blackbox exporter probes |
