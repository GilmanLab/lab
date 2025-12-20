# GitOps Image Pipeline Implementation Spec

## 1. High-Level Objective

* **Goal:** Create a GitOps-driven image management pipeline that declaratively manages machine images (ISOs, raw, qcow2) and distributes them to the lab environment via NAS/NFS.
* **Input:** Declarative YAML configuration defining image sources (HTTP downloads or Packer builds), validation rules, and destination paths.
* **Output:** Validated images in iDrive e2 (S3-compatible), synced to Synology NAS via Cloud Sync.
* **Key Constraint:** Must integrate with existing GitOps patterns (GitHub Actions CI/CD), support multiple image source types, and handle idempotent updates.

## 2. Existing Context (Grounding)

* **Language/Stack:** Go 1.23+, GitHub Actions, iDrive e2 (S3-compatible), Synology Cloud Sync
* **Relevant Files:**
    * `infrastructure/network/vyos/packer/` - Existing Packer build pattern for VyOS images
    * `docs/architecture/08_concepts/storage.md` - NFS storage architecture (images at `/volume1/images`)
    * `docs/architecture/appendices/B_bootstrap_procedure.md` - How images are consumed during PXE provisioning
* **Style Guide:**
    * Configuration files use YAML (consistent with Kubernetes patterns)
    * CLI follows Go best practices (cobra-style commands)
    * Errors must be wrapped with context

## 3. Technical Architecture (The Contract)

### A. Data Structures / Schema

```yaml
# images.yaml - Image pipeline configuration
apiVersion: images.lab.gilman.io/v1alpha1
kind: ImageManifest
metadata:
  name: lab-images
spec:
  defaults:
    # Default destination bucket path prefix
    destinationPrefix: images/
    # Default validation algorithm
    validation:
      algorithm: sha256

  images:
    # HTTP Download Source
    - name: talos-1.9.1
      source:
        type: http
        url: https://factory.talos.dev/image/.../metal-amd64.raw.xz
        checksum: sha256:abc123...
        # Optional: decompress after download
        decompress: xz
      destination:
        path: talos/talos-1.9.1-amd64.raw
      validation:
        algorithm: sha256
        expected: sha256:def456...  # Post-decompression checksum

    # Packer Build Source
    - name: vyos-gateway
      source:
        type: packer
        path: infrastructure/network/vyos/packer
        variables:
          vyos_iso_url: https://github.com/vyos/vyos-rolling-nightly-builds/...
          vyos_iso_checksum: sha256:...
        # Output artifact from Packer
        artifact: output/vyos-lab.raw
      destination:
        path: vyos/vyos-gateway.raw
      # No validation block needed - checksum computed after build and stored in metadata

    # ISO passthrough (no transformation)
    - name: harvester-1.4.0
      source:
        type: http
        url: https://releases.rancher.com/harvester/v1.4.0/harvester-v1.4.0-amd64.iso
        checksum: sha256:...
      destination:
        path: harvester/harvester-1.4.0-amd64.iso
      # No validation.expected needed - defaults to source.checksum when no decompress
```

```go
// Core types for the CLI
package config

type ImageManifest struct {
    APIVersion string   `yaml:"apiVersion"`
    Kind       string   `yaml:"kind"`
    Metadata   Metadata `yaml:"metadata"`
    Spec       Spec     `yaml:"spec"`
}

type Spec struct {
    Defaults Defaults `yaml:"defaults"`
    Images   []Image  `yaml:"images"`
}

type Image struct {
    Name        string      `yaml:"name"`
    Source      Source      `yaml:"source"`
    Destination Destination `yaml:"destination"`
    Validation  *Validation `yaml:"validation,omitempty"`
}

type Source struct {
    Type       string            `yaml:"type"` // "http" | "packer"
    URL        string            `yaml:"url,omitempty"`
    Checksum   string            `yaml:"checksum,omitempty"`
    Decompress string            `yaml:"decompress,omitempty"`
    Path       string            `yaml:"path,omitempty"`       // Packer directory
    Variables  map[string]string `yaml:"variables,omitempty"`  // Packer variables
    Artifact   string            `yaml:"artifact,omitempty"`   // Packer output
}

type Destination struct {
    Path string `yaml:"path"` // S3 key path
}

type Validation struct {
    Algorithm string `yaml:"algorithm"`          // "sha256" | "sha512" (default: sha256)
    Expected  string `yaml:"expected,omitempty"` // Post-processing checksum
    // If omitted: defaults to source.checksum (HTTP) or computed after build (Packer)
    // Required when: decompress is used (checksum changes after decompression)
}

// e2.sops.yaml structure (decrypted)
type E2Credentials struct {
    AccessKey string `yaml:"access_key"`
    SecretKey string `yaml:"secret_key"`
    Endpoint  string `yaml:"endpoint"`
    Bucket    string `yaml:"bucket"`
}
```

