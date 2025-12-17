#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = [
#   "ruamel.yaml",
# ]
# ///
"""
Prepend a version to a provisioning manifest and trim to max_versions.

Usage: prepend-version.py <system> <version>

Exit codes:
  0 - Success (version added or already existed)
  1 - Error
"""

import sys
from pathlib import Path

from ruamel.yaml import YAML


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <system> <version>", file=sys.stderr)
        return 1

    system = sys.argv[1]
    version = sys.argv[2]

    manifest_path = Path(f"provisioning/{system}/index.yaml")
    if not manifest_path.exists():
        print(f"Error: Manifest not found: {manifest_path}", file=sys.stderr)
        return 1

    # Use ruamel.yaml to preserve formatting and comments
    yaml = YAML()
    yaml.preserve_quotes = True

    manifest = yaml.load(manifest_path)

    # Check if version already exists
    versions = manifest.get("versions", [])
    if version in versions:
        print(f"Version {version} already exists in {manifest_path}, skipping")
        return 0

    # Get max_versions (default to 3)
    max_versions = manifest.get("max_versions", 3)

    # Prepend version and trim
    versions.insert(0, version)
    manifest["versions"] = versions[:max_versions]

    # Write back
    yaml.dump(manifest, manifest_path)

    print(f"Added {version} to {manifest_path} (max: {max_versions})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
