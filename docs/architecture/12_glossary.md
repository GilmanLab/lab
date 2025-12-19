# 12. Glossary

This glossary defines domain-specific terms and abbreviations used throughout the architecture documentation.

---

## Architecture Terms

| Term | Definition |
|:---|:---|
| **Genesis** | The multi-phase bootstrap sequence that provisions the entire infrastructure from bare metal to operational Platform Cluster. Phases: Seed → Pivot → HCI → Platform HA → Steady State. |
| **Platform Cluster** | The permanent, privileged Kubernetes cluster that acts as the "brain" of the lab. Hosts CAPI, Argo CD, Zitadel, OpenBAO, and Crossplane. Also called "Management Cluster" in CAPI terminology. |
| **Tenant Cluster** | An ephemeral, workload-focused Kubernetes cluster provisioned and managed by the Platform Cluster. Treated as "cattle" — easily created and destroyed via GitOps. |
| **Seed Cluster** | A temporary, single-node Talos Kubernetes cluster bootstrapped on the NAS to host Tinkerbell during the Genesis sequence. Destroyed after the Pivot phase. |
| **Split Plane** | The MS-02 network topology where the 2.5GbE port handles OOB/Provisioning (VLAN 20) while the 10GbE SFP+ ports handle Data Plane traffic (VLANs 10/30/40/50/60). |
| **Hybrid Trunk** | The UM760 network configuration where a single NIC carries both native VLAN 20 (for PXE boot) and tagged VLANs (30/40/50) for cluster traffic. |
| **OOB (Out-of-Band)** | Management traffic that occurs on a separate network from production data. Used for vPro/AMT remote management and PXE provisioning. |

---

## Technology Terms

| Term | Definition |
|:---|:---|
| **CAPI (Cluster API)** | A Kubernetes sub-project for declarative cluster lifecycle management. Uses CRDs to define clusters, machines, and infrastructure. |
| **Crossplane** | A Kubernetes add-on that extends the Kubernetes API with custom resources (XRDs) to manage infrastructure and services declaratively. |
| **XRD (Composite Resource Definition)** | A Crossplane concept that defines a custom API type (like `TenantCluster`) and how it composes into lower-level resources. |
| **Harvester** | An open-source HCI (Hyper-Converged Infrastructure) solution built on Kubernetes, KubeVirt, and Longhorn. Provides VM and storage management. |
| **Talos Linux** | A minimal, immutable Linux distribution designed for Kubernetes. Managed entirely via API — no SSH, no shell, no package manager. |
| **Tinkerbell** | A bare-metal provisioning engine providing PXE boot, metadata services, and workflow orchestration for installing operating systems on physical hardware. |
| **Longhorn** | A lightweight, distributed block storage system for Kubernetes. Provides replicated persistent volumes using local disks. |
| **Cilium** | An eBPF-based CNI (Container Network Interface) plugin providing networking, security, and observability for Kubernetes. Includes built-in BGP support. Handles L3-L4 traffic. |
| **Istio Ambient** | A sidecar-less service mesh mode using ztunnel (L4) and optional waypoint proxies (L7). Provides mTLS, traffic management, and authorization policies. |
| **ztunnel** | The node-level proxy in Istio Ambient mode. Runs as a DaemonSet and handles L4 mTLS encryption for all pods on the node. |
| **Waypoint Proxy** | An optional per-namespace Envoy proxy in Istio Ambient mode that enables L7 features like traffic shifting and authorization policies. |
| **SPIFFE** | Secure Production Identity Framework for Everyone. A standard for workload identity used by Istio for mTLS certificates. |
| **VyOS** | An open-source network operating system providing routing, firewall, VPN, and NAT capabilities. Used as the lab gateway. |
| **OpenBAO** | A community fork of HashiCorp Vault providing secrets management, PKI, and dynamic credential generation. |
| **Zitadel** | An open-source identity management platform providing OIDC, SAML, and user management. Used as the centralized identity provider. |
| **CloudNativePG** | A Kubernetes operator for managing PostgreSQL clusters with automated failover, backup, and recovery. |

