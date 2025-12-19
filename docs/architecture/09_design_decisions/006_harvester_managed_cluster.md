# ADR 006: Harvester as Managed Cluster

**Status**: Accepted
**Date**: 2025-12-19

## Context
Harvester is a bare-metal Kubernetes-based HCI (Hyper-Converged Infrastructure) platform that provides VM management, storage, and networking. It runs on dedicated hardware (MS-02 nodes) and is provisioned via Tinkerbell PXE boot.

Harvester requires configuration for:
*   **Networks**: VLANs for management, platform, tenant clusters, and storage replication
*   **Images**: VM templates (e.g., Talos Linux images)
*   **VMs**: Platform cluster control plane nodes (CP-2, CP-3) and standalone VMs (non-containerized workloads)

We need a strategy for managing Harvester configuration declaratively and integrating it into the GitOps workflow.

## Options

### Option A: Manual Configuration
Configure Harvester through its web UI or CLI.
*   **Mechanism**: Manually create networks, images, and VMs via Harvester UI.
*   **Pros**:
    *   **Simplicity**: No additional tooling or configuration.
    *   **Immediate**: Changes apply instantly.
*   **Cons**:
    *   **Not declarative**: Configuration drift, no GitOps benefits.
    *   **No audit trail**: Changes not tracked in version control.
    *   **Error-prone**: Manual operations increase risk of misconfiguration.

### Option B: Separate Tooling (Terraform/Ansible)
Use infrastructure-as-code tools (Terraform with Harvester provider, Ansible) to manage Harvester.
*   **Mechanism**: Define Harvester resources in Terraform/Ansible, apply separately from Kubernetes clusters.
*   **Pros**:
    *   **Declarative**: Infrastructure as code with state management.
    *   **Mature tooling**: Terraform Harvester provider exists.
*   **Cons**:
    *   **Fragmented workflow**: Separate tool from Argo CD/Kubernetes management.
    *   **No unified visibility**: Harvester config not visible in Argo CD.
    *   **Additional state management**: Terraform state requires separate backend.

### Option C: Register Harvester with Argo CD
Register Harvester as a managed cluster in Argo CD, syncing raw Harvester CRDs from Git.
*   **Mechanism**: Create Argo CD cluster Secret with Harvester kubeconfig. Store Harvester resources (ClusterNetwork, VirtualMachineImage, VirtualMachine CRDs) in `clusters/harvester/`. Argo CD syncs directly to Harvester API.
*   **Pros**:
    *   **GitOps consistency**: Harvester config managed via Git like all other clusters.
    *   **Unified visibility**: All infrastructure visible in Argo CD UI.
    *   **Declarative VM management**: Platform CP-2/CP-3 VMs defined as code.
    *   **Leverages existing tooling**: No new tools; uses Argo CD hub-and-spoke.
*   **Cons**:
    *   **Raw CRDs (not XRs)**: Harvester has its own API; cannot use Crossplane abstractions.
    *   **Manual registration**: Harvester cluster Secret must be created manually (Harvester isn't provisioned via CAPI).

## Decision
**Use Option C: Register Harvester as Managed Cluster.**

## Rationale
1.  **GitOps Consistency**: Aligns with platform's GitOps-first design principle; all infrastructure configuration lives in Git.
2.  **Declarative VM Management**: Platform cluster bootstrap VMs (CP-2, CP-3) are defined as VirtualMachine CRDs in `clusters/harvester/vms/platform/`, ensuring reproducible cluster expansion.
3.  **Unified Tooling**: Leverages existing Argo CD hub-and-spoke model; no additional tools (Terraform/Ansible) required.
4.  **Audit Trail**: All Harvester configuration changes tracked via Git commits with full history.
5.  **Standalone VM Support**: Non-containerized workloads (gaming VMs, NAS VMs) can be managed declaratively in `clusters/harvester/vms/standalone/`.
6.  **Bootstrap Integration**: During platform bootstrap (phase 3), Harvester cluster Secret is created manually, enabling Argo CD to immediately sync network/image/VM configuration.
7.  **Network Configuration**: VLAN configs (mgmt, platform, cluster, storage) are versioned and applied declaratively, preventing configuration drift.
