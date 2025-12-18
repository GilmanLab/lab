#!/usr/bin/env bash
set -euo pipefail

# Provision the bootstrap PXE appliance inside the guest.
#
# Expected inputs (environment variables):
# - BOOTSTRAP_IFACE
# - BOOTSTRAP_HTTP
# - BOOTSTRAP_INTERNAL_MAC
# - BOOTSTRAP_UPLINK_MAC
# - DHCP_RANGE
# - MINIPC_IP
# - MINIPC_MAC
# - TALOS_VERSION
# - VM_IP
# - VM_PREFIX
#
# Expected files already present (uploaded by Packer):
# - /tmp/vmlinuz
# - /tmp/initramfs.xz
# - /tmp/minipc.yaml
# - /tmp/talosconfig
# - /tmp/dnsmasq.conf.tpl
# - /tmp/boot.ipxe.tpl
# - /tmp/nginx-bootstrap.conf
# - /tmp/extract-talos-netboot.sh

require_env() {
  local name="$1"
  if [ -z "${!name:-}" ]; then
    echo "Missing required environment variable: ${name}" >&2
    exit 1
  fi
}

require_env BOOTSTRAP_IFACE
require_env BOOTSTRAP_HTTP
require_env BOOTSTRAP_INTERNAL_MAC
require_env BOOTSTRAP_UPLINK_MAC
require_env DHCP_RANGE
require_env MINIPC_IP
require_env MINIPC_MAC
require_env TALOS_VERSION
require_env VM_IP
require_env VM_PREFIX

echo "Installing packages..."
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y \
  dnsmasq \
  gettext-base \
  ipxe \
  iptables \
  libarchive-tools \
  nginx

echo "Preparing TFTP/HTTP roots..."
mkdir -p /srv/tftp /srv/http/talos/"${TALOS_VERSION}" /srv/http/configs
install -m 0644 /tmp/minipc.yaml /srv/http/configs/minipc.yaml
if [ -f /tmp/talosconfig ]; then
  install -m 0644 /tmp/talosconfig /srv/http/configs/talosconfig
fi

echo "Installing iPXE UEFI binary into TFTP root..."
if [ -f /usr/lib/ipxe/ipxe.efi ]; then
  install -m 0644 /usr/lib/ipxe/ipxe.efi /srv/tftp/ipxe.efi
elif [ -f /usr/lib/ipxe/snponly.efi ]; then
  install -m 0644 /usr/lib/ipxe/snponly.efi /srv/tftp/ipxe.efi
else
  echo "Unable to find ipxe.efi/snponly.efi under /usr/lib/ipxe" >&2
  ls -lah /usr/lib/ipxe || true
  exit 1
fi

echo "Installing Talos netboot artifacts..."
install -m 0644 /tmp/vmlinuz /srv/http/talos/"${TALOS_VERSION}"/vmlinuz
install -m 0644 /tmp/initramfs.xz /srv/http/talos/"${TALOS_VERSION}"/initramfs.xz

echo "Rendering dnsmasq + iPXE configs..."
export BOOTSTRAP_IFACE BOOTSTRAP_HTTP DHCP_RANGE MINIPC_IP MINIPC_MAC TALOS_VERSION VM_IP
envsubst < /tmp/dnsmasq.conf.tpl > /etc/dnsmasq.d/bootstrap.conf
# Do NOT envsubst iPXE scripts: it will clobber iPXE variable expansions like ${base}.
sed \
  -e "s|__BOOTSTRAP_HTTP__|${BOOTSTRAP_HTTP}|g" \
  -e "s|__TALOS_VERSION__|${TALOS_VERSION}|g" \
  /tmp/boot.ipxe.tpl > /srv/http/boot.ipxe
chmod 0644 /srv/http/boot.ipxe

echo "Configuring nginx..."
install -m 0644 /tmp/nginx-bootstrap.conf /etc/nginx/sites-available/bootstrap.conf
rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/bootstrap.conf /etc/nginx/sites-enabled/bootstrap.conf
nginx -t

