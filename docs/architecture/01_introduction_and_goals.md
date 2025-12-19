# 01. Introduction and Goals

## Project Vision
**To build a sophisticated, research-grade Platform Engineering laboratory on bare-metal hardware.**

This project represents a comprehensive effort to emulate a modern, production-grade cloud native datacenter within the constraints of a homelab. It serves as both a functional cloud environment for hosting services and a rigorous testbed for exploring advanced Platform Engineering concepts.

> "A fully automated, GitOps-focused, multi-cluster Talos Kubernetes architecture backed by Rancher Harvester."

## Motivation: The "Why"
The world of Cloud Native infrastructure is well-documented for hyperscalers (AWS, GCP, Azure). However, the patterns for **Bare Metal Kubernetes**—managing the physical layer, provisioning, and "day zero" bootstrapping—are often opaque or reliant on expensive proprietary enterprise tooling. Additionally, much of the literature targets massive enterprise scales, leaving a gap in understanding how these precepts adapt to smaller contexts.

I built this lab to:
1.  **Demystify Bare Metal**: Create a reproducible pattern for managing physical servers with the same GitOps elegance as cloud resources.
2.  **Research Platform Engineering**: Experiment with building a custom "Internal Developer Platform" (IDP) from the ground up, starting from the silicon.
3.  **Scale Adaptation**: Demonstrate that the rigorous precepts of Platform Engineering are not exclusive to the Enterprise, but can be successfully adapted to smaller contexts (the "Homelab Scale").
4.  **Demonstrate Capabilities**: Serve as a living portfolio of advanced infrastructure skills, showing how distinct technologies (Tinkerbell, Harvester, Talos, Argo CD) can be interwoven into a cohesive whole.
5.  **Host Services**: Provide a reliable (but not enterprise-critical) "Home Cloud" for streaming, gaming, and self-hosted OSS alternatives.

## Capabilities: The "What"
At its core, this project creates a **Computing Foundation**. It delivers:

*   **Elastic Compute**: A hyper-converged infrastructure layer (Harvester) capable of spawning ephemeral clusters.
*   **Cluster Factory**: A "Platform Cluster" that uses Cluster API (CAPI) to provision and lifecycle-manage downstream Kubernetes environments.
*   **GitOps Engine**: A state whereby the entire infrastructure—from OS configuration to application deployment—is defined in Git.
*   **future: Developer Platform**: While out of scope for *these* infrastructure documents, the ultimate goal is to sit a custom IDP on top of this stack, allowing for "1-click" environment provisioning.

## Design Philosophy

### 1. Reproducibility as Law
"Works on my machine" is forbidden. "I manually tweaked the config" is a failure.
The entire lab must be reproducible. If the house burns down, the `git clone` + `bootstrap` process—combined with cold-storage data backups—should restore the digital environment to an identical state on new hardware.

### 2. Immutable Infrastructure
We avoid "config management" (Ansible/Chef) in favor of **Immutability** and declarative configurations.
*   **OS**: Talos Linux is immutable; it has no SSH and no package manager. It is configured solely via API.
*   **Network**: VyOS configurations are fully backed by Git, extending the "Infrastructure as Code" philosophy to the physical gateway.
*   **VMs**: Nodes are treated as cattle, replaced rather than patched.

### 3. GitOps Everything
If it isn't in Git, it doesn't exist.
*   **Argo CD** is the source of truth.
*   Drift detection is active.
*   Manual changes are reverted automatically.

### 4. Strict Isolation
The Lab is a "Hostile Entity" to the Home Network.
*   A dedicated physical gateway (VyOS) enforces a strict firewall boundary.
*   The Lab can access the Internet, but the Lab cannot scan or access Home devices.

## High-Level Structure
The architecture is composed of distinct, decoupled layers:

1.  **Physical Layer**: Consumer bare-metal hardware (Minisforum MS-02s), networked via 25GbE (currently operating at 10GbE).
2.  **Provisioning Layer**: **Tinkerbell** handles the "Chicken and Egg" problem of booting bare metal.
3.  **HCI Layer**: **Rancher Harvester** virtualizes the hardware, providing flexible compute and block storage (Longhorn).
4.  **Platform Layer**: A privileged **Talos Kubernetes** cluster that acts as the "Brain", managing Identity (Zitadel), Secrets & PKI (OpenBAO), and Child Clusters.
5.  **Application Layer**: Downstream **Talos** clusters where actual workloads (Media, Games, Dev) reside.
