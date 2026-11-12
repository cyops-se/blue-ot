#!/usr/bin/env bash
#
# install.sh — create a Proxmox LXC for one blue-ot simulator + blue-proxy.
#
# Run this ON THE PROXMOX HOST (as root). It creates a Debian LXC, lays the
# rootfs bundle built by build-bundle.sh over it, installs the OT tools and
# starts the simulator and blue-proxy as systemd services. Inside the container
# blue-proxy owns Modbus TCP :1502 and forwards to the simulator on :5020.
#
#   ./install.sh --bundle blueot-modsim.tar.gz
#   ./install.sh --bundle blueot-blue-chp.tar.gz --ctid 210 \
#       --storage local-lvm --bridge vmbr0 --ip 192.168.10.42/24 --gw 192.168.10.1
#
# Defaults: next free CTID, privileged container, DHCP, 1 core / 512 MB / 4 GB,
# Debian 12 template on 'local', rootfs on 'local-lvm', cloud-init provisioned
# from the NoCloud seed (disable with --no-cloud-init).

set -euo pipefail

# ---- defaults ----
CTID=""
BUNDLE=""
SIM=""
HOSTNAME=""
STORAGE="local-lvm"          # rootfs storage
TPL_STORAGE="local"          # where the OS template lives
TEMPLATE="debian-12-standard" # substring or full volid (local:vztmpl/...)
BRIDGE="vmbr0"
IP="dhcp"                    # or CIDR, e.g. 192.168.1.50/24
GW=""
CORES=1
MEMORY=512
SWAP=256
DISK=4                       # GB
UNPRIV=0                     # 0 = privileged (tools need caps), 1 = unprivileged
ONBOOT=1
CLOUDINIT=1                  # install + provision cloud-init from the NoCloud seed

die() { echo "error: $*" >&2; exit 1; }

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
  cat <<'EOF'

Options:
  --bundle PATH         rootfs bundle (blueot-<sim>.tar.gz); required
  --sim NAME            simulator name (default: derived from bundle filename)
  --ctid N              container id (default: next free)
  --hostname NAME       default: blueot-<sim>
  --storage NAME        rootfs storage (default: local-lvm)
  --template-storage N  template storage (default: local)
  --template NAME|VOLID Debian template, substring or full volid (default: debian-12-standard)
  --bridge NAME         default: vmbr0
  --ip dhcp|CIDR        default: dhcp
  --gw IP               gateway (for static --ip)
  --cores N             default: 1
  --memory MB           default: 512
  --swap MB             default: 256
  --disk GB             default: 4
  --unprivileged        create an unprivileged container (default: privileged)
  --no-onboot           do not start the container at host boot
  --no-cloud-init       skip cloud-init (default: installed & provisioned from the NoCloud seed)
  -h, --help
EOF
}

# ---- parse args ----
while [ $# -gt 0 ]; do
  case "$1" in
    --bundle)           BUNDLE="$2"; shift 2 ;;
    --sim)              SIM="$2"; shift 2 ;;
    --ctid)             CTID="$2"; shift 2 ;;
    --hostname)         HOSTNAME="$2"; shift 2 ;;
    --storage)          STORAGE="$2"; shift 2 ;;
    --template-storage) TPL_STORAGE="$2"; shift 2 ;;
    --template)         TEMPLATE="$2"; shift 2 ;;
    --bridge)           BRIDGE="$2"; shift 2 ;;
    --ip)               IP="$2"; shift 2 ;;
    --gw)               GW="$2"; shift 2 ;;
    --cores)            CORES="$2"; shift 2 ;;
    --memory)           MEMORY="$2"; shift 2 ;;
    --swap)             SWAP="$2"; shift 2 ;;
    --disk)             DISK="$2"; shift 2 ;;
    --unprivileged)     UNPRIV=1; shift ;;
    --no-onboot)        ONBOOT=0; shift ;;
    --cloud-init)       CLOUDINIT=1; shift ;;
    --no-cloud-init)    CLOUDINIT=0; shift ;;
    -h|--help)          usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

command -v pct >/dev/null || die "pct not found — run this on a Proxmox VE host as root"
[ "$(id -u)" = 0 ] || die "must run as root"

# ---- bundle / sim ----
[ -n "$BUNDLE" ] || die "missing --bundle (build one with proxmox/build-bundle.sh)"
[ -f "$BUNDLE" ] || die "bundle not found: $BUNDLE"
if [ -z "$SIM" ]; then
  SIM="$(basename "$BUNDLE")"; SIM="${SIM#blueot-}"; SIM="${SIM%.tar.gz}"
fi
[ -n "$SIM" ] || die "could not determine --sim from bundle name"
[ -n "$HOSTNAME" ] || HOSTNAME="blueot-$SIM"

# ---- ctid ----
if [ -z "$CTID" ]; then
  CTID="$(pvesh get /cluster/nextid 2>/dev/null)" || die "could not get next free CTID"
fi
pct status "$CTID" >/dev/null 2>&1 && die "CTID $CTID already exists"

