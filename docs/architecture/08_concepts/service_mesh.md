# 08. Concepts - Service Mesh

## Overview

The service mesh provides **L7 security and traffic management** across all clusters. The architecture uses a layered approach:

| Layer | Technology | Responsibility |
|:---|:---|:---|
| **L3-L4** | Cilium | CNI, network policies, BGP load balancing, eBPF |
| **L7** | Istio Ambient | mTLS, traffic management, authorization policies |

> [!IMPORTANT]
> Istio Ambient is a **core service** deployed to all clusters (Platform and Tenant).

---

## Why Istio Ambient?

Istio Ambient mode is a **sidecar-less** service mesh architecture introduced in Istio 1.18+:

| Component | Role |
|:---|:---|
| **ztunnel** | Node-level proxy handling L4 mTLS (DaemonSet) |
| **Waypoint Proxy** | Optional per-namespace L7 proxy for traffic management |

### Comparison to Sidecar Mode

| Aspect | Sidecar Mode | Ambient Mode |
|:---|:---|:---|
| **Resource Overhead** | Per-pod proxy (Envoy) | Shared node proxy (ztunnel) |
| **Complexity** | Sidecar injection required | No injection; opt-in L7 |
| **Latency** | Extra hop per request | Reduced for L4-only traffic |
| **Adoption** | All-or-nothing per namespace | Gradual per-workload |

**Decision**: Ambient mode aligns with our "Simplicity > Complexity" principle — mTLS is automatic at L4, and L7 features are opt-in where needed.

---

## Architecture

### Layered Responsibility Model

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Application Traffic                          │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
                    ┌───────────▼───────────┐
                    │   Istio Ambient (L7)   │
                    │  ┌─────────────────┐   │
                    │  │ Waypoint Proxy  │   │  ← Traffic management,
                    │  │   (optional)    │   │    AuthorizationPolicy
                    │  └─────────────────┘   │
                    │  ┌─────────────────┐   │
                    │  │    ztunnel      │   │  ← mTLS, L4 policy
                    │  │  (per-node)     │   │
                    │  └─────────────────┘   │
                    └───────────┬───────────┘
                                │
                    ┌───────────▼───────────┐
                    │     Cilium (L3-L4)     │
                    │  • Network Policies    │
                    │  • BGP LoadBalancer    │
                    │  • eBPF datapath       │
                    └───────────────────────┘
```

### What Each Layer Handles

| Concern | Cilium | Istio Ambient |
|:---|:---:|:---:|
| Pod-to-pod routing | ✅ | — |
| Network policies (L3-L4) | ✅ | — |
| LoadBalancer VIPs (BGP) | ✅ | — |
| mTLS encryption | — | ✅ |
| Service identity (SPIFFE) | — | ✅ |
| Traffic shifting (canary) | — | ✅ |
| Authorization policies (L7) | — | ✅ |
| Retries, timeouts, circuit breaking | — | ✅ |

---

## Deployment

### Core Components

| Component | Deployment | Scope |
|:---|:---|:---|
| **istiod** | Deployment (1-3 replicas) | Cluster control plane |
| **ztunnel** | DaemonSet | Every node |
| **Waypoint Proxy** | Deployment (per-namespace) | Namespaces needing L7 |

### Namespace Enrollment

Namespaces opt into the mesh via labels:

```yaml
# L4 mTLS only (ztunnel)
apiVersion: v1
kind: Namespace
metadata:
  name: my-app
  labels:
    istio.io/dataplane-mode: ambient
```

```yaml
# L7 features via waypoint proxy
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-app-waypoint
  namespace: my-app
spec:
  gatewayClassName: istio-waypoint
```

---

## Certificate Integration: OpenBAO

Istio is configured to use **OpenBAO PKI** as the certificate authority instead of Istio's built-in CA.

### Integration Flow

```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│     istiod      │──────▶│   cert-manager  │──────▶│    OpenBAO      │
│                 │       │   (Issuer)      │       │   (PKI Engine)  │
└─────────────────┘       └─────────────────┘       └─────────────────┘
        │
        │ Distributes certs via
        │ xDS to ztunnel/waypoints
        ▼
┌─────────────────┐
│    ztunnel      │
│  (mTLS termination)
└─────────────────┘
```

### Benefits

| Aspect | Benefit |
|:---|:---|
| **Centralized PKI** | All certificates (Ingress, mTLS, Talos) from one CA |
| **Audit Trail** | OpenBAO logs all certificate issuance |
| **Rotation** | Automated short-lived certificates |

---

## Use Cases

### 1. Automatic mTLS

All pod-to-pod traffic is encrypted without application changes:

```yaml
# PeerAuthentication (cluster-wide strict mTLS)
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: default
  namespace: istio-system
spec:
  mtls:
    mode: STRICT
```

### 2. Authorization Policies

Fine-grained access control at L7:

```yaml
apiVersion: security.istio.io/v1
kind: AuthorizationPolicy
metadata:
  name: allow-frontend
  namespace: backend
spec:
  selector:
    matchLabels:
      app: api
  rules:
    - from:
        - source:
            principals: ["cluster.local/ns/frontend/sa/webapp"]
```

### 3. Traffic Shifting (Canary)

Gradual rollout of new versions:

```yaml
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: api
spec:
  hosts: [api]
  http:
    - route:
        - destination:
            host: api
            subset: v1
          weight: 90
        - destination:
            host: api
            subset: v2
          weight: 10
```

---

## Observability Integration

Istio exports metrics to the central Prometheus:

| Metric Type | Source | Examples |
|:---|:---|:---|
| **Request metrics** | ztunnel, waypoint | `istio_requests_total`, latency histograms |
| **Connection metrics** | ztunnel | `istio_tcp_connections_opened_total` |
| **Control plane** | istiod | `pilot_xds_pushes`, config sync latency |

Grafana dashboards are available for:
- Service-to-service traffic flow
- mTLS certificate status
- Waypoint proxy performance

---

## Compatibility Notes

### Cilium + Istio Ambient

| Concern | Status |
|:---|:---|
| **CNI Compatibility** | ✅ Cilium as CNI, Istio Ambient as mesh |
| **Network Policies** | ✅ Cilium L3-L4 policies enforced before Istio |
| **LoadBalancer** | ✅ Cilium BGP handles external traffic; Istio handles internal |
| **eBPF** | ✅ No conflict; different BPF programs |

> [!NOTE]
> Cilium's own L7 policy features (Envoy-based) are **not used** to avoid overlap with Istio.