### B. Interface Definitions

```go
// ImageSource abstracts different image acquisition strategies
type ImageSource interface {
    // Acquire downloads or builds the image, returning path to local file
    Acquire(ctx context.Context, workDir string) (string, error)
    // Name returns a human-readable identifier
    Name() string
}

// HTTPSource implements ImageSource for HTTP downloads
type HTTPSource struct {
    URL        string
    Checksum   string
    Decompress string
}

// PackerSource implements ImageSource for Packer builds
type PackerSource struct {
    Path      string
    Variables map[string]string
    Artifact  string
}

// ImageStore abstracts the storage backend
type ImageStore interface {
    // Upload stores an image at the given key
    Upload(ctx context.Context, key string, localPath string) error
    // Exists checks if an image exists at the given key
    Exists(ctx context.Context, key string) (bool, error)
    // GetChecksum retrieves the stored checksum for an image
    GetChecksum(ctx context.Context, key string) (string, error)
    // Delete removes an image
    Delete(ctx context.Context, key string) error
    // List returns all images under a prefix
    List(ctx context.Context, prefix string) ([]string, error)
}

// S3Store implements ImageStore for S3-compatible backends
type S3Store struct {
    client     *s3.Client
    bucket     string
    endpoint   string
}

// Validator abstracts validation strategies
type Validator interface {
    Validate(ctx context.Context, localPath string) error
}

// CredentialResolver abstracts credential loading
type CredentialResolver interface {
    // Resolve returns e2 credentials from env vars or SOPS file
    Resolve(ctx context.Context) (*E2Credentials, error)
}

// EnvCredentialResolver loads from environment variables
type EnvCredentialResolver struct{}

// SOPSCredentialResolver decrypts SOPS-encrypted file
type SOPSCredentialResolver struct {
    CredentialsPath string
    AgeKeyPath      string
}

// ChainedResolver tries resolvers in order until one succeeds
type ChainedResolver struct {
    Resolvers []CredentialResolver
}
```

### C. File Structure Plan

```
tools/
└── labctl/                          # New CLI tool
    ├── cmd/
    │   └── images/
    │       ├── sync.go              # sync command - process manifest
    │       ├── validate.go          # validate command - check manifest
    │       ├── list.go              # list command - show stored images
    │       └── prune.go             # prune command - remove orphaned images
    ├── internal/
    │   ├── config/
    │   │   └── manifest.go          # YAML parsing, validation
    │   ├── credentials/
    │   │   ├── resolver.go          # CredentialResolver interface
    │   │   ├── env.go               # Environment variable resolver
    │   │   └── sops.go              # SOPS file resolver (uses go.mozilla.org/sops/v3)
    │   ├── source/
    │   │   ├── source.go            # ImageSource interface
    │   │   ├── http.go              # HTTP download implementation
    │   │   └── packer.go            # Packer build implementation
    │   ├── store/
    │   │   └── s3.go                # S3Store implementation
    │   └── validator/
    │       └── checksum.go          # Checksum validation
    ├── main.go
    └── go.mod

images/                              # New top-level directory
├── images.yaml                      # Image manifest (source of truth)
├── e2.sops.yaml                     # e2 credentials (SOPS encrypted)
└── .sops.yaml                       # SOPS configuration (age public key)

.github/workflows/
└── images-sync.yml                  # GitHub Actions workflow
```

## 4. Implementation Steps (Prompt Chain)

1. **Define Types:** Create the core config structs in `tools/labctl/internal/config/manifest.go`
2. **Implement S3 Store:** Create S3-compatible storage backend in `tools/labctl/internal/store/s3.go`
3. **Implement HTTP Source:** Create HTTP download + decompression in `tools/labctl/internal/source/http.go`
4. **Implement Packer Source:** Create Packer build wrapper in `tools/labctl/internal/source/packer.go`
5. **Implement Sync Command:** Wire together sources, validation, and storage in `tools/labctl/cmd/images/sync.go`
6. **Create Manifest:** Define initial image manifest in `images/images.yaml`
7. **Create Workflow:** Define GitHub Actions workflow in `.github/workflows/images-sync.yml`
8. **Testing:** Write unit tests for each component