---

## Networking Terms

| Term | Definition |
|:---|:---|
| **LACP (Link Aggregation Control Protocol)** | IEEE 802.3ad protocol for combining multiple network connections into a single logical link for increased bandwidth and redundancy. |
| **ECMP (Equal-Cost Multi-Path)** | Routing strategy where traffic to a destination is distributed across multiple paths of equal cost. Used by VyOS for BGP load balancing. |
| **BGP (Border Gateway Protocol)** | Routing protocol used to advertise Service VIPs from cluster nodes to the VyOS gateway. Enables true load balancing and instant failover. |
| **VIP (Virtual IP)** | A floating IP address assigned to Kubernetes LoadBalancer services. Advertised via BGP from `10.10.50.0/24`. |
| **Transit Link** | The routed connection between the Home Network (CCR2004) and the Lab Network (VP6630). Carries allowed traffic between the two zones. |

---

## VLAN Reference

| VLAN ID | Name | Subnet | Purpose |
|:---|:---|:---|:---|
| 10 | `LAB_MGMT` | `10.10.10.0/24` | Harvester host management, switch |
| 20 | `LAB_PROV` | `10.10.20.0/24` | Tinkerbell provisioning, PXE boot |
| 30 | `LAB_PLATFORM` | `10.10.30.0/24` | Platform Cluster traffic |
| 40 | `LAB_CLUSTER` | `10.10.40.0/24` | Tenant Cluster traffic |
| 50 | `LAB_SERVICE` | `10.10.50.0/24` | LoadBalancer VIPs (BGP) |
| 60 | `LAB_STORAGE` | `10.10.60.0/24` | Longhorn storage replication |

---

## Hardware Reference

| Device | Model | Role |
|:---|:---|:---|
| **MS-02** | Minisforum MS-02 (i9-12900H, 64GB) | Harvester HCI nodes (×3) |
| **UM760** | Minisforum UM760 (Ryzen 7, 32GB) | Platform Cluster physical anchor |
| **VP6630** | (VyOS Gateway) | Lab router, firewall, BGP peer |
| **CCR2004** | MikroTik CCR2004 | Home router (out of scope) |
| **Synology NAS** | DiskStation DS923+ | NFS storage, backup target, bootstrap host |

---

## Abbreviations

| Abbreviation | Meaning |
|:---|:---|
| ADR | Architecture Decision Record |
| AMT | Active Management Technology (Intel vPro) |
| API | Application Programming Interface |
| CA | Certificate Authority |
| CCM | Cloud Controller Manager |
| CNI | Container Network Interface |
| CRD | Custom Resource Definition |
| CSI | Container Storage Interface |
| DHCP | Dynamic Host Configuration Protocol |
| HA | High Availability |
| HCI | Hyper-Converged Infrastructure |
| HTTPS | Hypertext Transfer Protocol Secure |
| IDP | Internal Developer Platform |
| mTLS | Mutual TLS (two-way certificate authentication) |
| NAT | Network Address Translation |
| NFS | Network File System |
| NVMe | Non-Volatile Memory Express |
| OIDC | OpenID Connect |
| OOB | Out-of-Band |
| PKI | Public Key Infrastructure |
| PV | PersistentVolume |
| PXE | Preboot Execution Environment |
| RBAC | Role-Based Access Control |
| RPO | Recovery Point Objective |
| RTO | Recovery Time Objective |
| SAML | Security Assertion Markup Language |
| SFP+ | Small Form-factor Pluggable (enhanced) |
| SSO | Single Sign-On |
| TLS | Transport Layer Security |
| UI | User Interface |
| VIP | Virtual IP |
| VLAN | Virtual Local Area Network |
| VM | Virtual Machine |
