# VyOS Gateway Image Build
# Builds a raw disk image with lab configuration baked in
# Target: VP6630 (Minisforum) - Lab Gateway Router

packer {
  required_plugins {
    qemu = {
      version = ">= 1.1.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

source "qemu" "vyos" {
  iso_url          = var.vyos_iso_url
  iso_checksum     = var.vyos_iso_checksum
  output_directory = var.output_directory
  shutdown_command = "sudo poweroff"
  disk_size        = var.disk_size
  format           = "raw"
  accelerator      = "kvm"
  memory           = var.memory
  cpus             = var.cpus
  net_device       = "virtio-net"
  disk_interface   = "virtio"

  # VyOS boot configuration
  boot_wait = "5s"
  boot_command = [
    # Wait for live system to boot
    "<enter><wait60>",
    # Login as vyos user (default password: vyos)
    "vyos<enter><wait2>",
    "vyos<enter><wait5>",
    # Run automated installation
    "install image<enter><wait2>",
    # Confirm disk selection
    "<enter><wait2>",
    # Confirm partition deletion
    "Yes<enter><wait2>",
    # Accept default root partition size
    "<enter><wait2>",
    # Image name
    "<enter><wait2>",
    # Copy running config
    "<enter><wait2>",
    # Set password for vyos user
    "vyos<enter><wait2>",
    "vyos<enter><wait2>",
    # Installation completes
    "<wait30>",
    # Reboot into installed system
    "reboot<enter><wait60>",
    # Login to installed system
    "vyos<enter><wait2>",
    "vyos<enter><wait5>",
    # Enable SSH for provisioner
    "configure<enter><wait2>",
    "set service ssh port 22<enter><wait2>",
    "set system login user vyos authentication plaintext-password vyos<enter><wait2>",
    "commit<enter><wait5>",
    "save<enter><wait5>",
    "exit<enter><wait2>"
  ]

  # SSH connection for provisioner
  ssh_username     = "vyos"
  ssh_password     = "vyos"
  ssh_timeout      = "30m"
  ssh_port         = 22

  # VM configuration
  vm_name       = "vyos-lab"
  headless      = true

  # QEMU settings
  qemuargs = [
    ["-m", "${var.memory}"],
    ["-smp", "${var.cpus}"]
  ]
}

build {
  name    = "vyos-lab-gateway"
  sources = ["source.qemu.vyos"]

  # Copy gateway configuration
  provisioner "file" {
    source      = "../configs/gateway.conf"
    destination = "/tmp/gateway.conf"
  }

  # Copy provisioning script
  provisioner "file" {
    source      = "scripts/provision.sh"
    destination = "/tmp/provision.sh"
  }

  # Run provisioning script (SSH key is required)
  provisioner "shell" {
    inline = [
      "chmod +x /tmp/provision.sh",
      "sudo /tmp/provision.sh '${var.ssh_key_type}' '${var.ssh_public_key}'"
    ]
  }

  # Final cleanup
  provisioner "shell" {
    inline = [
      # Remove SSH password auth (key-only after provisioning)
      "source /opt/vyatta/etc/functions/script-template",
      "configure",
      "delete system login user vyos authentication plaintext-password",
      "commit",
      "save",
      "exit",
      # Clean up temp files
      "rm -f /tmp/gateway.conf /tmp/provision.sh",
      # Clear command history
      "history -c"
    ]
  }

  # Rename output file to ensure .raw extension
  post-processor "shell-local" {
    inline = [
      "cd ${var.output_directory}",
      "if [ -f 'vyos-lab' ] && [ ! -f 'vyos-lab.raw' ]; then mv vyos-lab vyos-lab.raw; fi",
      "echo 'VyOS image built successfully!'",
      "echo 'Output: ${var.output_directory}/vyos-lab.raw'",
      "echo ''",
      "echo 'To use with Tinkerbell, copy to NAS:'",
      "echo '  scp ${var.output_directory}/vyos-lab.raw nas:/volume1/images/vyos-lab.raw'"
    ]
  }
}