## 5. Verification

* **Success Criteria:** `labctl images sync` successfully processes the manifest and uploads images to e2
* **Logs:** Must log each image processing step at INFO level, errors at ERROR level
* **Idempotency:** Running sync twice should skip already-uploaded images (based on checksum)

---

## 6. Detailed Design

### 6.1 Configuration Schema

The configuration schema supports two primary source types:

#### HTTP Sources

For pre-built images available via HTTP/HTTPS:

```yaml
- name: talos-1.9.1
  source:
    type: http
    url: https://factory.talos.dev/image/.../metal-amd64.raw.xz
    checksum: sha256:abc123...  # Pre-download checksum verification
    decompress: xz              # Optional: xz, gzip, zstd
  destination:
    path: talos/talos-1.9.1-amd64.raw
  validation:
    algorithm: sha256
    expected: sha256:def456...  # Required when decompress is used
```

#### Packer Sources

For custom-built images using existing Packer templates:

```yaml
- name: vyos-gateway
  source:
    type: packer
    path: infrastructure/network/vyos/packer  # Relative to repo root
    variables:
      vyos_iso_url: https://...
      vyos_iso_checksum: sha256:...
    artifact: output/vyos-lab.raw  # Relative to Packer path
  destination:
    path: vyos/vyos-gateway.raw
```

### 6.2 CLI Architecture

The CLI uses a subcommand structure under `labctl images`:

```
labctl images sync [flags]
    Process the image manifest, downloading/building and uploading images.

    --manifest PATH           Path to images.yaml (default: ./images/images.yaml)
    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key for SOPS decryption
    --dry-run                 Show what would be done without executing
    --force                   Force re-upload even if checksums match

labctl images validate [--manifest PATH]
    Validate the manifest syntax and check source availability.
    Does not require credentials.

    Checks performed:
    - YAML syntax and schema validation
    - HTTP sources: HEAD request to verify URL exists and get Content-Length
    - Packer sources: Verify directory exists and contains *.pkr.hcl files
    - All URLs use HTTPS
    - All HTTP sources have source.checksum specified
    - Decompressed sources have validation.expected specified

labctl images list [flags]
    List images currently stored in e2.

    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key for SOPS decryption
    --prefix PATH             Filter by path prefix

labctl images prune [flags]
    Remove images from e2 that are not in the manifest.

    --manifest PATH           Path to images.yaml (default: ./images/images.yaml)
    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key for SOPS decryption
    --dry-run                 Show what would be removed without executing
```

**Credential Resolution Order:**
1. Environment variables: `E2_ACCESS_KEY`, `E2_SECRET_KEY`, `E2_ENDPOINT`, `E2_BUCKET`
2. SOPS file (if `--credentials` and `--sops-age-key-file` provided)
3. Error if no credentials found (except for `validate` and `--dry-run`)

**Error Handling:**
- Wrap all errors with context using `fmt.Errorf("operation: %w", err)`
- Non-zero exit code on any failure
- Continue processing other images after individual failures (collect and report at end)

**Progress Reporting:**
- Use structured logging (slog)
- Show progress for downloads: bytes downloaded, percentage
- Show Packer build output in real-time

### 6.3 Image Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Image Lifecycle                                   │
└─────────────────────────────────────────────────────────────────────────┘

1. DECLARATION
   └─> images/images.yaml updated in Git

