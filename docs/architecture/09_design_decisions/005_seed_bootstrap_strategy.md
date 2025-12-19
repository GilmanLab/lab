# ADR 005: Seed Bootstrap Strategy

**Status**: Accepted
**Date**: 2025-12-19

## Context
The platform cluster hosts Crossplane, which processes Composite Resource (XR) Claims to provision infrastructure. This creates a chicken-and-egg problem: the platform cluster must bootstrap itself, but it cannot use Crossplane XRs during initial bootstrap because Crossplane isn't running yet.

Additionally, the seed cluster runs on a Synology NAS with limited resources (32GB RAM shared with DSM), making it unsuitable for running the full platform stack (Crossplane, CoreServices, Istio).

The platform cluster's first node (UM760 bare-metal) must be provisioned via PXE using Tinkerbell, which itself needs to be running somewhere during bootstrap.

## Options

### Option A: Full Platform on Seed
Run the complete platform stack (Crossplane, CoreServices, Tinkerbell) on the seed cluster, then migrate everything to the UM760.
*   **Mechanism**: Deploy all platform services on NAS, provision UM760, then migrate workloads.
*   **Pros**:
    *   **Uniform deployment**: Use XRs from the start.
    *   **Full GitOps**: No special-case manifests.
*   **Cons**:
    *   **Resource constraints**: NAS lacks RAM for Crossplane + Istio + CoreServices.
    *   **Migration complexity**: Live-migrating stateful Crossplane resources is risky.
    *   **Unnecessary overhead**: Full platform not needed just to PXE boot one node.

### Option B: Minimal Seed with Raw Manifests
Run only Tinkerbell on the seed cluster using raw Kubernetes manifests (not Crossplane XRs).
*   **Mechanism**: Deploy Tinkerbell via temporary Argo CD Application pointing to `bootstrap/seed/` containing raw CRDs. After UM760 joins, delete bootstrap Application and redeploy Tinkerbell via XRDs on the platform cluster.
*   **Pros**:
    *   **Minimal footprint**: Only Tinkerbell services on NAS.
    *   **Avoids chicken-and-egg**: Raw manifests don't require Crossplane.
    *   **Clean migration**: Delete bootstrap Application, apply platform configuration.
    *   **Resource-efficient**: Stays within NAS RAM constraints.
*   **Cons**:
    *   **Temporary divergence**: Seed configuration differs from steady-state.
    *   **Manual transition**: Requires deleting bootstrap Application.

### Option C: External Bootstrap Cluster
Use an external VM or container runtime (e.g., kind, k3s) for temporary Tinkerbell deployment.
*   **Mechanism**: Run Tinkerbell on disposable cluster outside the NAS.
*   **Pros**:
    *   **No seed cluster**: Avoids NAS resource usage entirely.
    *   **Clean separation**: Bootstrap infrastructure is ephemeral.
*   **Cons**:
    *   **Additional dependency**: Requires external VM or Docker environment.
    *   **Network complexity**: Must route PXE traffic to external cluster.
    *   **Disposable infrastructure**: Harder to debug or recover if issues arise.

## Decision
**Use Option B: Minimal Seed with Raw Manifests.**

## Rationale
1.  **Resource Constraints**: NAS has only 32GB RAM shared with Synology DSM; running Tinkerbell-only keeps memory usage minimal.
2.  **Avoids Chicken-and-Egg**: Raw Kubernetes manifests in `bootstrap/seed/` deploy Tinkerbell without requiring Crossplane.
3.  **Clean Migration Path**: After UM760 joins the platform cluster:
    *   Delete the temporary `bootstrap` Application
    *   Deploy full platform configuration via XRs in `clusters/platform/`
    *   Tinkerbell is redeployed via proper XRD-based path as `clusters/platform/apps/tinkerbell/`
4.  **Simplicity**: Seed runs a single, well-defined service (Tinkerbell) with minimal dependencies.
5.  **Debuggability**: Seed cluster remains accessible during bootstrap for troubleshooting.
6.  **Alignment with Design Principles**: Once platform is operational, all infrastructure is managed via Crossplane XRs (steady state).
