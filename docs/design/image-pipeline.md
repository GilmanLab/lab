# GitOps Image Pipeline Implementation Spec

## 1. High-Level Objective

* **Goal:** Create a GitOps-driven pipeline that manages source images (ISOs, raw, qcow2) and distributes them to the lab via NAS/NFS.
* **Input:** Declarative YAML configuration defining image sources, validation rules, and optional file updates.
* **Output:** Validated images in iDrive e2 (S3-compatible), synced to Synology NAS via Cloud Sync.
* **Key Constraint:** Downstream builds (Packer) are triggered via Git changes, not direct invocation.

## 2. Existing Context

* **Language/Stack:** Go 1.23+, GitHub Actions, iDrive e2, Synology Cloud Sync, Mergify
* **Relevant Files:**
    * `infrastructure/network/vyos/packer/` - Existing Packer build (consumes source images)
    * `docs/architecture/08_concepts/storage.md` - NFS storage architecture
* **Style Guide:**
    * Configuration files use YAML
    * CLI follows Go best practices (cobra-style commands)

## 3. Technical Architecture

### A. Configuration Schema

```yaml
# images/images.yaml
apiVersion: images.lab.gilman.io/v1alpha1
kind: ImageManifest
metadata:
  name: lab-images
spec:
  images:
    # Simple source image (download and upload)
    - name: talos-1.9.1
      source:
        url: https://factory.talos.dev/image/.../metal-amd64.raw.xz
        checksum: sha256:abc123...
        decompress: xz  # Optional: xz, gzip, zstd
      destination: talos/talos-1.9.1-amd64.raw
      validation:
        algorithm: sha256
        expected: sha256:def456...  # Post-decompression checksum

    # Source image that triggers downstream build
    - name: vyos-iso
      source:
        url: https://github.com/vyos/vyos-rolling-nightly-builds/releases/download/1.5-rolling-202412190007/vyos-1.5-rolling-202412190007-amd64.iso
        checksum: sha256:abc123...
      destination: vyos/vyos-1.5-rolling-202412190007.iso
      updateFile:
        path: infrastructure/network/vyos/packer/source.auto.pkrvars.hcl
        replacements:
          - pattern: 'vyos_iso_url\s*=\s*"[^"]*"'
            value: 'vyos_iso_url = "{{ .Source.URL }}"'
          - pattern: 'vyos_iso_checksum\s*=\s*"[^"]*"'
            value: 'vyos_iso_checksum = "{{ .Source.Checksum }}"'

    # Harvester ISO (no transformation)
    - name: harvester-1.4.0
      source:
        url: https://releases.rancher.com/harvester/v1.4.0/harvester-v1.4.0-amd64.iso
        checksum: sha256:...
      destination: harvester/harvester-1.4.0-amd64.iso
```

### B. Data Structures

```go
package config

type ImageManifest struct {
    APIVersion string   `yaml:"apiVersion"`
    Kind       string   `yaml:"kind"`
    Metadata   Metadata `yaml:"metadata"`
    Spec       Spec     `yaml:"spec"`
}

type Spec struct {
    Images []Image `yaml:"images"`
}

type Image struct {
    Name        string      `yaml:"name"`
    Source      Source      `yaml:"source"`
    Destination string      `yaml:"destination"`
    Validation  *Validation `yaml:"validation,omitempty"`
    UpdateFile  *UpdateFile `yaml:"updateFile,omitempty"`
}

type Source struct {
    URL        string `yaml:"url"`
    Checksum   string `yaml:"checksum"`
    Decompress string `yaml:"decompress,omitempty"` // xz, gzip, zstd
}

type Validation struct {
    Algorithm string `yaml:"algorithm"` // sha256, sha512
    Expected  string `yaml:"expected"`  // Required when decompress is used
}

type UpdateFile struct {
    Path         string        `yaml:"path"`
    Replacements []Replacement `yaml:"replacements"`
}

type Replacement struct {
    Pattern string `yaml:"pattern"` // Regex pattern
    Value   string `yaml:"value"`   // Replacement with template vars: {{ .Source.URL }}, {{ .Source.Checksum }}
}

// Credentials (from SOPS-encrypted file)
type E2Credentials struct {
    AccessKey string `yaml:"access_key"`
    SecretKey string `yaml:"secret_key"`
    Endpoint  string `yaml:"endpoint"`
    Bucket    string `yaml:"bucket"`
}
```

### C. File Structure

