# Concept: Unified Control Plane

## Overview

The **Unified Control Plane** is the API layer that sits between the GitOps engine (Argo CD) and the raw infrastructure primitives (Kubernetes, Helm, CAPI). It is implemented using **Crossplane v2**.

Its primary purpose is **Simplification**. It wraps complex, multi-resource operational patterns into single, high-level API objects called **Composite Resources (XRs)**.

## Topology: Hub and Spoke

Crossplane is deployed in a distributed manner to bring the API to where it is needed:

1.  **Platform Cluster (The Hub)**:
    *   **Role**: Infrastructure Factory & Central Services.
    *   **Responsibility**:
        *   Provisions downstream clusters via `TenantCluster` (CAPI).
        *   Hosts centralized shared services via `SharedServices`.
2.  **Downstream Clusters (The Spokes)**:
    *   **Role**: Application Runtime.
    *   **Responsibility**:
        *   Runs the local "Base Layer" via `PlatformServices`.
        *   Runs tenant workloads via `Application`, `Service`, and `Database`.

## Core Resources (XRDs)

This is an exemplary list of core resources provided by our Control Plane.

### Infrastructure Layer (Platform Only)
*   **`TenantCluster`**: Defines a downstream Kubernetes cluster (Size, Version, Node Count). Composes Cluster API resources to provision VMs on Harvester & bootstrap Talos.

### Service Layer
*   **`SharedServices`**: (Platform Only) Deploys centralized "Public Good" services like Zitadel (IDM) or OpenBAO (Secrets).
*   **`PlatformServices`**: (All Clusters) Deploys the mandatory base layer required on *every* cluster (e.g., Cilium CNI, Harvester Cloud Controller Manager, Longhorn).

### Application Layer (Tenant Driven)
*   **`Application`**: A generic wrapper for any containerized application. Automatically handles `Deployment` or `StatefulSet` creation.
*   **`Service`**: A generic wrapper for exposing applications. Handles `Service`, `Ingress`, and `HTTPRoute` creation.
*   **`Database`**: A wrapper for **CloudNativePG**. Provisions a production-ready PostgreSQL cluster with backups and monitoring enabled.

## Packaging and Lifecycle

We treat our Control Plane as software. All definitions are versioned, built, and tested.

### Composition Functions
We strictly use **Go** for all Composition Functions. This provides a strongly-typed, testable, and high-performance environment for writing complex infrastructure logic. We avoid simple YAML "Patch & Transform" for anything non-trivial to ensure maintainability.

### Distribution Strategy
We distribute our API definitions as **Configuration Packages** (OCI Images).

*   **Monorepo Structure**: All packages live in `packages/` within the git root.
*   **Tagging Strategy**: We use standard monorepo tagging (e.g., `infrastructure/v1.2.0`) to trigger releases for specific packages.

### CI/CD Workflow
1.  **Build**: The CI system monitors tags. When a tag like `infrastructure/vX.Y.Z` is pushed, it runs `crossplane xpkg build`.
2.  **Publish**: The resulting OCI image is pushed to the container registry (e.g., `ghcr.io/gilmanlab/xrp-infrastructure:vX.Y.Z`).
3.  **Deploy**: Renovate (or manual PRs) updates the `Configuration` resource in the GitOps repository to pin the new version.

## GitOps Workflow

1.  **Configuration**: Argo CD installs the `Configuration` packages to the appropriate clusters.
2.  **Claim**: Argo CD syncs a simplified "Claim" YAML (e.g., `kind: Application`) to the cluster.
3.  **Composition**: Crossplane expands this Claim into the necessary low-level resources.
4.  **Actuation**: Kubernetes controllers (Helm, CAPI, CNPG) apply the actual change.
