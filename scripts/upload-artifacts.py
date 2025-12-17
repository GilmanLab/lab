#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.12"
# dependencies = [
#   "boto3",
# ]
# ///
"""
Upload artifacts to iDrive e2 (S3-compatible storage).

Usage: upload-artifacts.py <system> <version>

Environment variables:
  IDRIVE_BUCKET   - S3 bucket name
  IDRIVE_ENDPOINT - S3 endpoint URL (e.g., s3.us-west-1.idrivee2.com)
  IDRIVE_REGION   - AWS region (e.g., us-west-1)
  AWS_ACCESS_KEY_ID / IDRIVE_ACCESS_KEY - Access key
  AWS_SECRET_ACCESS_KEY / IDRIVE_SECRET_KEY - Secret key
"""

import os
import sys
from pathlib import Path

import boto3
from botocore.config import Config


def get_env(name: str, fallback: str | None = None) -> str:
    """Get environment variable with optional fallback name."""
    value = os.environ.get(name)
    if not value and fallback:
        value = os.environ.get(fallback)
    if not value:
        raise EnvironmentError(f"Environment variable {name} (or {fallback}) is required")
    return value


def create_s3_client():
    """Create S3 client configured for iDrive e2."""
    endpoint = get_env("IDRIVE_ENDPOINT")
    region = get_env("IDRIVE_REGION", "AWS_DEFAULT_REGION")

    # Support both AWS standard and IDRIVE prefixed env vars
    access_key = os.environ.get("AWS_ACCESS_KEY_ID") or os.environ.get("IDRIVE_ACCESS_KEY")
    secret_key = os.environ.get("AWS_SECRET_ACCESS_KEY") or os.environ.get("IDRIVE_SECRET_KEY")

    if not access_key or not secret_key:
        raise EnvironmentError("AWS_ACCESS_KEY_ID/IDRIVE_ACCESS_KEY and AWS_SECRET_ACCESS_KEY/IDRIVE_SECRET_KEY required")

    return boto3.client(
        "s3",
        endpoint_url=f"https://{endpoint}",
        region_name=region,
        aws_access_key_id=access_key,
        aws_secret_access_key=secret_key,
        config=Config(signature_version="s3v4"),
    )


def upload_file(s3_client, bucket: str, local_path: Path, s3_key: str) -> None:
    """Upload a file to S3 with progress."""
    file_size = local_path.stat().st_size
    uploaded = 0

    def progress_callback(bytes_transferred):
        nonlocal uploaded
        uploaded += bytes_transferred
        pct = (uploaded / file_size) * 100
        print(f"\r  Uploading {local_path.name}: {uploaded:,} / {file_size:,} bytes ({pct:.1f}%)", end="")

    s3_client.upload_file(
        str(local_path),
        bucket,
        s3_key,
        Callback=progress_callback,
    )
    print()  # newline after progress


def main() -> int:
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <system> <version>", file=sys.stderr)
        return 1

    system = sys.argv[1]
    version = sys.argv[2]

    try:
        bucket = get_env("IDRIVE_BUCKET")
    except EnvironmentError as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    download_dir = Path(f"artifacts/{system}/{version}")
    if not download_dir.exists():
        print(f"Error: Artifacts directory not found: {download_dir}", file=sys.stderr)
        return 1

    try:
        s3 = create_s3_client()
    except EnvironmentError as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    s3_prefix = f"{system}/{version}"
    print(f"Uploading artifacts to s3://{bucket}/{s3_prefix}/")

    # Upload all files in the artifacts directory
    files = sorted(download_dir.iterdir())
    for local_path in files:
        if local_path.is_file():
            s3_key = f"{s3_prefix}/{local_path.name}"
            upload_file(s3, bucket, local_path, s3_key)

    # List uploaded files
    print(f"\nVerifying uploads in s3://{bucket}/{s3_prefix}/")
    response = s3.list_objects_v2(Bucket=bucket, Prefix=s3_prefix)

    if "Contents" in response:
        for obj in response["Contents"]:
            print(f"  {obj['Key']} ({obj['Size']:,} bytes)")
    else:
        print("  Warning: No objects found after upload", file=sys.stderr)

    print(f"\nSuccessfully uploaded {system} {version}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