```
tools/
└── labctl/
    ├── cmd/
    │   └── images/
    │       ├── sync.go       # Download, upload, update files, create PR
    │       ├── validate.go   # Check manifest syntax and URLs
    │       ├── list.go       # List stored images
    │       └── prune.go      # Remove orphaned images
    ├── internal/
    │   ├── config/
    │   │   └── manifest.go   # YAML parsing
    │   ├── credentials/
    │   │   ├── env.go        # Environment variable resolver
    │   │   └── sops.go       # SOPS file resolver
    │   ├── store/
    │   │   └── s3.go         # S3-compatible storage
    │   └── updater/
    │       └── file.go       # Regex-based file updates
    ├── main.go
    └── go.mod

images/
├── images.yaml               # Image manifest
├── e2.sops.yaml              # e2 credentials (SOPS encrypted)
└── .sops.yaml                # SOPS config (age + PGP keys)

.github/workflows/
├── images-sync.yml           # Source image pipeline
└── packer-vyos.yml           # VyOS image build (triggered by file change)
```

## 4. CLI Interface

```
labctl images sync [flags]
    Download source images, upload to e2, update files, create PR if needed.

    --manifest PATH           Path to images.yaml (default: ./images/images.yaml)
    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key for SOPS decryption
    --dry-run                 Show what would be done without executing
    --force                   Force re-upload even if checksums match

labctl images validate [--manifest PATH]
    Validate manifest syntax and check source URLs (HEAD requests).

labctl images list [flags]
    List images stored in e2.

    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key

labctl images prune [flags]
    Remove images from e2 not in manifest. Manual-only (not run automatically).

    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key
    --dry-run                 Show what would be removed
```

**Credential Resolution Order:**
1. Environment variables: `E2_ACCESS_KEY`, `E2_SECRET_KEY`, `E2_ENDPOINT`, `E2_BUCKET`
2. SOPS file via `--credentials` (uses gpg-agent for PGP or `--sops-age-key-file` for age)

## 5. Image Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Source Images                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. DECLARATION                                                              │
│     └─> images/images.yaml updated with new source URL/checksum             │
│                                                                              │
│  2. SYNC WORKFLOW (images-sync.yml)                                         │
│     └─> labctl images sync                                                  │
│         ├─> Download source image                                           │
│         ├─> Verify checksum                                                 │
│         ├─> Decompress if needed                                            │
│         ├─> Upload to e2                                                    │
│         └─> If updateFile specified:                                        │
│             ├─> Apply regex replacements to file                            │
│             └─> Create PR with changes                                      │
│                                                                              │
│  3. AUTO-MERGE (Mergify)                                                    │
│     └─> PR auto-merged if:                                                  │
│         ├─> Author is github-actions[bot]                                   │
│         ├─> Label is "automated"                                            │
│         └─> CI checks pass                                                  │
│                                                                              │
│  4. CLOUD SYNC                                                              │
│     └─> Synology pulls from e2 to /volume1/images/                          │
│                                                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                              Derived Images                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  5. PACKER WORKFLOW (packer-vyos.yml)                                       │
│     └─> Triggered by changes to source.auto.pkrvars.hcl                     │
│         ├─> packer init && packer build                                     │
│         ├─> Upload built image to e2                                        │
│         └─> Cloud Sync pulls to NAS                                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 6. Idempotency

**Checksum Comparison:**
```
1. Compute effective checksum: validation.expected ?? source.checksum
2. Check if metadata/<path>.json exists in e2
   ├── No  → Download and upload
   └── Yes → Compare effective checksum against stored checksum
             ├── Match    → Skip (already uploaded)
             └── Mismatch → Re-download and upload
3. After upload, write metadata with checksum
```

**Metadata Schema:**
```json
{
  "name": "talos-1.9.1",
  "checksum": "sha256:abc123...",
  "size": 1234567890,
  "uploadedAt": "2024-12-20T10:00:00Z",
  "source": {
    "url": "https://factory.talos.dev/..."
  }
}
```

## 7. S3 Bucket Structure

```
lab-images/
├── images/
│   ├── talos/
│   │   └── talos-1.9.1-amd64.raw
│   ├── vyos/
│   │   ├── vyos-1.5-rolling-202412190007.iso    # Source ISO
│   │   └── vyos-gateway.raw                      # Built by Packer
│   └── harvester/
│       └── harvester-1.4.0-amd64.iso
└── metadata/
    ├── talos/
    │   └── talos-1.9.1-amd64.raw.json
    ├── vyos/
    │   ├── vyos-1.5-rolling-202412190007.iso.json
    │   └── vyos-gateway.raw.json
    └── harvester/
        └── harvester-1.4.0-amd64.iso.json
```

