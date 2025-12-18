packer {
  required_plugins {
    qemu = {
      version = ">= 1.1.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

# ─────────────────────────────────────────────────────────────────────────────
# Base Image
# ─────────────────────────────────────────────────────────────────────────────

variable "ubuntu_iso_url" {
  description = "Ubuntu Server ISO URL"
  type        = string
  default     = "https://releases.ubuntu.com/24.04.3/ubuntu-24.04.3-live-server-amd64.iso"
}

variable "ubuntu_iso_checksum" {
  description = "SHA256 checksum of the Ubuntu ISO (verified against upstream SHA256SUMS)"
  type        = string
  default     = "sha256:c3514bf0056180d09376462a7a1b4f213c1d6e8ea67fae5c25099c6fd3d8274b"
}

# ─────────────────────────────────────────────────────────────────────────────
# VM Resources
# ─────────────────────────────────────────────────────────────────────────────

variable "vm_name" {
  description = "Name of the output VM image"
  type        = string
  default     = "bootstrap-pxe"
}

variable "output_dir" {
  description = "Local build output directory (build.sh sets this to artifacts/bootstrap/<version>/)"
  type        = string
  default     = "build/bootstrap/dev"
}

variable "disk_size" {
  description = "VM disk size"
  type        = string
  default     = "20G"
}

variable "cpus" {
  description = "Number of vCPUs"
  type        = number
  default     = 2
}

variable "memory" {
  description = "Memory in MB"
  type        = number
  default     = 2048
}

# ─────────────────────────────────────────────────────────────────────────────
# SSH (Packer build-time only)
# ─────────────────────────────────────────────────────────────────────────────

variable "ssh_username" {
  description = "SSH username for Packer provisioning"
  type        = string
  default     = "packer"
}

variable "ssh_password" {
  description = "SSH password for Packer provisioning"
  type        = string
  default     = "packer"
  sensitive   = true
}

variable "ssh_timeout" {
  description = "SSH connection timeout"
  type        = string
  default     = "30m"
}

# ─────────────────────────────────────────────────────────────────────────────
# Network: Bootstrap Appliance
# ─────────────────────────────────────────────────────────────────────────────

variable "vm_ip" {
  description = "Static IP for the bootstrap appliance on the isolated PXE network"
  type        = string
  default     = "192.168.2.1"
}

variable "vm_prefix" {
  description = "CIDR prefix length (e.g. 24 for /24)"
  type        = number
  default     = 24
}

variable "bootstrap_iface" {
  description = "Interface name for dnsmasq binding inside the VM"
  type        = string
  default     = "eth0"
}

variable "bootstrap_internal_mac" {
  description = "Deterministic MAC for the isolated/PXE NIC (used by netplan on first boot)"
  type        = string
  default     = "02:11:32:24:64:5a"
}

variable "bootstrap_uplink_mac" {
  description = "Deterministic MAC for the optional uplink NIC (NAT to internet)"
  type        = string
  default     = "02:11:32:24:64:5c"
}

variable "dhcp_range" {
  description = "dnsmasq DHCP range (start,end) - only used for non-target clients"
  type        = string
  default     = "192.168.2.10,192.168.2.50"
}

# ─────────────────────────────────────────────────────────────────────────────
# Network: Target Mini-PC (PXE Client)
# ─────────────────────────────────────────────────────────────────────────────

variable "minipc_mac" {
  description = "MAC address of the target mini-PC (DHCP is pinned to this MAC only)"
  type        = string
  default     = "02:11:32:24:64:5b"
}

variable "minipc_ip" {
  description = "IP address to assign to the mini-PC via DHCP"
  type        = string
  default     = "192.168.2.2"
}

# ─────────────────────────────────────────────────────────────────────────────
# Talos Configuration
# ─────────────────────────────────────────────────────────────────────────────

variable "talos_version" {
  description = "Talos version (without 'v' prefix)"
  type        = string
  default     = "1.11.6"
}

variable "talos_kernel_path" {
  description = "Path to Talos kernel (vmlinuz-amd64)"
  type        = string
}

variable "talos_initramfs_path" {
  description = "Path to Talos initramfs (initramfs-amd64.xz)"
  type        = string
}

variable "machineconfig_path" {
  description = "Path to plaintext Talos machineconfig (decrypted by build.sh)"
  type        = string
  default     = "build/bootstrap/controlplane.yaml"
}

variable "talosconfig_path" {
  description = "Path to plaintext talosconfig (optional, served at /configs/talosconfig)"
  type        = string
  default     = "build/bootstrap/talosconfig"
}

locals {
  bootstrap_http = "http://${var.vm_ip}"
}

source "qemu" "bootstrap" {
  vm_name          = var.vm_name
  output_directory = "${var.output_dir}/qemu"

  # Produce a RAW disk image for Synology VMM import
  format    = "raw"
  disk_size = var.disk_size

  iso_url      = var.ubuntu_iso_url
  iso_checksum = var.ubuntu_iso_checksum

  accelerator  = "kvm"
  machine_type = "q35"

  # UEFI (OVMF)
  qemuargs = [
    ["-cpu", "host"],
    ["-bios", "/usr/share/OVMF/OVMF_CODE.fd"],
  ]

  cpus   = var.cpus
  memory = var.memory

  headless  = true
  boot_wait = "5s"

  http_directory = "provisioning/bootstrap/packer/http"

  # Ubuntu autoinstall via nocloud-net
  boot_command = [
    "<wait5>",
    "c",
    "<wait>",
    "linux /casper/vmlinuz --- autoinstall ds=nocloud-net\\;s=http://{{ .HTTPIP }}:{{ .HTTPPort }}/ ",
    "<enter><wait>",
    "initrd /casper/initrd",
    "<enter><wait>",
    "boot",
    "<enter>"
  ]

  ssh_username = var.ssh_username
  ssh_password = var.ssh_password
  ssh_timeout  = var.ssh_timeout

  shutdown_command = "echo '${var.ssh_password}' | sudo -S poweroff"
}

build {
  sources = ["source.qemu.bootstrap"]

  # Copy Talos netboot artifacts + configs into the VM
  provisioner "file" {
    source      = var.talos_kernel_path
    destination = "/tmp/vmlinuz"
  }

  provisioner "file" {
    source      = var.talos_initramfs_path
    destination = "/tmp/initramfs.xz"
  }

  provisioner "file" {
    source      = var.machineconfig_path
    destination = "/tmp/minipc.yaml"
  }

  provisioner "file" {
    source      = var.talosconfig_path
    destination = "/tmp/talosconfig"
  }

  provisioner "file" {
    source      = "provisioning/bootstrap/files/dnsmasq.conf"
    destination = "/tmp/dnsmasq.conf.tpl"
  }

  provisioner "file" {
    source      = "provisioning/bootstrap/files/boot.ipxe"
    destination = "/tmp/boot.ipxe.tpl"
  }

  provisioner "file" {
    source      = "provisioning/bootstrap/files/nginx-bootstrap.conf"
    destination = "/tmp/nginx-bootstrap.conf"
  }

  provisioner "file" {
    source      = "provisioning/bootstrap/files/bootstrap-nat.sh"
    destination = "/tmp/bootstrap-nat.sh"
  }

  provisioner "file" {
    source      = "provisioning/bootstrap/files/bootstrap-nat.service"
    destination = "/tmp/bootstrap-nat.service"
  }

  provisioner "file" {
    source      = "provisioning/bootstrap/packer/scripts/provision-bootstrap.sh"
    destination = "/tmp/provision-bootstrap.sh"
  }

  provisioner "shell" {
    environment_vars = [
      "BOOTSTRAP_IFACE=${var.bootstrap_iface}",
      "BOOTSTRAP_HTTP=${local.bootstrap_http}",
      "BOOTSTRAP_INTERNAL_MAC=${var.bootstrap_internal_mac}",
      "BOOTSTRAP_UPLINK_MAC=${var.bootstrap_uplink_mac}",
      "DHCP_RANGE=${var.dhcp_range}",
      "MINIPC_IP=${var.minipc_ip}",
      "MINIPC_MAC=${var.minipc_mac}",
      "TALOS_VERSION=${var.talos_version}",
      "VM_IP=${var.vm_ip}",
      "VM_PREFIX=${var.vm_prefix}",
    ]
    inline = [
      "chmod +x /tmp/provision-bootstrap.sh",
      "sudo -E /tmp/provision-bootstrap.sh",
    ]
  }

  # Copy the produced raw disk to output_dir root with a known name.
  post-processor "shell-local" {
    inline = [
      "cp -f '${var.output_dir}/qemu/${var.vm_name}' '${var.output_dir}/${var.vm_name}.raw'",
      "echo 'Wrote: ${var.output_dir}/${var.vm_name}.raw'",
    ]
  }
}


