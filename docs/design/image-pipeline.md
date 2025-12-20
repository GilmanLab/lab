# GitOps Image Pipeline Implementation Spec

## 1. High-Level Objective

* **Goal:** Create a GitOps-driven pipeline that manages source images (ISOs, raw, qcow2) and distributes them to the lab via NAS/NFS.
* **Input:** Declarative YAML configuration defining image sources, validation rules, and optional file updates.
* **Output:** Validated images in iDrive e2 (S3-compatible), synced to Synology NAS via Cloud Sync.

## 2. Existing Context

* **Language/Stack:** Go 1.23+, GitHub Actions, iDrive e2, Synology Cloud Sync, Mergify
* **Relevant Files:**
    * `infrastructure/network/vyos/vyos-build/` - VyOS image build using vyos-build toolchain
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

    # VyOS ISO for reference/manual builds
    - name: vyos-iso
      source:
        url: https://github.com/vyos/vyos-rolling-nightly-builds/releases/download/1.5-rolling-202412190007/vyos-1.5-rolling-202412190007-amd64.iso
        checksum: sha256:abc123...
      destination: vyos/vyos-1.5-rolling-202412190007.iso

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
    │       ├── sync.go       # Download, upload, update files, set outputs
    │       ├── validate.go   # Check manifest syntax and URLs
    │       ├── list.go       # List stored images
    │       ├── prune.go      # Remove orphaned images
    │       └── upload.go     # Upload local file to e2
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
├── packer-ssh.sops.yaml      # SSH keypair for image builds (SOPS encrypted)
└── .sops.yaml                # SOPS config (age + PGP keys)

.github/workflows/
├── images-sync.yml           # Source image pipeline
└── vyos-build.yml            # VyOS image build using vyos-build toolchain
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
    Validate manifest syntax, check source URLs (HEAD requests), and verify
    updateFile regex patterns compile successfully.

labctl images list [flags]
    List images stored in e2.

    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key

labctl images prune [flags]
    Remove images from e2 not in manifest. Manual-only (not run automatically).

    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key
    --dry-run                 Show what would be removed

labctl images upload [flags]
    Upload a local file to e2. Used by build workflows to upload built images.
    Computes SHA256 checksum and writes metadata JSON (same format as sync).

    --source PATH             Path to local file to upload (required)
    --destination PATH        Destination path in e2 bucket (required)
    --credentials PATH        Path to SOPS-encrypted credentials file
    --sops-age-key-file PATH  Path to age private key
    --name STRING             Image name for metadata (defaults to destination filename)

    Metadata written to: metadata/<destination>.json
    Example: --destination vyos/vyos-gateway.raw → metadata/vyos/vyos-gateway.raw.json
```

**CLI Output Contract:**

The `sync` command sets GitHub Actions outputs via `$GITHUB_OUTPUT`:
- `files_changed=true|false` — Whether any `updateFile` replacements modified files

Example implementation:
```bash
echo "files_changed=true" >> "$GITHUB_OUTPUT"
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
│  5. VYOS BUILD WORKFLOW (vyos-build.yml)                                    │
│     └─> Triggered by changes to vyos-build/ or configs/                     │
│         ├─> Run vyos-build in Docker container                              │
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
// For sync (HTTP sources)
{
  "name": "talos-1.9.1",
  "checksum": "sha256:abc123...",
  "size": 1234567890,
  "uploadedAt": "2024-12-20T10:00:00Z",
  "source": {
    "url": "https://factory.talos.dev/..."
  }
}

