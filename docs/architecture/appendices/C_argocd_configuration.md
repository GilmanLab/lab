# Appendix C: Argo CD Configuration Reference

> **Document Type**: Technical Reference
> **Date**: 2025-12-19
> **Related Concepts**: See main architecture documentation for high-level understanding

This appendix provides detailed technical reference for Argo CD configuration in the lab infrastructure, including cluster registration, ApplicationSet definitions, sync wave strategies, and secrets management integration.

---

## Table of Contents

- [Cluster Registration](#cluster-registration)
- [ApplicationSet Definitions](#applicationset-definitions)
- [Sync Wave Strategy](#sync-wave-strategy)
- [Project Structure](#project-structure)
- [Health Checks and Sync Policies](#health-checks-and-sync-policies)
- [Secrets Management Integration](#secrets-management-integration)

---

## Cluster Registration

Argo CD uses the Hub-and-Spoke model where a single Argo CD instance on the platform cluster manages all clusters. Clusters are registered via Secrets with the `argocd.argoproj.io/secret-type: cluster` label.

### Platform Cluster Self-Registration

The platform cluster must register itself to be managed by its own Argo CD instance. This registration happens during the initial bootstrap process.

```yaml
# File: Created during genesis (bootstrap/genesis/scripts/install-argocd.sh)
# Location: platform cluster, argocd namespace
# Purpose: Enables platform cluster to manage itself via Argo CD

apiVersion: v1
kind: Secret
metadata:
  name: platform
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: cluster
    lab.gilman.io/cluster-name: platform
type: Opaque
stringData:
  name: platform
  server: https://kubernetes.default.svc
  config: |
    {
      "tlsClientConfig": {
        "insecure": false
      }
    }
```

**Key Points:**
- Uses in-cluster endpoint (`https://kubernetes.default.svc`)
- Created manually during bootstrap step
- Required for ApplicationSets to target platform cluster
- Label `lab.gilman.io/cluster-name: platform` enables ApplicationSet selector matching

---

### Harvester Cluster Registration

Harvester is registered as a managed cluster after it is provisioned via Tinkerbell PXE boot. This registration is performed manually during bootstrap step 12.

```yaml
# File: Created manually during bootstrap step 12
# Location: platform cluster, argocd namespace
# Purpose: Enables Argo CD to manage Harvester infrastructure

apiVersion: v1
kind: Secret
metadata:
  name: harvester
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: cluster
    lab.gilman.io/cluster-name: harvester
type: Opaque
stringData:
  name: harvester
  server: https://harvester.lab.local:6443
  config: |
    {
      "tlsClientConfig": {
        "caData": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...",
        "certData": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...",
        "keyData": "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVkt..."
      }
    }
```

**How to Obtain Values:**
```bash
# Extract Harvester kubeconfig
kubectl --kubeconfig /path/to/harvester.kubeconfig config view --raw -o json | \
  jq -r '.clusters[0].cluster."certificate-authority-data"'  # caData

kubectl --kubeconfig /path/to/harvester.kubeconfig config view --raw -o json | \
  jq -r '.users[0].user."client-certificate-data"'  # certData

kubectl --kubeconfig /path/to/harvester.kubeconfig config view --raw -o json | \
  jq -r '.users[0].user."client-key-data"'  # keyData
```

**Key Points:**
- Uses external endpoint (Harvester API server)
- Requires TLS client certificates for authentication
- Manages raw Harvester CRDs (networks, images, VMs)
- Different from tenant clusters (which use Crossplane XRs)

---

### Tenant Cluster Automatic Registration

Tenant clusters are automatically registered when their TenantCluster XR is created. The Crossplane composition includes logic to create the Argo CD cluster Secret with the CAPI-generated kubeconfig.

```yaml
# File: Automatically created by TenantCluster XR composition
# Location: platform cluster, argocd namespace
# Purpose: Enables Argo CD to immediately discover and deploy to new tenant cluster

apiVersion: v1
kind: Secret
metadata:
  name: media
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: cluster
    lab.gilman.io/cluster-name: media
    lab.gilman.io/cluster-type: tenant
    lab.gilman.io/managed-by: crossplane
type: Opaque
stringData:
  name: media
  server: https://10.10.40.10:6443
  config: |
    {
      "tlsClientConfig": {
        "caData": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...",
        "certData": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...",
        "keyData": "LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVkt..."
      }
    }
```

**TenantCluster XR Composition Logic (Conceptual):**
```yaml
# Within the TenantCluster XRD composition
# This shows the concept - actual implementation uses Crossplane Functions

resources:
  # ... CAPI Cluster resources ...

  # Argo CD cluster registration Secret
  - name: argocd-cluster-secret
    base:
      apiVersion: v1
      kind: Secret
      metadata:
        namespace: argocd
        labels:
          argocd.argoproj.io/secret-type: cluster
          lab.gilman.io/cluster-type: tenant
          lab.gilman.io/managed-by: crossplane
      type: Opaque
    patches:
      - type: FromCompositeFieldPath
        fromFieldPath: metadata.name
        toFieldPath: metadata.name
      - type: FromCompositeFieldPath
        fromFieldPath: metadata.name
        toFieldPath: stringData.name
      - type: FromCompositeFieldPath
        fromFieldPath: status.controlPlaneEndpoint
        toFieldPath: stringData.server
        transforms:
          - type: string
            string:
              fmt: "https://%s:6443"
      - type: FromCompositeFieldPath
        fromFieldPath: status.kubeconfig
        toFieldPath: stringData.config
```

**Key Points:**
- Fully automated - no manual intervention required
- Created as soon as CAPI generates kubeconfig
- Enables immediate deployment of apps to new cluster
- Lifecycle tied to TenantCluster XR (deleted when XR is deleted)

---

## ApplicationSet Definitions

### cluster-definitions ApplicationSet

This ApplicationSet syncs cluster-level configuration files (`cluster.yaml`, `core.yaml`, `platform.yaml`) to the **platform cluster** where Crossplane processes the XR Claims.

```yaml
# File: clusters/platform/apps/argocd/applicationsets/cluster-definitions.yaml
# Purpose: Deploy cluster definitions as XR Claims to platform cluster for Crossplane to process

apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: cluster-definitions
  namespace: argocd
spec:
  goTemplate: true
  goTemplateOptions: ["missingkey=error"]

  generators:
    - git:
        repoURL: https://github.com/gilmanlab/lab.git
        revision: HEAD
        directories:
          - path: clusters/*
            exclude: false

  template:
    metadata:
      name: 'cluster-{{.path.basename}}'
      annotations:
        argocd.argoproj.io/manifest-generate-paths: '{{.path}}'
      labels:
        app.kubernetes.io/managed-by: argocd
        lab.gilman.io/cluster-name: '{{.path.basename}}'
        lab.gilman.io/app-type: cluster-definition

    spec:
      project: clusters

      # Always deploy to platform cluster - Crossplane lives here
      destination:
        server: https://kubernetes.default.svc
        namespace: default

      source:
        repoURL: https://github.com/gilmanlab/lab.git
        targetRevision: HEAD
        path: '{{.path}}'
        directory:
          recurse: false
          include: '*.yaml'
          exclude: 'apps/**'

      syncPolicy:
        automated:
          prune: true
          selfHeal: true
          allowEmpty: false
        syncOptions:
          - CreateNamespace=true
          - ServerSideApply=true
          - RespectIgnoreDifferences=true
        retry:
          limit: 5
          backoff:
            duration: 5s
            factor: 2
            maxDuration: 3m

      ignoreDifferences:
        - group: "*"
          kind: "*"
          jqPathExpressions:
            - '.metadata.managedFields'
```

**What This Creates:**

| Cluster Directory | Application Name | Contains | Deployed To |
|:---|:---|:---|:---|
| `clusters/platform/` | `cluster-platform` | `core.yaml`, `platform.yaml` | Platform cluster |
| `clusters/harvester/` | `cluster-harvester` | (none - uses `config/` and `vms/`) | Platform cluster |
| `clusters/media/` | `cluster-media` | `cluster.yaml`, `core.yaml` | Platform cluster |
| `clusters/dev/` | `cluster-dev` | `cluster.yaml`, `core.yaml` | Platform cluster |
| `clusters/prod/` | `cluster-prod` | `cluster.yaml`, `core.yaml` | Platform cluster |

**Key Configuration Details:**
- `recurse: false` - Only files in cluster root directory, not subdirectories
- `include: '*.yaml'` - Only YAML files
- `exclude: 'apps/**'` - Explicitly exclude apps subdirectory
- `ServerSideApply=true` - Required for proper Crossplane field management
- Always targets `https://kubernetes.default.svc` (platform cluster)

---

### cluster-apps ApplicationSet

This ApplicationSet uses a matrix generator to deploy applications to their correct destination clusters. It combines cluster discovery with git directory discovery.

```yaml
# File: clusters/platform/apps/argocd/applicationsets/cluster-apps.yaml
# Purpose: Deploy applications from clusters/*/apps/* to their respective target clusters

apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: cluster-apps
  namespace: argocd
spec:
  goTemplate: true
  goTemplateOptions: ["missingkey=error"]

  generators:
    - matrix:
        generators:
          # Generator 1: Discover all registered clusters
          - clusters:
              selector:
                matchExpressions:
                  - key: lab.gilman.io/cluster-name
                    operator: Exists
              values:
                clusterName: '{{.name}}'

          # Generator 2: For each cluster, discover its app directories
          - git:
              repoURL: https://github.com/gilmanlab/lab.git
              revision: HEAD
              directories:
                - path: 'clusters/{{.values.clusterName}}/apps/*'

  template:
    metadata:
      name: '{{.values.clusterName}}-{{.path.basename}}'
      annotations:
        argocd.argoproj.io/manifest-generate-paths: '{{.path}}'
      labels:
        app.kubernetes.io/managed-by: argocd
        lab.gilman.io/cluster-name: '{{.values.clusterName}}'
        lab.gilman.io/app-name: '{{.path.basename}}'
        lab.gilman.io/app-type: application

    spec:
      project: apps

      # Deploy to actual cluster (uses server from cluster Secret)
      destination:
        server: '{{.server}}'
        namespace: default

      source:
        repoURL: https://github.com/gilmanlab/lab.git
        targetRevision: HEAD
        path: '{{.path}}'
        directory:
          recurse: true
          include: '*.yaml'

      syncPolicy:
        automated:
          prune: true
          selfHeal: true
          allowEmpty: false
        syncOptions:
          - CreateNamespace=true
          - ServerSideApply=true
          - RespectIgnoreDifferences=true
        retry:
          limit: 5
          backoff:
            duration: 5s
            factor: 2
            maxDuration: 3m

      ignoreDifferences:
        - group: "*"
          kind: "*"
          jqPathExpressions:
            - '.metadata.managedFields'
```

**Matrix Generator Flow:**

```
Step 1: Cluster Generator discovers registered clusters
  → platform (server: https://kubernetes.default.svc)
  → harvester (server: https://harvester.lab.local:6443)
  → media (server: https://10.10.40.10:6443)
  → dev (server: https://10.10.40.20:6443)

Step 2: For each cluster, Git Generator discovers apps
  → clusters/platform/apps/tinkerbell/
  → clusters/platform/apps/observability/
  → clusters/platform/apps/capi/
  → clusters/media/apps/plex/
  → clusters/media/apps/jellyfin/
  → (etc.)

Step 3: Matrix combines them
  → platform-tinkerbell (deployed to platform cluster)
  → platform-observability (deployed to platform cluster)
  → media-plex (deployed to media cluster)
  → media-jellyfin (deployed to media cluster)
  → (etc.)
```

**Example Generated Application:**

```yaml
# Generated for clusters/media/apps/plex/
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: media-plex
  namespace: argocd
  labels:
    lab.gilman.io/cluster-name: media
    lab.gilman.io/app-name: plex
    lab.gilman.io/app-type: application
spec:
  project: apps
  destination:
    server: https://10.10.40.10:6443  # From media cluster Secret
    namespace: default
  source:
    repoURL: https://github.com/gilmanlab/lab.git
    targetRevision: HEAD
    path: clusters/media/apps/plex
    directory:
      recurse: true
```

**Key Configuration Details:**
- Matrix generator enables many-to-many mapping
- `recurse: true` - Include all files in app directory tree
- Destination server comes from cluster Secret (`{{.server}}`)
- Each cluster only gets its own apps (path filtering via `clusterName`)

---

## Sync Wave Strategy

Argo CD sync waves control the order in which resources are deployed. This ensures dependencies are satisfied before dependent resources are created.

### Sync Wave Ordering

| Wave | Resource Type | Example | Rationale |
|:---:|:---|:---|:---|
| `-5` | Namespaces | Namespace CRDs | Must exist before other resources |
| `-4` | CRDs and XRDs | Crossplane XRDs, CAPI CRDs | Define custom resource types |
| `-3` | Operators | Crossplane, CAPI providers, cert-manager | Process custom resources |
| `-2` | Configurations | Crossplane Configurations, provider configs | Configure operators |
| `-1` | XR Claims (Infrastructure) | CoreServices XR, PlatformServices XR | Core infrastructure layer |
| `0` | XR Claims (Clusters) | TenantCluster XR | Cluster provisioning |
| `1` | XR Claims (Apps) | Application XR, Database XR | Application-level resources |
| `2` | Post-deployment | Backup jobs, monitoring alerts | Depends on apps being ready |

### Implementation Examples

**CoreServices XR (Wave -1):**
```yaml
# File: clusters/platform/core.yaml
# Purpose: Deploy foundational platform services before anything else

apiVersion: platform.gilman.io/v1alpha1
kind: CoreServices
metadata:
  name: platform
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
    argocd.argoproj.io/sync-options: "SkipDryRunOnMissingResource=true"
spec:
  crossplane:
    version: "1.14.5"
    providers:
      - provider-kubernetes
      - provider-helm
  certManager:
    version: "1.13.3"
  capi:
    version: "1.6.1"
    providers:
      - harvester
      - talos
```

**TenantCluster XR (Wave 0):**
```yaml
# File: clusters/media/cluster.yaml
# Purpose: Provision tenant cluster after core services are ready

apiVersion: infrastructure.gilman.io/v1alpha1
kind: TenantCluster
metadata:
  name: media
  annotations:
    argocd.argoproj.io/sync-wave: "0"
    argocd.argoproj.io/sync-options: "SkipDryRunOnMissingResource=true"
spec:
  controlPlane:
    replicas: 3
    machineType: "standard-medium"
  workers:
    replicas: 3
    machineType: "standard-large"
  network:
    vlan: 40
    subnet: "10.10.40.0/24"
  talos:
    version: "1.9.0"
```

**Application XR (Wave 1):**
```yaml
# File: clusters/media/apps/plex/plex.yaml
# Purpose: Deploy application after cluster exists

apiVersion: platform.gilman.io/v1alpha1
kind: Application
metadata:
  name: plex
  annotations:
    argocd.argoproj.io/sync-wave: "1"
spec:
  chart:
    repository: https://charts.example.com
    name: plex
    version: "1.2.3"
  values:
    persistence:
      enabled: true
      size: "500Gi"
```

### Wave Progression Flow

```
Wave -5: Namespaces created
    ↓
Wave -4: XRDs and CRDs installed
    ↓
Wave -3: Operators deployed (Crossplane, CAPI)
    ↓ (Wait for operators to be ready)
Wave -2: Provider configurations applied
    ↓
Wave -1: CoreServices XR processed
    ↓ (Crossplane deploys cert-manager, etc.)
Wave 0: TenantCluster XR processed
    ↓ (CAPI provisions VMs, creates cluster)
Wave 1: Application XR processed
    ↓ (Apps deployed to tenant cluster)
Wave 2: Post-deployment resources
```

**Sync Options:**
- `SkipDryRunOnMissingResource=true` - Required for XRs (CRD may not exist yet)
- `RespectIgnoreDifferences=true` - Prevent sync loops on Crossplane-managed fields

---

## Project Structure

Argo CD Projects provide logical grouping and RBAC boundaries for applications.

### clusters Project

```yaml
# File: clusters/platform/apps/argocd/projects/clusters.yaml
# Purpose: Project for cluster definition applications

apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: clusters
  namespace: argocd
spec:
  description: Cluster definitions (XR Claims for infrastructure)

  sourceRepos:
    - https://github.com/gilmanlab/lab.git

  # Cluster definitions always deploy to platform cluster
  destinations:
    - server: https://kubernetes.default.svc
      namespace: '*'

  # Allow all Crossplane XR types
  clusterResourceWhitelist:
    - group: 'infrastructure.gilman.io'
      kind: '*'
    - group: 'platform.gilman.io'
      kind: '*'

  namespaceResourceWhitelist:
    - group: '*'
      kind: '*'

  orphanedResources:
    warn: true
```

### apps Project

```yaml
# File: clusters/platform/apps/argocd/projects/apps.yaml
# Purpose: Project for application deployments across all clusters

apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: apps
  namespace: argocd
spec:
  description: Application deployments to managed clusters

  sourceRepos:
    - https://github.com/gilmanlab/lab.git

  # Apps can deploy to any registered cluster
  destinations:
    - server: '*'
      namespace: '*'

  # Allow Application XRs and Helm releases
  clusterResourceWhitelist:
    - group: 'platform.gilman.io'
      kind: 'Application'
    - group: 'platform.gilman.io'
      kind: 'Database'

  namespaceResourceWhitelist:
    - group: '*'
      kind: '*'

  orphanedResources:
    warn: true
```

### harvester Project

```yaml
# File: clusters/platform/apps/argocd/projects/harvester.yaml
# Purpose: Project for Harvester infrastructure resources

apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: harvester
  namespace: argocd
spec:
  description: Harvester HCI cluster configuration

  sourceRepos:
    - https://github.com/gilmanlab/lab.git

  # Only deploy to Harvester cluster
  destinations:
    - server: https://harvester.lab.local:6443
      namespace: '*'

  # Allow Harvester-specific CRDs
  clusterResourceWhitelist:
    - group: 'network.harvesterhci.io'
      kind: '*'
    - group: 'kubevirt.io'
      kind: '*'

  namespaceResourceWhitelist:
    - group: '*'
      kind: '*'

  orphanedResources:
    warn: true
```

---

## Health Checks and Sync Policies

### Custom Health Checks

Argo CD needs custom health checks to understand Crossplane XR status.

```yaml
# File: clusters/platform/apps/argocd/config/argocd-cm.yaml
# Purpose: Configure custom health checks for Crossplane resources

apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  # Custom health check for Crossplane XRs
  resource.customizations.health.infrastructure.gilman.io_TenantCluster: |
    hs = {}
    if obj.status ~= nil then
      if obj.status.conditions ~= nil then
        for i, condition in ipairs(obj.status.conditions) do
          if condition.type == "Ready" then
            if condition.status == "True" then
              hs.status = "Healthy"
              hs.message = "Cluster is ready"
              return hs
            elseif condition.status == "False" then
              hs.status = "Degraded"
              hs.message = condition.message
              return hs
            end
          end
        end
      end
    end
    hs.status = "Progressing"
    hs.message = "Waiting for cluster to be ready"
    return hs

  # Custom health check for CAPI Clusters
  resource.customizations.health.cluster.x-k8s.io_Cluster: |
    hs = {}
    if obj.status ~= nil then
      if obj.status.phase == "Provisioned" then
        hs.status = "Healthy"
        hs.message = "Cluster is provisioned"
        return hs
      elseif obj.status.phase == "Failed" then
        hs.status = "Degraded"
        hs.message = "Cluster provisioning failed"
        return hs
      end
    end
    hs.status = "Progressing"
    hs.message = "Cluster is provisioning"
    return hs
```

### Global Sync Policies

```yaml
# File: clusters/platform/apps/argocd/config/argocd-cm.yaml (continued)
# Purpose: Set default sync policies for all applications

apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  # (health checks above)

  # Global sync options
  application.resourceTrackingMethod: annotation+label

  # Timeout for sync operations
  timeout.reconciliation: 300s
  timeout.hard.reconciliation: 0

  # Resource exclusions (don't manage these)
  resource.exclusions: |
    - apiGroups:
      - cilium.io
      kinds:
      - CiliumIdentity
      clusters:
      - "*"
```

---

## Secrets Management Integration

The lab uses Vault Secrets Operator (VSO) for injecting secrets into XR Claims and applications.

### VSO Configuration

VSO is deployed as part of the CoreServices XR and integrates with OpenBAO (deployed via PlatformServices XR).

```yaml
# File: Part of CoreServices XR composition
# Purpose: Deploy VSO to enable secret injection

apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: vault-secrets-operator
  namespace: vault-secrets-operator-system
spec:
  chart:
    spec:
      chart: vault-secrets-operator
      version: "0.4.3"
      sourceRef:
        kind: HelmRepository
        name: hashicorp
  values:
    defaultVaultConnection:
      enabled: true
      address: "http://openbao.openbao-system.svc:8200"
      skipTLSVerify: false
```

### Secret Injection in XR Claims

**Example: Database XR with VSO Secret Injection**

```yaml
# File: clusters/media/apps/plex/database.yaml
# Purpose: Provision PostgreSQL database with credentials from Vault

apiVersion: platform.gilman.io/v1alpha1
kind: Database
metadata:
  name: plex-db
  annotations:
    argocd.argoproj.io/sync-wave: "0"  # Before Application XR
spec:
  engine: postgresql
  version: "16"
  size: "20Gi"

  # VSO injects credentials from Vault
  credentialsSecretRef:
    name: plex-db-credentials

---
# VSO VaultStaticSecret - fetches from Vault and creates K8s Secret
apiVersion: secrets.hashicorp.com/v1beta1
kind: VaultStaticSecret
metadata:
  name: plex-db-credentials
  annotations:
    argocd.argoproj.io/sync-wave: "-1"  # Before Database XR
spec:
  vaultAuthRef: default
  mount: kv
  type: kv-v2
  path: databases/plex/credentials

  destination:
    name: plex-db-credentials
    create: true

  refreshAfter: 30s

  # Secret mapping
  hmacSecretData: true
```

**Vault Secret Structure:**
```bash
# In OpenBAO/Vault at path: kv/databases/plex/credentials
{
  "username": "plex_user",
  "password": "randomly-generated-secure-password",
  "database": "plex"
}
```

### VSO in ApplicationSets

VSO is also used by ApplicationSets to inject cluster-specific secrets:

```yaml
# File: clusters/platform/apps/argocd/vso/cluster-secrets.yaml
# Purpose: Create VaultStaticSecrets for each cluster's sensitive configuration

apiVersion: secrets.hashicorp.com/v1beta1
kind: VaultStaticSecret
metadata:
  name: media-cluster-secrets
  namespace: argocd
spec:
  vaultAuthRef: default
  mount: kv
  type: kv-v2
  path: clusters/media/secrets

  destination:
    name: media-cluster-secrets
    create: true

  # Used by Applications deployed to media cluster
  labels:
    lab.gilman.io/cluster-name: media
```

**Benefits of VSO Integration:**
- Secrets stored in Vault, not in Git
- Automatic rotation and refresh
- Audit trail of secret access
- Centralized secret management
- No plaintext secrets in repository

---

## Summary Table

| Component | Purpose | Key Features |
|:---|:---|:---|
| **Cluster Registration** | Enable Argo CD to manage multiple clusters | Platform self-registration, Harvester manual, tenant automatic via XR |
| **cluster-definitions ApplicationSet** | Deploy XR Claims to platform cluster | Git directory discovery, platform-targeted |
| **cluster-apps ApplicationSet** | Deploy apps to correct clusters | Matrix generator, multi-cluster routing |
| **Sync Waves** | Control deployment order | -5 to +2, ensures dependencies |
| **Projects** | Logical grouping and RBAC | clusters, apps, harvester projects |
| **Health Checks** | Custom Crossplane/CAPI status | Lua-based health assessment |
| **VSO Integration** | Secret management | Vault-backed, auto-rotation, no Git secrets |

---

## References

- See main architecture documentation for high-level concepts
- See Appendix D for Harvester-specific configuration
- Argo CD ApplicationSet documentation: https://argo-cd.readthedocs.io/en/stable/user-guide/application-set/
- Vault Secrets Operator: https://github.com/hashicorp/vault-secrets-operator
