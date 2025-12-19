# Homelab Architecture Documentation

> A Platform Engineering laboratory on bare-metal hardware, following the [arc42](https://arc42.org/) documentation template.

---

## Overview

This documentation describes a fully automated, GitOps-focused, multi-cluster Talos Kubernetes architecture backed by Rancher Harvester. It serves as both an operational reference and a research platform for Platform Engineering concepts.

---

## Document Index

### Core Architecture (arc42)

| Section | Description |
|:---|:---|
| [01. Introduction and Goals](01_introduction_and_goals.md) | Vision, motivation, design philosophy |
| [02. Constraints](02_constraints.md) | Hardware, integration, and operational constraints |
| [03. Context and Scope](03_context_and_scope.md) | System boundaries, external interfaces |
| [04. Solution Strategy](04_solution_strategy.md) | Strategic decisions, technology stack |
| [05. Building Blocks](05_building_blocks/) | Component architecture (see below) |
| [06. Runtime View](06_runtime_view.md) | Key runtime scenarios and workflows |
| [07. Deployment View](07_deployment_view.md) | Physical and virtual topology |
| [08. Cross-cutting Concepts](08_concepts/) | Architectural patterns (see below) |
| [09. Design Decisions](09_design_decisions/) | ADRs (see below) |
| [10. Quality Requirements](10_quality.md) | SLAs, availability, quality scenarios |
| [11. Risks and Technical Debt](11_risks.md) | Known risks, failure modes, debt register |
| [12. Glossary](12_glossary.md) | Terms, abbreviations, reference tables |

### Building Blocks (Section 05)

| Component | Description |
|:---|:---|
| [Harvester HCI](05_building_blocks/01_harvester_hci.md) | Hyper-converged infrastructure layer |
| [Tinkerbell Provisioning](05_building_blocks/02_tinkerbell_provisioning.md) | Bare-metal provisioning engine |
| [Platform Cluster](05_building_blocks/03_platform_cluster.md) | Central management cluster |
| [Downstream Clusters](05_building_blocks/04_downstream_clusters.md) | Tenant workload clusters |

### Cross-cutting Concepts (Section 08)

| Concept | Description |
|:---|:---|
| [Control Plane](08_concepts/control_plane.md) | Crossplane unified API layer |
| [Load Balancing](08_concepts/load_balancing.md) | BGP-based service exposure |
| [Networking](08_concepts/networking.md) | VLAN architecture, Split Plane design |
| [Observability](08_concepts/observability.md) | Prometheus, Grafana, alerting |
| [Secrets and Identity](08_concepts/secrets_and_identity.md) | OpenBAO and Zitadel integration |
| [Service Mesh](08_concepts/service_mesh.md) | Istio Ambient + Cilium layered model |
| [Storage](08_concepts/storage.md) | Longhorn and NFS tiered storage |

### Design Decisions (Section 09)

| ADR | Decision |
|:---|:---|
| [ADR 001](09_design_decisions/001_use_bgp_loadbalancing.md) | BGP over Layer 2 for load balancing |
| [ADR 002](09_design_decisions/002_networking_topology.md) | LACP bonding over physical segregation |
| [ADR 003](09_design_decisions/003_vyos_gitops.md) | Ansible + GitHub Actions + Tailscale for VyOS management |

---

## Quick Start

1. **Understand the Vision**: Start with [Introduction and Goals](01_introduction_and_goals.md)
2. **Know the Constraints**: Review [Constraints](02_constraints.md) for hardware context
3. **Learn the Stack**: Study [Solution Strategy](04_solution_strategy.md) for technology choices
4. **Explore Components**: Dive into [Building Blocks](05_building_blocks/) for component details
5. **Reference Terms**: Consult the [Glossary](12_glossary.md) for unfamiliar terminology

---

## Key Principles

- **Reproducibility**: Everything is in Git; `git clone` + bootstrap restores the lab
- **Immutability**: Talos Linux has no SSH; nodes are replaced, not patched
- **GitOps**: Argo CD is the single source of truth
- **Isolation**: Lab is firewalled from the home network
