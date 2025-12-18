#!/usr/bin/env bash
# Bootstrap appliance NAT script: routes traffic from PXE network to uplink.
#
# This script is run at boot by bootstrap-nat.service.
# It enables IP forwarding and sets up iptables MASQUERADE rules.
set -euo pipefail

PXE_IFACE="${BOOTSTRAP_IFACE:-eth0}"

# Determine uplink as the interface with the default route.
# DHCP on the uplink can be slow; wait briefly so NAT becomes reliable.
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

