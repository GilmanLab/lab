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
import sys
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
                for chunk in resp.iter_bytes(chunk_size=8192):
                    f.write(chunk)
                    downloaded += len(chunk)
                    if total:
                        pct = (downloaded / total) * 100
                        print(f"\r  {downloaded:,} / {total:,} bytes ({pct:.1f}%)", end="")

    print()  # newline after progress


def verify_sha256(file_path: Path, expected_hash: str) -> bool:
    """Verify SHA256 checksum of a file."""
    sha256 = hashlib.sha256()
    with open(file_path, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256.update(chunk)
    return sha256.hexdigest() == expected_hash


def download_talos(version: str, manifest: dict, download_dir: Path) -> None:
    """Download Talos Linux ISO and verify checksum."""
    owner = manifest["source"]["github"]["owner"]
    repo = manifest["source"]["github"]["repo"]
    asset_pattern = manifest["source"].get("asset_pattern", "metal-amd64.iso")

    print(f"Fetching release info for {owner}/{repo} {version}...")
    assets = get_release_assets(owner, repo, version)

    # Download ISO
    if asset_pattern not in assets:
        raise ValueError(f"Asset '{asset_pattern}' not found in release {version}")

    iso_path = download_dir / asset_pattern
    download_file(assets[asset_pattern], iso_path, asset_pattern)

    # Download and parse checksums
    if "sha256sum.txt" not in assets:
        raise ValueError("sha256sum.txt not found in release")

    checksum_path = download_dir / "sha256sum.txt"
    download_file(assets["sha256sum.txt"], checksum_path, "sha256sum.txt")

    # Extract expected hash for our asset
    expected_hash = None
    for line in checksum_path.read_text().splitlines():
        if line.endswith(f"  {asset_pattern}"):
            expected_hash = line.split()[0]
            break

    if not expected_hash:
        raise ValueError(f"Checksum for {asset_pattern} not found in sha256sum.txt")

    # Verify
    print(f"Verifying checksum for {asset_pattern}...")
    if not verify_sha256(iso_path, expected_hash):
        raise ValueError("Checksum verification failed!")

    print("Checksum verified successfully")

    # Write individual checksum file for upload
    individual_checksum = download_dir / f"{asset_pattern}.sha256"
    individual_checksum.write_text(f"{expected_hash}  {asset_pattern}\n")


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
        match system:
            case "talos":
                download_talos(version, manifest, download_dir)
            case "vyos":
                print("Error: VyOS download not yet implemented", file=sys.stderr)
                return 1
            case _:
                print(f"Error: Unknown system: {system}", file=sys.stderr)
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
