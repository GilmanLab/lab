# VyOS Packer Variables
# Infrastructure: VP6630 Gateway Router
#
# Network configuration (interfaces, IPs, VLANs) is defined in:
#   ../configs/gateway.conf
#
# If interface names need to change, update gateway.conf directly.

variable "vyos_iso_url" {
  type        = string
  description = "URL to VyOS ISO image"
  default     = "https://github.com/vyos/vyos-rolling-nightly-builds/releases/download/1.5-rolling-202412190007/vyos-1.5-rolling-202412190007-amd64.iso"
}

variable "vyos_iso_checksum" {
  type        = string
  description = "SHA256 checksum of VyOS ISO (format: sha256:HASH)"
  # No default - must be provided via build script or command line
  # Build script calculates: sha256:$(sha256sum vyos.iso | awk '{print $1}')
}

variable "output_directory" {
  type        = string
  description = "Directory for output image"
  default     = "output"
}

variable "disk_size" {
  type        = string
  description = "Disk size for VyOS image"
  default     = "8G"
}

variable "memory" {
  type        = number
  description = "Memory for build VM (MB)"
  default     = 2048
}

variable "cpus" {
  type        = number
  description = "CPUs for build VM"
  default     = 2
}

# SSH Configuration (required)
variable "ssh_key_type" {
  type        = string
  description = "SSH key type (e.g., ssh-rsa, ssh-ed25519, ecdsa-sha2-nistp256)"
  # No default - must be provided via build script

  validation {
    condition     = length(var.ssh_key_type) > 0
    error_message = "SSH key type is required."
  }
}

variable "ssh_public_key" {
  type        = string
  description = "SSH public key body (base64 encoded) for vyos user"
  sensitive   = true
  # No default - must be provided via build script or PKR_VAR_ssh_public_key
  # Build script extracts from: ~/.ssh/id_rsa.pub (or --ssh-key flag)

  validation {
    condition     = length(var.ssh_public_key) > 0
    error_message = "SSH public key is required. Set PKR_VAR_ssh_public_key or use build script with --ssh-key flag."
  }
}
