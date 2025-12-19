# 04. Solution Strategy

This section details the fundamental architectural patterns chosen to achieve the goals of Reproducibility and Resilience.

## Strategic Pillars

### 1. The "Management Cluster" Pattern (CAPI)
Instead of treating Kubernetes clusters as pets, we treat them as resources.
*   **Strategy**: A central, permanent **Platform Cluster** is the "Factory" that produces downstream clusters.
*   **Mechanism**: **Cluster API (CAPI)**. The Platform Cluster holds the CAPI controllers. It talks to the Harvester API to deliver VMs and the Talos API to bootstrap them.
*   **Benefit**: Downstream clusters can be blown away and recreated with a single `kubectl delete cluster` command (or Git commit).

### 2. Immutable Operating System (Talos)
We reject the notion of managing Linux distributions (Ubuntu/Debian) with generic configuration management.
*   **Strategy**: Use **Talos Linux**, a minimal, hardened, and API-managed OS.
*   **Implication**: No SSH access. Updates are performed by swapping the OS image (A/B partitioning) initiated via API.
*   **Benefit**: Eliminates "Configuration Drift" at the OS layer. A node is either in the desired state or it is replaced.

### 3. Hyper-Converged Infrastructure (HCI)
We require the flexibility of virtualization but the performance of bare metal.
*   **Strategy**: **Rancher Harvester** provides a unified layer for Compute (KubeVirt) and Storage (Longhorn).
*   **Implication**: The physical layer is abstracted. We can resize Control Planes, add Worker nodes, or snapshot entire clusters without touching cables.
*   **Integration**: Harvester *is* a Kubernetes cluster itself, making it native to our tooling stack.

### 4. GitOps at the Core
The state of the system must match the state of the repo.
*   **Strategy**: **Argo CD** is the engine.
*   **Scope**: Argo manages the *Platform* applications, the *Cluster Definitions* (CAPI YAMLs), and the *Downstream* applications.
*   **App of Apps**: A hierarchical pattern is used to cascade configurations from the root repository to all child clusters.

### 5. Unified Control Plane (Crossplane)
We treat Infrastructure as an API.
*   **Strategy**: **Crossplane v2** provides the high-level API surface for the platform.
*   **Mechanism**: **Composite Resource Definitions (XRDs)**. We define abstract resources like `TenantCluster` (Infrastructure) and `PlatformService` (Applications).
*   **Benefit**: Simplifies GitOps. Instead of managing complex Helm values or raw CAPI objects, we manage simple, high-level "Claims".
*   **Topology**: Deployed on **Both** Platform (Infra) and Tenant (Apps) clusters.

## Technology Stack Summary

| Layer | Technology | Justification |
| :--- | :--- | :--- |
| **Bare Metal Provisioning** | **Tinkerbell** | GitOps-friendly, Docker-based workflow engine. Works well with scarce DHCP environments (Smee). |
| **Hypervisor** | **Harvester** | Built on K8s. Native integration with Rancher/CAPI. Includes KubeVirt for VM management. |
| **Storage** | **Longhorn** | Distributed block storage with replication. Integrated with Harvester; available to all clusters via CSI. |
| **Kubernetes OS** | **Talos Linux** | Security, Immutability, Minimal footprint. API-driven management, no SSH. |
| **Gateway** | **VyOS** | Lab router providing inter-VLAN routing, DHCP, NAT, and BGP peering with clusters. |
| **Networking** | **Cilium** | eBPF-based CNI. High performance, built-in observability (Hubble), security policies, and BGP for LoadBalancer VIPs. |
| **Service Mesh** | **Istio Ambient** | Sidecar-less mesh using ztunnel. Provides mTLS, traffic management, and observability without per-pod proxies. |
| **Orchestration** | **Cluster API** | The standard for declarative cluster lifecycle management. Uses Harvester and Talos providers. |
| **Control Plane** | **Crossplane** | Unified API layer (v2). Exposes high-level XRDs (`TenantCluster`, `PlatformService`) to simplify GitOps. |
| **GitOps** | **Argo CD** | Industry standard, robust UI, excellent Helm/Kustomize support. Source of truth for all clusters. |
| **Identity** | **Zitadel** | Centralized OIDC/SAML provider. SSO for Argo CD, Grafana, kubectl, and Harvester. |
| **Secrets & PKI** | **OpenBAO** | Vault fork for secrets management, dynamic credentials, and PKI. Injected via Vault Agent sidecars. |
| **Certificates** | **cert-manager** | Automated certificate lifecycle. Integrates with OpenBAO PKI backend for TLS issuance. |
| **Database** | **CloudNativePG** | Kubernetes-native PostgreSQL operator. Production-ready clusters with automated backups and failover. |
| **Observability** | **Prometheus + Grafana** | Centralized metrics collection and visualization. Downstream clusters report to Platform. |