# ---- resolve the OS template ----
resolve_template() {
  if [[ "$TEMPLATE" == *:vztmpl/* ]]; then echo "$TEMPLATE"; return; fi
  local have
  have="$(pveam list "$TPL_STORAGE" 2>/dev/null | awk 'NR>1{print $1}' | grep -F "$TEMPLATE" | sort -V | tail -1 || true)"
  if [ -z "$have" ]; then
    echo ">> template '$TEMPLATE' not present on $TPL_STORAGE, downloading" >&2
    pveam update >/dev/null 2>&1 || true
    local avail
    avail="$(pveam available --section system | awk '{print $NF}' | grep -F "$TEMPLATE" | sort -V | tail -1 || true)"
    [ -n "$avail" ] || die "no template matching '$TEMPLATE' available"
    pveam download "$TPL_STORAGE" "$avail" >&2
    have="$(pveam list "$TPL_STORAGE" | awk 'NR>1{print $1}' | grep -F "$avail" | tail -1)"
  fi
  [ -n "$have" ] || die "could not resolve template '$TEMPLATE'"
  echo "$have"
}
OSTEMPLATE="$(resolve_template)"
echo ">> template: $OSTEMPLATE"

# ---- create the container ----
NET="name=eth0,bridge=$BRIDGE,ip=$IP"
[ -n "$GW" ] && NET="$NET,gw=$GW"

echo ">> creating CT $CTID ($HOSTNAME), $([ "$UNPRIV" = 1 ] && echo unprivileged || echo privileged)"
pct create "$CTID" "$OSTEMPLATE" \
  --hostname "$HOSTNAME" \
  --cores "$CORES" --memory "$MEMORY" --swap "$SWAP" \
  --rootfs "$STORAGE:$DISK" \
  --net0 "$NET" \
  --features nesting=1,keyctl=1 \
  --unprivileged "$UNPRIV" \
  --ostype debian \
  --onboot "$ONBOOT"

echo ">> starting CT $CTID"
pct start "$CTID"

# ---- wait for the container network ----
echo -n ">> waiting for network"
for _ in $(seq 1 30); do
  if pct exec "$CTID" -- getent hosts deb.debian.org >/dev/null 2>&1; then break; fi
  echo -n "."; sleep 2
done
echo

# ---- lay the rootfs bundle over the container ----
echo ">> applying bundle $(basename "$BUNDLE")"
pct push "$CTID" "$BUNDLE" /tmp/blueot-bundle.tar.gz
pct exec "$CTID" -- tar -xzf /tmp/blueot-bundle.tar.gz -C /
pct exec "$CTID" -- rm -f /tmp/blueot-bundle.tar.gz
# ensure the exec bit is set even if the bundle was built where tar did not
# record it (e.g. on Windows/MSYS)
pct exec "$CTID" -- chmod 0755 /opt/blue-ot/simulator /opt/blue-ot/blue-proxy /usr/local/bin/blueot-sniff

# ---- install OT tools ----
PKGS="tcpdump iptables tshark vim nano iproute2 iputils-ping net-tools less procps ca-certificates"
[ "$CLOUDINIT" = 1 ] && PKGS="$PKGS cloud-init"
echo ">> installing tools: $PKGS"
pct exec "$CTID" -- bash -c '
  set -e
  export DEBIAN_FRONTEND=noninteractive
  echo "wireshark-common wireshark-common/install-setuid boolean false" | debconf-set-selections
  for i in 1 2 3 4 5; do apt-get update -qq && break || sleep 5; done
  apt-get install -y --no-install-recommends '"$PKGS"'
  apt-get clean && rm -rf /var/lib/apt/lists/*
'

# ---- enable and start the services ----
echo ">> enabling services"
pct exec "$CTID" -- systemctl daemon-reload
pct exec "$CTID" -- systemctl enable --now simulator.service blue-proxy.service

# ---- cloud-init (installed above with the tools) ----
if [ "$CLOUDINIT" = 1 ]; then
  echo ">> provisioning cloud-init (NoCloud seed)"
  # Proxmox owns the hostname/network of an LXC; let cloud-init handle the rest
  # (users, SSH keys, packages, files, commands) from the seed without renaming.
  pct exec "$CTID" -- sh -c 'printf "preserve_hostname: true\n" > /etc/cloud/cloud.cfg.d/91-proxmox.cfg'
  pct exec "$CTID" -- systemctl enable cloud-init.target cloud-init-local.service \
    cloud-init.service cloud-config.service cloud-final.service >/dev/null 2>&1 || true
  # provision now from the baked-in seed (also runs on every future boot)
  pct exec "$CTID" -- cloud-init init --local >/dev/null 2>&1 || true
  pct exec "$CTID" -- cloud-init init        >/dev/null 2>&1 || true
  pct exec "$CTID" -- cloud-init modules --mode=config >/dev/null 2>&1 || true
  pct exec "$CTID" -- cloud-init modules --mode=final  >/dev/null 2>&1 || true
fi

# ---- report ----
echo
echo "=============================================================="
echo " CT $CTID ($HOSTNAME) — simulator: $SIM"
pct exec "$CTID" -- systemctl --no-pager --no-legend list-units 'simulator.service' 'blue-proxy.service' || true
IPADDR="$(pct exec "$CTID" -- hostname -I 2>/dev/null | awk '{print $1}')"
echo " address      : ${IPADDR:-<none yet>}"
echo " Modbus (proxy): ${IPADDR:-<ip>}:1502  ->  simulator :5020"
echo " proxy web     : http://${IPADDR:-<ip>}:8083"
echo " enter         : pct enter $CTID"
echo " sniff         : pct exec $CTID -- blueot-sniff"
[ "$CLOUDINIT" = 1 ] && echo " cloud-init    : $(pct exec "$CTID" -- cloud-init status 2>/dev/null | awk '{print $2}') (NoCloud seed)"
echo "=============================================================="
