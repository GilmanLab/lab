#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = [
#   "httpx",
#   "ruamel.yaml",
# ]
# ///
"""
Download ISO artifacts for a provisioning system.

Usage: download-iso.py <system> <version>
"""

import hashlib
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import httpx
from ruamel.yaml import YAML

GITHUB_API = "https://api.github.com"


def get_manifest(system: str) -> dict:
    """Load the provisioning manifest for a system."""
    manifest_path = Path(f"provisioning/{system}/index.yaml")
    if not manifest_path.exists():
        raise FileNotFoundError(f"Manifest not found: {manifest_path}")

    yaml = YAML()
    return yaml.load(manifest_path)


def get_release_assets(owner: str, repo: str, version: str) -> dict[str, str]:
    """Get download URLs for release assets."""
    url = f"{GITHUB_API}/repos/{owner}/{repo}/releases/tags/{version}"

    with httpx.Client() as client:
        resp = client.get(url)
        resp.raise_for_status()
        release = resp.json()

    return {asset["name"]: asset["browser_download_url"] for asset in release["assets"]}


def download_file(url: str, dest: Path, desc: str = "") -> None:
    """Download a file with progress indication."""
    print(f"Downloading {desc or url}...")

    with httpx.Client(follow_redirects=True) as client:
        with client.stream("GET", url) as resp:
            resp.raise_for_status()
            total = int(resp.headers.get("content-length", 0))

            with open(dest, "wb") as f:
                downloaded = 0
                last_pct_logged = -10  # Track last logged percentage
                for chunk in resp.iter_bytes(chunk_size=8192):
                    f.write(chunk)
                    downloaded += len(chunk)
                    if total:
                        pct = (downloaded / total) * 100
                        # Only log every 10%
                        if pct >= last_pct_logged + 10:
                            print(f"  {pct:.0f}% ({downloaded:,} / {total:,} bytes)")
                            last_pct_logged = int(pct // 10) * 10

    print(f"  Downloaded {dest.name}")


def verify_sha256(file_path: Path, expected_hash: str) -> bool:
    """Verify SHA256 checksum of a file."""
    sha256 = hashlib.sha256()
    with open(file_path, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256.update(chunk)
    return sha256.hexdigest() == expected_hash


def verify_cosign(file_path: Path, bundle_path: Path, identity_regexp: str, oidc_issuer: str) -> bool:
    """Verify a file using cosign with a Sigstore bundle."""
    if not shutil.which("cosign"):
        print("  Warning: cosign not found, skipping signature verification")
        return True  # Don't fail if cosign isn't available

    cmd = [
        "cosign", "verify-blob",
        "--bundle", str(bundle_path),
        "--certificate-identity-regexp", identity_regexp,
        "--certificate-oidc-issuer", oidc_issuer,
        str(file_path),
    ]

    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"  cosign verification failed: {result.stderr}", file=sys.stderr)
        return False

    return True


def verify_minisig(file_path: Path, sig_path: Path, public_key: str) -> bool:
    """Verify a file using minisign."""
    if not shutil.which("minisign"):
        print("  Warning: minisign not found, skipping signature verification")
        return True  # Don't fail if minisign isn't available

    with tempfile.NamedTemporaryFile(mode="w", suffix=".pub", delete=False) as f:
        f.write(public_key)
        pubkey_path = f.name

    try:
        cmd = ["minisign", "-Vm", str(file_path), "-p", pubkey_path]
        result = subprocess.run(cmd, capture_output=True, text=True)
        if result.returncode != 0:
            print(f"  minisign verification failed: {result.stderr}", file=sys.stderr)
            return False
        return True
    finally:
        Path(pubkey_path).unlink()


def download_github_release(version: str, manifest: dict, download_dir: Path) -> None:
    """Download and verify a GitHub release asset based on manifest config."""
    github = manifest["source"]["github"]
    owner, repo = github["owner"], github["repo"]
    asset_pattern = manifest["source"]["asset_pattern"]
    verification = manifest.get("verification", {})

    print(f"Fetching release info for {owner}/{repo} {version}...")
    assets = get_release_assets(owner, repo, version)

    # Download primary asset
    if asset_pattern not in assets:
        raise ValueError(f"Asset '{asset_pattern}' not found in release {version}")

    iso_path = download_dir / asset_pattern
    download_file(assets[asset_pattern], iso_path, asset_pattern)

    # Checksum verification (config-driven)
    checksum_cfg = verification.get("checksum", {})
    checksum_type = checksum_cfg.get("type", "none")
    expected_hash = None

    match checksum_type:
        case "combined_file":
            filename = checksum_cfg.get("filename", "sha256sum.txt")
            if filename not in assets:
                raise ValueError(f"{filename} not found in release")

            checksum_path = download_dir / filename
            download_file(assets[filename], checksum_path, filename)

            # Parse combined checksum file
            for line in checksum_path.read_text().splitlines():
                if line.endswith(f"  {asset_pattern}"):
                    expected_hash = line.split()[0]
                    break

            if not expected_hash:
                raise ValueError(f"Checksum for {asset_pattern} not found in {filename}")

            print("Verifying SHA256 checksum...")
            if not verify_sha256(iso_path, expected_hash):
                raise ValueError("SHA256 checksum verification failed!")
            print("  SHA256 checksum verified")

        case "per_file":
            checksum_name = f"{asset_pattern}.sha256"
            if checksum_name not in assets:
                raise ValueError(f"{checksum_name} not found in release")

            checksum_path = download_dir / checksum_name
            download_file(assets[checksum_name], checksum_path, checksum_name)

            # Per-file format: just the hash, or "hash  filename"
            content = checksum_path.read_text().strip()
            expected_hash = content.split()[0]

            print("Verifying SHA256 checksum...")
            if not verify_sha256(iso_path, expected_hash):
                raise ValueError("SHA256 checksum verification failed!")
            print("  SHA256 checksum verified")

        case "none":
            print("  Skipping checksum verification (not configured)")

    # Signature verification (config-driven)
    sig_cfg = verification.get("signature", {})
    sig_type = sig_cfg.get("type", "none")

    match sig_type:
        case "cosign":
            bundle_name = f"{asset_pattern}.bundle"
            bundle_path = download_dir / bundle_name
            if bundle_name in assets:
                download_file(assets[bundle_name], bundle_path, bundle_name)
                print("Verifying cosign signature...")
                if not verify_cosign(
                    iso_path,
                    bundle_path,
                    identity_regexp=f"https://github.com/{owner}/{repo}/",
                    oidc_issuer="https://token.actions.githubusercontent.com",
                ):
                    raise ValueError("Cosign signature verification failed!")
                print("  Cosign signature verified")
            else:
                print(f"  Warning: {bundle_name} not found, skipping signature verification")

        case "minisig":
            sig_name = f"{asset_pattern}.minisig"
            sig_path = download_dir / sig_name
            public_key = sig_cfg.get("public_key")
            if not public_key:
                raise ValueError("minisig verification requires public_key in manifest")
            if sig_name in assets:
                download_file(assets[sig_name], sig_path, sig_name)
                print("Verifying minisig signature...")
                if not verify_minisig(iso_path, sig_path, public_key):
                    raise ValueError("Minisig signature verification failed!")
                print("  Minisig signature verified")
            else:
                raise ValueError(f"{sig_name} not found in release")

        case "none":
            print("  Skipping signature verification (not configured)")

    # Write individual checksum file for upload (if we have a hash)
    if expected_hash:
        individual_checksum = download_dir / f"{asset_pattern}.sha256"
        individual_checksum.write_text(f"{expected_hash}  {asset_pattern}\n")


def download_direct_url(version: str, manifest: dict, download_dir: Path) -> None:
    """Download from a direct URL template."""
    url_template = manifest["source"]["url_template"]
    asset_pattern = manifest["source"].get("asset_pattern")
    verification = manifest.get("verification", {})

    # Substitute version into URL
    url = url_template.replace("${version}", version)

    # Derive filename from URL or asset_pattern
    if asset_pattern:
        filename = asset_pattern.replace("${version}", version)
    else:
        filename = url.split("/")[-1]

    iso_path = download_dir / filename
    download_file(url, iso_path, filename)

    # Checksum verification
    checksum_cfg = verification.get("checksum", {})
    checksum_type = checksum_cfg.get("type", "none")
    expected_hash = None

    match checksum_type:
        case "per_file":
            checksum_url = f"{url}.sha256"
            checksum_path = download_dir / f"{filename}.sha256"
            download_file(checksum_url, checksum_path, f"{filename}.sha256")

            content = checksum_path.read_text().strip()
            expected_hash = content.split()[0]

            print("Verifying SHA256 checksum...")
            if not verify_sha256(iso_path, expected_hash):
                raise ValueError("SHA256 checksum verification failed!")
            print("  SHA256 checksum verified")

        case "none":
            print("  Skipping checksum verification (not configured)")

    # Signature verification
    sig_cfg = verification.get("signature", {})
    sig_type = sig_cfg.get("type", "none")

    match sig_type:
        case "minisig":
            sig_url = f"{url}.minisig"
            sig_path = download_dir / f"{filename}.minisig"
            public_key = sig_cfg.get("public_key")
            if not public_key:
                raise ValueError("minisig verification requires public_key in manifest")
            download_file(sig_url, sig_path, f"{filename}.minisig")
            print("Verifying minisig signature...")
            if not verify_minisig(iso_path, sig_path, public_key):
                raise ValueError("Minisig signature verification failed!")
            print("  Minisig signature verified")

        case "none":
            print("  Skipping signature verification (not configured)")

    # Write individual checksum file if we computed one
    if expected_hash:
        individual_checksum = download_dir / f"{filename}.sha256"
        individual_checksum.write_text(f"{expected_hash}  {filename}\n")


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <system> <version>", file=sys.stderr)
        return 1

    system = sys.argv[1]
    version = sys.argv[2]

    try:
        manifest = get_manifest(system)
    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    download_dir = Path(f"artifacts/{system}/{version}")
    download_dir.mkdir(parents=True, exist_ok=True)

    try:
        source_type = manifest.get("source", {}).get("type", "github_release")
        match source_type:
            case "github_release":
                download_github_release(version, manifest, download_dir)
            case "direct_url":
                download_direct_url(version, manifest, download_dir)
            case _:
                print(f"Error: Unknown source type: {source_type}", file=sys.stderr)
                return 1
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    print(f"\nDownloaded artifacts to {download_dir}:")
    for f in sorted(download_dir.iterdir()):
        print(f"  {f.name} ({f.stat().st_size:,} bytes)")

    return 0


if __name__ == "__main__":
    sys.exit(main())