## 8. GitHub Actions Workflows

### 8.1 Source Image Sync (images-sync.yml)

```yaml
name: Sync Images

on:
  push:
    branches: [main]
    paths:
      - 'images/**'
  workflow_dispatch:
    inputs:
      force:
        description: 'Force re-upload all images'
        type: boolean
        default: false
      prune:
        description: 'Run prune after sync'
        type: boolean
        default: false

concurrency:
  group: images-sync
  cancel-in-progress: false

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Build labctl
        run: go build -o labctl ./tools/labctl

      - name: Write SOPS age key
        run: |
          echo "${{ secrets.SOPS_AGE_KEY }}" > /tmp/age-key.txt
          chmod 600 /tmp/age-key.txt

      - name: Sync Images
        id: sync
        run: |
          FLAGS=""
          if [ "${{ inputs.force }}" == "true" ]; then FLAGS="--force"; fi

          ./labctl images sync \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt \
            $FLAGS

      - name: Create PR if files changed
        if: steps.sync.outputs.files_changed == 'true'
        uses: peter-evans/create-pull-request@v5
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          commit-message: 'chore: update source image references'
          title: 'chore: update source image references'
          body: |
            Automated update of source image references.

            Updated by `labctl images sync`.
          branch: automated/image-updates
          labels: automated
          delete-branch: true

      - name: Prune Orphaned Images
        if: inputs.prune == true
        run: |
          ./labctl images prune \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt
```

### 8.2 Packer Build (packer-vyos.yml)

```yaml
name: Build VyOS Image

on:
  push:
    branches: [main]
    paths:
      - 'infrastructure/network/vyos/packer/**'
  workflow_dispatch:

concurrency:
  group: packer-vyos
  cancel-in-progress: false

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: hashicorp/setup-packer@v3.1.0
        with:
          version: '1.11.2'

      - name: Packer Init
        run: packer init infrastructure/network/vyos/packer

      - name: Packer Build
        run: |
          packer build \
            -var "ssh_public_key=$(cat ~/.ssh/id_ed25519.pub)" \
            infrastructure/network/vyos/packer

      - name: Write SOPS age key
        run: |
          echo "${{ secrets.SOPS_AGE_KEY }}" > /tmp/age-key.txt
          chmod 600 /tmp/age-key.txt

      - name: Upload to e2
        run: |
          # Use AWS CLI or labctl to upload
          # Output is at infrastructure/network/vyos/packer/output/vyos-lab.raw
          ./labctl images upload \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt \
            --source infrastructure/network/vyos/packer/output/vyos-lab.raw \
            --destination vyos/vyos-gateway.raw
```

### 8.3 Mergify Configuration (.mergify.yml)

```yaml
pull_request_rules:
  - name: Auto-merge automated image updates
    conditions:
      - author=github-actions[bot]
      - label=automated
      - base=main
      - "#approved-reviews-by>=0"  # No approval required for bot PRs
      - check-success=sync         # CI must pass
    actions:
      merge:
        method: squash
        commit_message_template: |
          {{ title }}

          {{ body }}
```

## 9. Security

### SOPS-Encrypted Credentials

```yaml
# images/e2.sops.yaml (encrypted)
access_key: ENC[AES256_GCM,data:...,type:str]
secret_key: ENC[AES256_GCM,data:...,type:str]
endpoint: https://e2.idrive.com
bucket: lab-images
sops:
    age:
        - recipient: age1...  # CI key
    pgp:
        - XXXX...             # Yubikey
    encrypted_regex: ^(access_key|secret_key)$
```

```yaml
# images/.sops.yaml
creation_rules:
  - path_regex: .*\.sops\.yaml$
    key_groups:
      - age:
          - age1...  # CI key
        pgp:
          - XXXX...  # Yubikey
```

### Validation Requirements

- All URLs must use HTTPS (CLI rejects `http://`)
- `source.checksum` required for all images
- `validation.expected` required when `decompress` is used

## 10. Synology Cloud Sync

Configured manually on NAS (not GitOps-managed):

1. Provider: S3-compatible (iDrive e2)
2. Bucket: `lab-images`
3. Remote path: `images/`
4. Local path: `/volume1/images/`
5. Direction: Download only
6. Schedule: Continuous or every 5 minutes

## 11. Open Questions

1. **Packer SSH Keys:** Generate per-build in workflow or store encrypted?
2. **Image Retention:** Keep all versions until explicit prune, or auto-expire?

## 12. Future Considerations

- Image signing (Sigstore/GPG)
- Multi-architecture support (arm64)
- Slack/Discord notifications on failures