// For upload (local files, e.g., vyos-build output)
{
  "name": "vyos-gateway",
  "checksum": "sha256:def456...",
  "size": 8589934592,
  "uploadedAt": "2024-12-20T12:00:00Z",
  "source": {
    "type": "local",
    "path": "/tmp/vyos-gateway.raw"
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
│   │   └── vyos-gateway.raw                      # Built by vyos-build
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
  pull_request:
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
  group: images-sync-${{ github.ref }}
  cancel-in-progress: false

permissions:
  contents: write
  pull-requests: write

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

      # Skip SOPS on PRs (dry-run doesn't need credentials)
      - name: Write SOPS age key
        if: github.event_name != 'pull_request'
        run: |
          echo "${{ secrets.SOPS_AGE_KEY }}" > /tmp/age-key.txt
          chmod 600 /tmp/age-key.txt

      # PR: validate manifest only (no credentials needed)
      - name: Validate Manifest (PR)
        if: github.event_name == 'pull_request'
        run: ./labctl images validate

      # Push/dispatch: full sync with credentials
      - name: Sync Images
        if: github.event_name != 'pull_request'
        id: sync
        run: |
          FLAGS=""
          if [ "${{ inputs.force }}" == "true" ]; then FLAGS="--force"; fi

          ./labctl images sync \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt \
            $FLAGS

      - name: Create PR if files changed
        if: github.event_name == 'push' && steps.sync.outputs.files_changed == 'true'
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
        if: github.event_name != 'pull_request' && inputs.prune == true
        run: |
          ./labctl images prune \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt
```

### 8.2 VyOS Build (vyos-build.yml)

```yaml
name: Build VyOS Image

on:
  push:
    branches: [master]
    paths:
      - 'infrastructure/network/vyos/vyos-build/**'
      - 'infrastructure/network/vyos/configs/gateway.conf'
  pull_request:
    paths:
      - 'infrastructure/network/vyos/vyos-build/**'
      - 'infrastructure/network/vyos/configs/gateway.conf'
  workflow_dispatch:
    inputs:
      upload:
        description: 'Upload image to e2 storage'
        type: boolean
        default: true

concurrency:
  group: vyos-build-${{ github.ref }}
  cancel-in-progress: false

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Validate flavor template
        run: |
          TEMPLATE="infrastructure/network/vyos/vyos-build/build-flavors/gateway.toml"
          if [[ ! -f "${TEMPLATE}" ]]; then
            echo "ERROR: Template file not found"
            exit 1
          fi
          if ! grep -q '%%SSH_KEY_TYPE%%' "${TEMPLATE}"; then
            echo "ERROR: Template missing %%SSH_KEY_TYPE%% placeholder"
            exit 1
          fi
          if ! grep -q '%%SSH_PUBLIC_KEY%%' "${TEMPLATE}"; then
            echo "ERROR: Template missing %%SSH_PUBLIC_KEY%% placeholder"
            exit 1
          fi

  build:
    if: github.event_name == 'push' || github.event_name == 'workflow_dispatch'
    runs-on: ubuntu-latest
    needs: validate
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Build labctl
        run: go build -o labctl ./tools/labctl

      - name: Install SOPS
        run: |
          curl -LO https://github.com/getsops/sops/releases/download/v3.9.2/sops-v3.9.2.linux.amd64
          chmod +x sops-v3.9.2.linux.amd64
          sudo mv sops-v3.9.2.linux.amd64 /usr/local/bin/sops

      - name: Write SOPS age key
        run: |
          echo "${{ secrets.SOPS_AGE_KEY }}" > /tmp/age-key.txt
          chmod 600 /tmp/age-key.txt

      - name: Extract SSH public key
        env:
          SOPS_AGE_KEY_FILE: /tmp/age-key.txt
        run: |
          sops --decrypt \
            --extract '["ssh_public_key"]' images/packer-ssh.sops.yaml > /tmp/ssh_key.pub

      - name: Clone vyos-build
        run: |
          git clone -b current --single-branch --depth 1 \
            https://github.com/vyos/vyos-build.git /tmp/vyos-build

      - name: Generate build flavor
        run: |
          ./infrastructure/network/vyos/vyos-build/scripts/generate-flavor.sh \
            "$(cat /tmp/ssh_key.pub)" \
            /tmp/vyos-build/data/build-flavors/gateway.toml

      - name: Build VyOS image
        run: |
          VERSION="lab-$(date +%Y%m%d%H%M%S)"
          docker run --rm --privileged \
            -v /tmp/vyos-build:/vyos \
            -v /dev:/dev \
            -w /vyos \
            vyos/vyos-build:current \
            bash -c "sudo ./build-vyos-image --architecture amd64 --build-by ci@lab.gilman.io --build-type release --version ${VERSION} gateway"

          RAW_FILE=$(find /tmp/vyos-build -maxdepth 1 -name "*.raw" -type f 2>/dev/null | head -1)
          cp "${RAW_FILE}" /tmp/vyos-gateway.raw

      - name: Upload to e2
        if: github.event_name == 'push' || (github.event_name == 'workflow_dispatch' && inputs.upload)
        run: |
          ./labctl images upload \
            --credentials images/e2.sops.yaml \
            --sops-age-key-file /tmp/age-key.txt \
            --source /tmp/vyos-gateway.raw \
            --destination vyos/vyos-gateway.raw
```

**VyOS Build Process:** The workflow uses the official `vyos/vyos-build` Docker container
with build flavors. The `gateway.toml` flavor embeds the VyOS configuration directly into
the image, with SSH credentials injected via placeholder replacement.

### 8.3 Mergify Configuration (.mergify.yml)

```yaml
pull_request_rules:
  - name: Auto-merge automated image updates
    conditions:
      - author=github-actions[bot]
      - label=automated
      - base=main
      - "#approved-reviews-by>=0"  # No approval required for bot PRs
      - "check-success=Build VyOS Image / validate"
    actions:
      merge:
        method: squash
        commit_message_template: |
          {{ title }}

          {{ body }}
```

**Check Name Format:** `Workflow Name / Job Name`

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

### SOPS-Encrypted SSH Keypair

Used by VyOS builds for image provisioning. The public key is baked into the image; the private key is stored for future use (e.g., post-build testing).

```bash
# Generate keypair (filename kept as packer-ssh for compatibility)
ssh-keygen -t ed25519 -f packer-ssh -N "" -C "vyos-ci"

# Create SOPS file
cat > images/packer-ssh.sops.yaml << 'EOF'
ssh_public_key: "ssh-ed25519 AAAA... vyos-ci"
ssh_private_key: |
  -----BEGIN OPENSSH PRIVATE KEY-----
  ...
  -----END OPENSSH PRIVATE KEY-----
EOF

# Encrypt
sops --encrypt --in-place images/packer-ssh.sops.yaml
```

```yaml
# images/packer-ssh.sops.yaml (encrypted)
ssh_public_key: ENC[AES256_GCM,data:...,type:str]
ssh_private_key: ENC[AES256_GCM,data:...,type:str]  # Optional: for future use
sops:
    age:
        - recipient: age1...  # CI key
    pgp:
        - XXXX...             # Yubikey
    encrypted_regex: ^(ssh_public_key|ssh_private_key)$
```

**Current Usage:** Only `ssh_public_key` is extracted during build. The private key is retained for potential future automation (e.g., post-build smoke tests).

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

1. **Image Retention:** Keep all versions until explicit prune, or auto-expire?

## 12. Future Considerations

- Image signing (Sigstore/GPG)
- Multi-architecture support (arm64)
- Slack/Discord notifications on failures
