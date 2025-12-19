# 02. Constraints

This section outlines the hard technical and organizational boundaries within which the solution must operate.

## Technical Constraints (Hardware)

| Constraint | Context | Implication for Architecture |
| :--- | :--- | :--- |
| **MS-02 Split Plane** | The Minisforum MS-02 nodes have 2x 10G/25G SFP+ ports and 2x 2.5G RJ45 ports. **vPro/AMT only works reliably on the 2.5G ports.** | We must cable the 2.5G ports to the Gateway (VP6630) for OOB/PXE, and SFP+ to the Switch for Data. Network design must accommodate this split physically. |
| **UM760 Connectivity** | The Minisforum UM760 (Platform Node) has a single 2.5G RJ45 NIC. | Physical separation of "Provisioning" and "Data" traffic is impossible. We must use a **Hybrid Trunk** configuration (Native VLAN 20 + Tagged VLANs) on a single cable. |
| **Switch Bandwidth** | The Lab Switch (Mikrotik) is 10GbE SFP+ (8 ports). MS-02s are connected via 2x 10GbE DACs each. | Bandwidth is physically capped at 10GbE per link (25GbE capable NICs downgraded). Sufficient ports exist to connect both SFP28 ports per MS-02, enabling either **LACP Bonding** (20Gbps aggregate) or **Physical Segregation** (Storage vs Workload). |
| **Storage Locality** | We rely on **Longhorn** (Replicated Block Storage) over the network. | Network latency is critical. We must decide between **Bonding** (Shared 20Gbps pipe) or **Physical Segregation** (Dedicating one 10GbE link strictly to Storage) to manage contention. |

## Integration Constraints (The "Neighbors")

| Constraint | Context | Implication for Architecture |
| :--- | :--- | :--- |
| **Home Isolation** | The Lab exists physically within a Home Network but logically must be treated as a "Hostile/External" entity. | The Upstream Router (CCR2004) configuration must remain minimal. We cannot rely on OSPF/BGP propagation into the Home Network. A simple Static Route + Transit Link is the only allowed coupling. |
| **Bootstrap Seed Requirement** | A temporary "Seed" cluster is required to kickoff the automation. Currently fulfilled by the Synology NAS. | The bootstrap role is portable. While the NAS is the default host, *any* machine (Laptop, Desktop VM) capable of running a Talos VM and accessing the network can satisfy this requirement, MITIGATING the single point of failure. |

## Operational Constraints

| Constraint | Context | Implication for Architecture |
| :--- | :--- | :--- |
| **Bus Factor of 1** | Only one administrator (Josh) manages the entire stack. | **Simplicity > Complexity**. If a feature requires constant manual tuning, it is rejected. Automation (GitOps) is mandatory to reduce toil. |
| **Power Budget** | Residential power circuit limits. | We cannot simply scale out nodes endlessly. The cluster must be efficient. Power-shedding strategies (shutting down non-essential nodes) may be needed in the future. |
