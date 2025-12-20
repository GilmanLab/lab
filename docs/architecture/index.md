# Homelab Architecture Documentation

## At a Glance

**What**: A bare-metal Platform Engineering lab running multi-cluster Kubernetes on Harvester HCI, fully managed via GitOps.

**Stack**: Tinkerbell (PXE) → Harvester (VMs) → Talos Linux (K8s OS) → Argo CD (GitOps) → Crossplane (Platform API)

**Hardware**: 3× MS-02 nodes (Harvester), 1× UM760 (Platform anchor), VyOS gateway, Synology NAS

**Clusters**: Platform Cluster (brain) manages ephemeral Tenant Clusters via Cluster API

**Principles**: Reproducible (`git clone` + bootstrap), Immutable (no SSH), GitOps (Argo CD is truth), Isolated (firewalled from home network)

---

## Find What You Need

### By Task

| I want to... | Start here |
|:-------------|:-----------|
| Understand the project vision and goals | [01. Introduction and Goals](01_introduction_and_goals.md) |
| Bootstrap the lab from scratch | [Appendix B: Bootstrap Procedure](appendices/B_bootstrap_procedure.md) |
| Add or modify a cluster | [Platform Cluster](05_building_blocks/03_platform_cluster.md) → [Downstream Clusters](05_building_blocks/04_downstream_clusters.md) |
| Configure networking or VLANs | [Networking Concept](08_concepts/networking.md) → [ADR 002: Networking Topology](09_design_decisions/002_networking_topology.md) |
| Set up or debug Argo CD | [Argo CD Concept](08_concepts/argocd.md) → [Appendix C: Argo CD Configuration](appendices/C_argocd_configuration.md) |
| Configure Harvester VMs or storage | [Harvester HCI](05_building_blocks/01_harvester_hci.md) → [Appendix D: Harvester Configuration](appendices/D_harvester_configuration.md) |
| Understand load balancing / BGP | [Load Balancing Concept](08_concepts/load_balancing.md) → [ADR 001: BGP Load Balancing](09_design_decisions/001_use_bgp_loadbalancing.md) |
| Manage secrets or identity | [Secrets and Identity](08_concepts/secrets_and_identity.md) |
| Look up a term or VLAN | [Glossary](12_glossary.md) (includes VLAN table, hardware reference) |

### By Component

| Component | What it does | Docs |
|:----------|:-------------|:-----|
| **Harvester** | HCI layer providing VMs (KubeVirt) + storage (Longhorn) | [Building Block](05_building_blocks/01_harvester_hci.md), [Concept](08_concepts/harvester.md), [Config](appendices/D_harvester_configuration.md) |
| **Tinkerbell** | PXE boot and bare-metal provisioning | [Building Block](05_building_blocks/02_tinkerbell_provisioning.md) |
| **Platform Cluster** | Management cluster running CAPI, Argo CD, Crossplane | [Building Block](05_building_blocks/03_platform_cluster.md) |
| **Tenant Clusters** | Ephemeral workload clusters | [Building Block](05_building_blocks/04_downstream_clusters.md) |
| **Argo CD** | GitOps engine, hub-and-spoke model | [Concept](08_concepts/argocd.md), [ADR 004](09_design_decisions/004_argocd_hub_spoke.md), [Config](appendices/C_argocd_configuration.md) |
| **Crossplane** | Unified control plane with XRDs | [Concept](08_concepts/control_plane.md) |
| **Cilium** | eBPF CNI with BGP for LoadBalancer VIPs | [Networking](08_concepts/networking.md), [Load Balancing](08_concepts/load_balancing.md) |
| **Istio Ambient** | Sidecar-less service mesh (ztunnel + waypoints) | [Service Mesh](08_concepts/service_mesh.md) |
| **VyOS** | Lab gateway router with BGP, DHCP, NAT | [ADR 003](09_design_decisions/003_vyos_gitops.md) |
| **OpenBAO + Zitadel** | Secrets/PKI and identity/SSO | [Secrets and Identity](08_concepts/secrets_and_identity.md) |
| **Longhorn** | Distributed block storage | [Storage](08_concepts/storage.md) |

---

## Quick Reference

### Network (VLANs)

| VLAN | Name | Subnet | Purpose |
|:-----|:-----|:-------|:--------|
| 10 | LAB_MGMT | 10.10.10.0/24 | Harvester hosts, switch |
| 20 | LAB_PROV | 10.10.20.0/24 | Tinkerbell/PXE |
| 30 | LAB_PLATFORM | 10.10.30.0/24 | Platform Cluster |
| 40 | LAB_CLUSTER | 10.10.40.0/24 | Tenant Clusters |
| 50 | LAB_SERVICE | 10.10.50.0/24 | LoadBalancer VIPs |
| 60 | LAB_STORAGE | 10.10.60.0/24 | Longhorn replication |

