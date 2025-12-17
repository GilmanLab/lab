#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = [
#   "ruamel.yaml",
# ]
# ///
"""
Check if a version exists in a provisioning manifest.

Usage: check-version-exists.py <system> <version>

Exit codes:
  0 - Version does NOT exist (proceed with update)
  1 - Version EXISTS (skip update)
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

    yaml = YAML()
    manifest = yaml.load(manifest_path)

    versions = manifest.get("versions", [])
    if version in versions:
        print(f"Version {version} already tracked")
        return 1  # Exists - condition fails, skip update

    print(f"Version {version} not tracked, proceeding")
    return 0  # Does not exist - condition passes, proceed


if __name__ == "__main__":
    sys.exit(main())
