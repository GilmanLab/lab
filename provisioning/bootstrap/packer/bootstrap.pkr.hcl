packer {
  required_plugins {
    qemu = {
      version = ">= 1.1.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

variable "ubuntu_iso_url" {
  type    = string
  default = "https://releases.ubuntu.com/24.04.3/ubuntu-24.04.3-live-server-amd64.iso"
}

variable "ubuntu_iso_checksum" {
  # Example: "sha256:..."
  type    = string
  # Verified by downloading the ISO and validating against upstream SHA256SUMS.
  default = "sha256:c3514bf0056180d09376462a7a1b4f213c1d6e8ea67fae5c25099c6fd3d8274b"
}

variable "vm_name" {
  type    = string
  default = "bootstrap-pxe"
}

variable "output_dir" {
  type    = string
  # Local build output directory (not committed). build.sh points this to
  # artifacts/bootstrap/<bootstrap_version>/ so it can be uploaded to iDrive e2.
  default = "build/bootstrap/dev"
}

variable "disk_size" {
  type    = string
  default = "20G"
}

variable "cpus" {
  type    = number
  default = 2
}

variable "memory" {
  type    = number
  default = 2048
}

variable "ssh_username" {
  type    = string
  default = "packer"
}

variable "ssh_password" {
  type      = string
  default   = "packer"
  sensitive = true
}

variable "ssh_timeout" {
  type    = string
  default = "30m"
}

variable "vm_ip" {
  # Static IP on the isolated link (applied on first boot after image build)
  type    = string
  default = "192.168.2.1"
}

variable "vm_prefix" {
  # CIDR prefix, e.g. 24 for 192.168.2.0/24
  type    = number
  default = 24
}

variable "bootstrap_iface" {
  # Interface name to bind dnsmasq to inside the VM.
  type    = string
  default = "eth0"
}

variable "bootstrap_internal_mac" {
  # Deterministic MAC for the *isolated/PXE* NIC in runtime environments (libvirt/Synology).
  # This allows netplan to reliably identify the correct interface for the static IP + DHCP/TFTP/HTTP.
  #
  # Note: This MAC is not applied during the Packer build (Packer manages its own NIC); it is used
  # by the guest netplan on first boot after import.
  type    = string
  default = "02:11:32:24:64:5a"
}

variable "bootstrap_uplink_mac" {
  # Deterministic MAC for an optional uplink NIC (DHCP) used to provide outbound internet.
  # The appliance will NAT traffic from the PXE NIC out via this uplink when present.
  type    = string
  default = "02:11:32:24:64:5c"
}

variable "minipc_mac" {
  type = string
  default = "02:11:32:24:64:5b"
}

variable "minipc_ip" {
  type    = string
  default = "192.168.2.2"
}

variable "dhcp_range" {
  # dnsmasq range syntax: start,end
  type    = string
  default = "192.168.2.10,192.168.2.50"
}

variable "talos_version" {
  type = string
  default = "1.11.6"
}

variable "talos_kernel_path" {
  # Path to a local Talos kernel image (e.g. vmlinuz-amd64 from Talos release).
  type = string
}

variable "talos_initramfs_path" {
  # Path to a local Talos initramfs (e.g. initramfs-amd64.xz from Talos release).
  type = string
}

variable "machineconfig_path" {
  # Path to a plaintext Talos machineconfig to embed and serve.
  #
  # Recommended workflow:
  # - Keep SOPS-encrypted node config under provisioning/bootstrap/config/controlplane.yaml
  # - Use provisioning/bootstrap/build.sh to decrypt it into build/bootstrap/controlplane.yaml
  type    = string
  default = "build/bootstrap/controlplane.yaml"
}

variable "talosconfig_path" {
  # Optional convenience: a plaintext Talos client config (talosconfig).
  # build.sh can decrypt provisioning/bootstrap/config/talosconfig into this location.
  # If provided, it will be served over HTTP as /configs/talosconfig.
  type    = string
  default = "build/bootstrap/talosconfig"
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
    source      = "provisioning/bootstrap/scripts/extract-talos-netboot.sh"
    destination = "/tmp/extract-talos-netboot.sh"
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

  # Normalize output location: copy the produced raw disk to output_dir root.
  post-processor "shell-local" {
    inline = [
      "set -euo pipefail",
      "OUT='${var.output_dir}'",
      "mkdir -p \"$OUT\"",
      # qemu builder emits a single raw disk; name varies by plugin version.
      # Find the newest *.raw in the qemu output dir and copy it.
      "RAW_SRC=$(ls -t \"$OUT/qemu\"/*.raw 2>/dev/null | head -n1 || true)",
      "if [ -z \"$RAW_SRC\" ]; then RAW_SRC=$(ls -t \"$OUT/qemu\"/*.img 2>/dev/null | head -n1 || true); fi",
      "if [ -z \"$RAW_SRC\" ] && [ -f \"$OUT/qemu/${var.vm_name}\" ]; then RAW_SRC=\"$OUT/qemu/${var.vm_name}\"; fi",
      "if [ -z \"$RAW_SRC\" ]; then echo 'Unable to find built raw image under output dir' >&2; ls -lah \"$OUT/qemu\" >&2; exit 1; fi",
      "cp -f \"$RAW_SRC\" \"$OUT/${var.vm_name}.raw\"",
      "echo \"Wrote: $OUT/${var.vm_name}.raw\"",
    ]
  }
}