### Key ADRs

| Decision | Summary |
|:---------|:--------|
| [ADR 001](09_design_decisions/001_use_bgp_loadbalancing.md) | BGP over L2 for load balancing (Cilium advertises VIPs to VyOS) |
| [ADR 002](09_design_decisions/002_networking_topology.md) | LACP bonding over physical segregation |
| [ADR 003](09_design_decisions/003_vyos_gitops.md) | Ansible + GitHub Actions + Tailscale for VyOS |
| [ADR 004](09_design_decisions/004_argocd_hub_spoke.md) | Hub-and-spoke Argo CD (not agent model) |
| [ADR 005](09_design_decisions/005_seed_bootstrap_strategy.md) | Minimal seed cluster with raw manifests |
| [ADR 006](09_design_decisions/006_harvester_managed_cluster.md) | Harvester as Argo CD managed cluster |

---

## Full Document Index

### Core Architecture (arc42)

| # | Section | Description |
|:--|:--------|:------------|
| 01 | [Introduction and Goals](01_introduction_and_goals.md) | Vision, motivation, design philosophy |
| 02 | [Constraints](02_constraints.md) | Hardware, integration, and operational constraints |
| 03 | [Context and Scope](03_context_and_scope.md) | System boundaries, external interfaces |
| 04 | [Solution Strategy](04_solution_strategy.md) | Strategic decisions, technology stack |
| 05 | [Building Blocks](05_building_blocks/) | Component architecture (4 docs) |
| 06 | [Runtime View](06_runtime_view.md) | Key runtime scenarios and workflows |
| 07 | [Deployment View](07_deployment_view.md) | Physical and virtual topology |
| 08 | [Cross-cutting Concepts](08_concepts/) | Architectural patterns (10 docs) |
| 09 | [Design Decisions](09_design_decisions/) | ADRs (6 docs) |
| 10 | [Quality Requirements](10_quality.md) | SLAs, availability, quality scenarios |
| 11 | [Risks and Technical Debt](11_risks.md) | Known risks, failure modes, debt register |
| 12 | [Glossary](12_glossary.md) | Terms, VLANs, hardware, abbreviations |

### Building Blocks (Section 05)

| Doc | Component | Key Content |
|:----|:----------|:------------|
| [01](05_building_blocks/01_harvester_hci.md) | Harvester HCI | VM management, Longhorn storage, network config |
| [02](05_building_blocks/02_tinkerbell_provisioning.md) | Tinkerbell | PXE boot, workflow templates, Smee/Hegel/Rufio |
| [03](05_building_blocks/03_platform_cluster.md) | Platform Cluster | CAPI, Argo CD, Crossplane, identity services |
| [04](05_building_blocks/04_downstream_clusters.md) | Downstream Clusters | Tenant cluster lifecycle, workload isolation |

### Cross-cutting Concepts (Section 08)

| Doc | Concept | Key Content |
|:----|:--------|:------------|
| [argocd](08_concepts/argocd.md) | Argo CD | Hub-and-spoke, ApplicationSets, health checks |
| [control_plane](08_concepts/control_plane.md) | Control Plane | Crossplane XRDs, TenantCluster/PlatformService |
| [harvester](08_concepts/harvester.md) | Harvester | HCI integration, managed as Argo CD cluster |
| [load_balancing](08_concepts/load_balancing.md) | Load Balancing | BGP, Cilium, VyOS ECMP |
| [networking](08_concepts/networking.md) | Networking | VLANs, Split Plane, Hybrid Trunk |
| [observability](08_concepts/observability.md) | Observability | Prometheus, Grafana, alerting |
| [secrets_and_identity](08_concepts/secrets_and_identity.md) | Secrets & Identity | OpenBAO, Zitadel, PKI |
| [service_mesh](08_concepts/service_mesh.md) | Service Mesh | Istio Ambient, ztunnel, waypoints |
| [storage](08_concepts/storage.md) | Storage | Longhorn block, NFS tiered |

### Appendices

| Doc | Content |
|:----|:--------|
| [A. Repository Structure](appendices/A_repository_structure.md) | Monorepo layout, directory conventions |
| [B. Bootstrap Procedure](appendices/B_bootstrap_procedure.md) | 4-phase, 18-step genesis runbook |
| [C. Argo CD Configuration](appendices/C_argocd_configuration.md) | ApplicationSets, cluster secrets, health checks |
| [D. Harvester Configuration](appendices/D_harvester_configuration.md) | Network, storage, VM CRD examples |