echo "Configuring static IP (PXE NIC) + optional uplink DHCP for first boot (do not apply during packer build)..."
mkdir -p /etc/netplan
cat > /etc/netplan/01-bootstrap-network.yaml <<EOF
network:
  version: 2
  renderer: networkd
  ethernets:
    pxe:
      match:
        macaddress: "${BOOTSTRAP_INTERNAL_MAC}"
      set-name: "${BOOTSTRAP_IFACE}"
      addresses: ["${VM_IP}/${VM_PREFIX}"]
      optional: true
    uplink:
      match:
        macaddress: "${BOOTSTRAP_UPLINK_MAC}"
      set-name: "eth1"
      dhcp4: true
      dhcp6: false
      optional: true
EOF

echo "Prevent cloud-init from overwriting netplan on first boot..."
mkdir -p /etc/cloud/cloud.cfg.d
echo 'network: {config: disabled}' > /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg

echo "Configuring NAT (PXE -> uplink) if uplink is present..."
install -m 0755 /dev/stdin /usr/local/sbin/bootstrap-nat.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

PXE_IFACE="${BOOTSTRAP_IFACE:-eth0}"

# Determine uplink as the interface that currently has the default route.
# DHCP on the uplink can be a little late; wait briefly so NAT becomes reliable.
UPLINK_IFACE=""
for _ in $(seq 1 60); do
  UPLINK_IFACE="$(ip route show default 2>/dev/null | awk '{print $5}' | head -n1 || true)"
  if [ -n "$UPLINK_IFACE" ]; then
    break
  fi
  sleep 1
done
if [ -z "$UPLINK_IFACE" ]; then
  echo "[bootstrap-nat] no default route after waiting; failing so systemd will retry"
  exit 1
fi
if [ "$UPLINK_IFACE" = "$PXE_IFACE" ]; then
  echo "[bootstrap-nat] default route is on $PXE_IFACE; refusing to NAT"
  exit 0
fi

echo "[bootstrap-nat] enabling ip_forward and NAT: $PXE_IFACE -> $UPLINK_IFACE"
sysctl -w net.ipv4.ip_forward=1 >/dev/null

if ! iptables -t nat -C POSTROUTING -o "$UPLINK_IFACE" -j MASQUERADE 2>/dev/null; then
  iptables -t nat -A POSTROUTING -o "$UPLINK_IFACE" -j MASQUERADE
fi
if ! iptables -C FORWARD -i "$PXE_IFACE" -o "$UPLINK_IFACE" -j ACCEPT 2>/dev/null; then
  iptables -A FORWARD -i "$PXE_IFACE" -o "$UPLINK_IFACE" -j ACCEPT
fi
if ! iptables -C FORWARD -i "$UPLINK_IFACE" -o "$PXE_IFACE" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; then
  iptables -A FORWARD -i "$UPLINK_IFACE" -o "$PXE_IFACE" -m state --state RELATED,ESTABLISHED -j ACCEPT
fi
EOF

cat > /etc/systemd/system/bootstrap-nat.service <<EOF
[Unit]
Description=Bootstrap appliance NAT (PXE network -> uplink)
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
Environment=BOOTSTRAP_IFACE=${BOOTSTRAP_IFACE}
ExecStart=/usr/local/sbin/bootstrap-nat.sh
Restart=on-failure
RestartSec=2
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

echo "Enable services..."
systemctl enable nginx
systemctl enable dnsmasq
systemctl enable bootstrap-nat.service

echo "Smoke-checking rendered configs (non-fatal)..."
head -n 200 /etc/dnsmasq.d/bootstrap.conf || true
head -n 200 /srv/http/boot.ipxe || true
ls -lah /srv/tftp /srv/http/talos/"${TALOS_VERSION}" /srv/http/configs || true

echo "Provisioning complete."