2. TRIGGER
   └─> GitHub Actions workflow triggered on:
       - Push to main affecting images/** or infrastructure/**/packer/**
       - Manual workflow dispatch

3. ACQUISITION
   └─> labctl images sync
       ├─> HTTP: Download → Verify → Decompress
       └─> Packer: Build → Collect artifact

4. VALIDATION
   └─> Verify final artifact checksum (if specified)

5. UPLOAD
   └─> Upload to iDrive e2 bucket
       └─> Store metadata (checksum, timestamp, source info)

6. SYNC TO NAS
   └─> Synology Cloud Sync pulls from e2
       └─> Images available at /volume1/images/

7. CONSUMPTION
   └─> Tinkerbell/PXE references images from NAS
```

**Versioning Strategy:**
- Image names include version: `talos-1.9.1`, `vyos-gateway-20241220`
- Old versions are retained until explicitly removed via `prune` or manifest update
- Manifest is the source of truth for which versions should exist

**Update Flow:**
1. Update image version in manifest (e.g., `talos-1.9.1` → `talos-1.9.2`)
2. Commit and push to main
3. Workflow runs, uploads new version
4. (Optional) Run `prune` to remove old version

### 6.4 Validation Strategies

| Strategy | Use Case | Implementation |
|:---------|:---------|:---------------|
| `checksum` | HTTP downloads, post-build verification | SHA256/SHA512 hash comparison |

> **Security Policy:** All HTTP sources MUST use HTTPS and specify a `source.checksum`.
> The `validation: none` option is intentionally omitted - every image must be validated.

**Enforced Requirements:**
- HTTP URLs must use `https://` scheme (CLI rejects `http://`)
- `source.checksum` is required for all HTTP sources (pre-download verification)
- `validation.expected` is required when `decompress` is used (post-decompression verification)
- Packer sources compute checksum after build (stored in metadata for future comparison)

**Default Behavior for `validation.expected`:**
- **HTTP without decompress**: Defaults to `source.checksum` (file unchanged after download)
- **HTTP with decompress**: Must be explicitly specified (checksum changes after decompression)
- **Packer**: Not applicable - checksum computed after build and stored in metadata

**Checksum Workflow:**

```
HTTP Source:
  1. Fetch Content-Length for progress tracking
  2. Download to temp file with streaming hash
  3. Verify downloaded checksum matches source.checksum
  4. If decompress specified: decompress to new temp file
  5. Verify decompressed checksum matches validation.expected

Packer Source:
  1. Run packer build
  2. Compute checksum of artifact
  3. Store checksum as metadata in S3
```

### 6.5 S3 Bucket Structure

```
lab-images/                          # Bucket name
├── images/                          # Image files
│   ├── talos/
│   │   ├── talos-1.9.1-amd64.raw
│   │   └── talos-1.9.2-amd64.raw
│   ├── vyos/
│   │   └── vyos-gateway.raw
│   └── harvester/
│       └── harvester-1.4.0-amd64.iso
│
└── metadata/                        # Image metadata (JSON)
    ├── talos/
    │   ├── talos-1.9.1-amd64.raw.json
    │   └── talos-1.9.2-amd64.raw.json
    ├── vyos/
    │   └── vyos-gateway.raw.json
    └── harvester/
        └── harvester-1.4.0-amd64.iso.json
```

**Metadata Schema:**

```json
{
  "name": "talos-1.9.1",
  "checksum": "sha256:abc123...",
  "size": 1234567890,
  "uploadedAt": "2024-12-20T10:00:00Z",
  "source": {
    "type": "http",
    "url": "https://factory.talos.dev/..."
  }
}
```

#### Checksum Storage and Idempotency

Checksums are stored in the metadata JSON file, **not** derived from S3 ETags (which are unreliable for multipart uploads).

**Sync Idempotency Flow:**

```
1. Parse manifest, get expected checksum for image
2. Check if metadata/<path>.json exists in S3
   ├── No  → Image missing, proceed to acquire/upload
   └── Yes → Read metadata, compare checksum
             ├── Match    → Skip (already uploaded)
             └── Mismatch → Re-acquire and upload
3. After upload, write metadata/<path>.json with computed checksum
```

**Checksum Comparison:**
- For HTTP sources: Compare manifest `validation.expected` against stored `checksum`
- For Packer sources: Checksum computed after build, compared against stored `checksum`
- `--force` bypasses comparison and always re-uploads

**Upload Behavior:**
- Same path can be overwritten (mutable uploads)
- `--force` forces re-upload regardless of checksum match
- Metadata is always updated on upload

### 6.6 GitHub Actions Workflow

```yaml
# .github/workflows/images-sync.yml
name: Sync Images

on:
  push:
    branches: [main]
    paths:
      - 'images/**'
      - 'infrastructure/**/packer/**'  # Recursive match for nested paths
  workflow_dispatch:
    inputs:
      force:
        description: 'Force re-upload all images'
        required: false
        type: boolean
        default: false
      prune:
        description: 'Run prune after sync'
        required: false
        type: boolean
        default: false

# Prevent concurrent runs to avoid sync/prune race conditions
concurrency:
  group: images-sync
  cancel-in-progress: false  # Let running jobs complete

jobs:
  sync:
    runs-on: ubuntu-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Setup Packer
        uses: hashicorp/setup-packer@v3.1.0  # Pin to specific version
        with:
          version: '1.11.2'

      - name: Build labctl
        run: go build -o labctl ./tools/labctl

      - name: Validate Manifest
        run: ./labctl images validate

      - name: Write SOPS age key
        run: |
          echo "${{ secrets.SOPS_AGE_KEY }}" > /tmp/age-key.txt
          chmod 600 /tmp/age-key.txt

      - name: Sync Images
        run: |
          FLAGS=""
          if [ "${{ inputs.force }}" == "true" ]; then
            FLAGS="--force"
          fi
          ./labctl images sync \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt \
            $FLAGS

      - name: Prune Orphaned Images
        if: inputs.prune == true
        run: |
          ./labctl images prune \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt
```

> **Note:** Prune is manual-only (`workflow_dispatch` with `prune: true`) to prevent
> accidental deletion. Automatic pruning on every sync risks race conditions and
> unintended image removal.

**Workflow Characteristics:**
- **Triggers:** Push to main (relevant paths) or manual dispatch
- **Caching:** Packer plugins cached between runs
- **Failure Handling:** Each image is processed independently; partial success is reported
- **Secrets:** Single `SOPS_AGE_KEY` secret decrypts `images/e2.sops.yaml`

### 6.7 Security Considerations

| Concern | Mitigation |
|:--------|:-----------|
| **e2 Credentials** | SOPS-encrypted in Git; decrypted at runtime |
| **Checksum Verification** | Pre-download checksums prevent MITM attacks |
| **Packer Variables** | Secrets passed via environment variables, not command line |
| **SSH Keys (VyOS)** | Generated per-build; not stored in manifest |
| **S3 Bucket Access** | Private bucket; Cloud Sync uses separate read-only credentials |
| **SOPS Key** | Single GitHub secret; unlocks all encrypted files |

#### SOPS-Encrypted Credentials

e2 credentials are stored encrypted in Git using SOPS with age encryption:

```yaml
# images/e2.sops.yaml (encrypted)
# Only access_key and secret_key are sensitive; endpoint/bucket are public
access_key: ENC[AES256_GCM,data:...,type:str]
secret_key: ENC[AES256_GCM,data:...,type:str]
endpoint: https://e2.idrive.com        # Not encrypted (public endpoint)
bucket: lab-images                      # Not encrypted (public bucket name)
sops:
    age:
        - recipient: age1...
    encrypted_regex: ^(access_key|secret_key)$
    version: 3.8.1
```

```yaml
# images/.sops.yaml
creation_rules:
  - path_regex: .*\.sops\.yaml$
    key_groups:
      - age:
          - age1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  # CI key
        pgp:
          - XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX  # Yubikey (personal)
```

#### CLI Credential Resolution

The CLI resolves credentials in this order (first match wins):

1. **Environment variables**: `E2_ACCESS_KEY`, `E2_SECRET_KEY`, `E2_ENDPOINT`, `E2_BUCKET`
2. **SOPS-encrypted file**: `--credentials` flag (optionally with `--sops-age-key-file`)

```bash
# Option 1: Environment variables (override)
export E2_ACCESS_KEY=...
export E2_SECRET_KEY=...
labctl images sync

# Option 2: SOPS with PGP/Yubikey (local dev)
# gpg-agent handles decryption automatically
labctl images sync --credentials images/e2.sops.yaml

# Option 3: SOPS with age key file (CI)
labctl images sync \
  --credentials images/e2.sops.yaml \
  --sops-age-key-file /tmp/age-key.txt
```

#### Credential Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Git Repository                                  │
│                                                                          │
│   images/                                                                │
│   ├── images.yaml          # Image manifest (plaintext)                  │
│   ├── e2.sops.yaml         # e2 credentials (SOPS encrypted)            │
│   └── .sops.yaml           # SOPS config (age + PGP keys)               │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
              Local Dev                        GitHub Actions
                    │                               │
         ┌──────────▼──────────┐       ┌───────────▼───────────┐
         │  Yubikey (PGP)      │       │  SOPS_AGE_KEY secret  │
         │  gpg-agent handles  │       │  written to temp file │
         │  decryption         │       └───────────┬───────────┘
         └──────────┬──────────┘                   │
                    │                               │
                    └───────────────┬───────────────┘
                                    │
                         ┌──────────▼──────────┐
                         │      labctl         │
                         │  Decrypts e2.sops   │
                         └──────────┬──────────┘
                                    │
                         ┌──────────▼──────────┐
                         │     iDrive e2       │
                         └─────────────────────┘
```

#### Why SOPS over GitHub Secrets?

| Aspect | GitHub Secrets | SOPS in Git |
|:-------|:---------------|:------------|
| **Auditability** | Separate UI, no history | Full Git history |
| **GitOps alignment** | Secrets outside Git | Secrets in Git (encrypted) |
| **Rotation** | Manual UI update | Commit new encrypted file |
| **Local dev** | Export from UI | Same file as CI |
| **Single secret** | One per credential | One age key unlocks all |

#### Key Management

**Two keys encrypt each secret:**

| Key | Type | Used By | Storage |
|:----|:-----|:--------|:--------|
| CI key | age | GitHub Actions | `SOPS_AGE_KEY` secret |
| Personal key | PGP | Local dev | Yubikey (hardware) |

**Setup:**
```bash
# Generate age key for CI
age-keygen -o ci-age-key.txt
# Add public key to images/.sops.yaml
# Store private key in GitHub secret: SOPS_AGE_KEY

# Get PGP fingerprint from Yubikey
gpg --card-status | grep 'sec'
# Add fingerprint to images/.sops.yaml
```

**Local usage:**
```bash
# SOPS automatically uses gpg-agent → Yubikey
# No --sops-age-key-file needed locally
labctl images sync --credentials images/e2.sops.yaml
```

**Rotation:**
- **Age key (CI)**: Generate new key, run `sops updatekeys images/e2.sops.yaml`, update GitHub secret
- **PGP key**: Update fingerprint in `.sops.yaml`, run `sops updatekeys`

#### Dry-Run Without Credentials

`--dry-run` mode works without credentials for validation:

```bash
# Validates manifest syntax, checks source URLs, but doesn't upload
labctl images sync --dry-run
```

### 6.8 Synology Cloud Sync Configuration

Cloud Sync is configured manually on the Synology NAS:

1. Create Cloud Sync task
2. Provider: S3-compatible (iDrive e2)
3. Bucket: `lab-images`
4. Remote path: `images/`
5. Local path: `/volume1/images/`
6. Sync direction: **Download only** (remote → local)
7. Schedule: Continuous or every 5 minutes

This configuration is not GitOps-managed (Synology limitation) but is documented in `docs/architecture/appendices/` for disaster recovery.

---

## 7. Example Configuration

```yaml
# images/images.yaml
apiVersion: images.lab.gilman.io/v1alpha1
kind: ImageManifest
metadata:
  name: lab-images
spec:
  defaults:
    destinationPrefix: images/
    validation:
      algorithm: sha256

  images:
    # Talos Linux for platform and tenant clusters
    - name: talos-1.9.1
      source:
        type: http
        url: https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.9.1/metal-amd64.raw.xz
        checksum: sha256:...
        decompress: xz
      destination:
        path: talos/talos-1.9.1-amd64.raw
      validation:
        algorithm: sha256
        expected: sha256:...

    # VyOS Gateway Router
    - name: vyos-gateway
      source:
        type: packer
        path: infrastructure/network/vyos/packer
        variables:
          vyos_iso_url: https://github.com/vyos/vyos-rolling-nightly-builds/releases/download/1.5-rolling-202412190007/vyos-1.5-rolling-202412190007-amd64.iso
          vyos_iso_checksum: sha256:...
        artifact: output/vyos-lab.raw
      destination:
        path: vyos/vyos-gateway.raw

    # Harvester HCI
    - name: harvester-1.4.0
      source:
        type: http
        url: https://releases.rancher.com/harvester/v1.4.0/harvester-v1.4.0-amd64.iso
        checksum: sha256:...
      destination:
        path: harvester/harvester-1.4.0-amd64.iso
```

---

## 8. Open Questions

1. **Packer Secret Handling:** How should sensitive Packer variables (SSH keys) be injected? Options:
   - Generate per-build in workflow (recommended)
   - Store in SOPS-encrypted file alongside e2 credentials

2. **Parallel Builds:** Should Packer builds run in parallel? Consider GitHub Actions runner resources.

## 8.1 Resolved Decisions

| Question | Decision |
|:---------|:---------|
| **Image Retention** | Explicit deletion only via manifest removal + manual prune |
| **Upload Immutability** | Mutable - same path can be overwritten; `--force` bypasses checksum |
| **Validate Behavior** | HEAD requests for HTTP, directory check for Packer (no builds) |
| **HTTPS Requirement** | Enforced - CLI rejects `http://` URLs |
| **Checksum Requirement** | Enforced - all HTTP sources must specify `source.checksum` |

---

## 9. Future Considerations

- **Image Signing:** GPG or Sigstore signing for supply chain security
- **Multi-Architecture:** Support for arm64 images (future hardware)
- **Caching:** Local cache of downloaded images to speed up workflow runs
- **Notifications:** Slack/Discord notifications on build failures
