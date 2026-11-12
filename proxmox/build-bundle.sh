#!/usr/bin/env bash
#
# build-bundle.sh — build a Proxmox LXC rootfs overlay for one simulator.
#
# Produces proxmox/dist/blueot-<SIM>.tar.gz: a tar whose paths are laid over the
# root of a stock Debian LXC by proxmox/install.sh. It contains the simulator
# and blue-proxy binaries, their systemd units and env files, the config, the
# blueot-sniff helper and the cloud-init seed. Only Go is required to build it
# (no Docker, no Proxmox) — run it on a dev box, then copy the tarball to the
# Proxmox host.
#
#   ./proxmox/build-bundle.sh modsim
#   ./proxmox/build-bundle.sh all          # build every simulator
#
# The SIM argument map is kept in sync with docker/Dockerfile.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARCH="${ARCH:-amd64}"
OUT_DIR="${OUT_DIR:-$ROOT/proxmox/dist}"
SIMS="modsim blue-station blue-chp blue-water blue-wwtp"

build_one() {
  local SIM="$1" dir cmd device sim_args
  case "$SIM" in
    modsim)       dir=scada/backend; cmd=modsim;       device=sim
                  sim_args='-addr=0.0.0.0:5020 -http=0.0.0.0:8081' ;;
    blue-station) dir=blue-station;  cmd=blue-station;  device=blue-station
                  sim_args='-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8080 -chp=' ;;
    blue-chp)     dir=blue-chp;      cmd=blue-chp;      device=blue-chp
                  sim_args='-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8082' ;;
    blue-water)   dir=blue-water;    cmd=blue-water;    device=blue-water
                  sim_args='-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8085' ;;
    blue-wwtp)    dir=blue-wwtp;     cmd=blue-wwtp;     device=blue-wwtp
                  sim_args='-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8086' ;;
    *) echo "unknown SIM '$SIM' (want: $SIMS)" >&2; return 2 ;;
  esac

  local stage="$WORK/$SIM"
  rm -rf "$stage"
  mkdir -p \
    "$stage/opt/blue-ot" \
    "$stage/etc/blue-ot" \
    "$stage/etc/systemd/system" \
    "$stage/usr/local/bin" \
    "$stage/etc/cloud/cloud.cfg.d" \
    "$stage/var/lib/cloud/seed/nocloud"

  echo ">> building $SIM ($ARCH)"
  ( cd "$ROOT/$dir" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
      go build -trimpath -buildvcs=false -ldflags '-s -w' \
      -o "$stage/opt/blue-ot/simulator" "./cmd/$cmd" )
  ( cd "$ROOT/blue-proxy" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
      go build -trimpath -buildvcs=false -ldflags '-s -w' \
      -o "$stage/opt/blue-ot/blue-proxy" ./cmd/blue-proxy )

  cp "$ROOT/scada/backend/data/config.json"      "$stage/opt/blue-ot/config.json"
  cp "$ROOT/docker/systemd/simulator.service"    "$stage/etc/systemd/system/simulator.service"
  cp "$ROOT/docker/systemd/blue-proxy.service"   "$stage/etc/systemd/system/blue-proxy.service"
  cp "$ROOT/docker/bin/blueot-sniff"             "$stage/usr/local/bin/blueot-sniff"
  cp "$ROOT/docker/cloud-init/90-blueot.cfg"     "$stage/etc/cloud/cloud.cfg.d/90-blueot.cfg"
  cp "$ROOT/docker/cloud-init/user-data"         "$stage/var/lib/cloud/seed/nocloud/user-data"
  chmod +x "$stage/usr/local/bin/blueot-sniff" "$stage/opt/blue-ot/simulator" "$stage/opt/blue-ot/blue-proxy"

  printf 'SIM_NAME=%s\nSIM_ARGS=%s\n' "$SIM" "$sim_args" > "$stage/etc/blue-ot/sim.env"
  printf 'PROXY_DEVICE=%s\nPROXY_ARGS=-config=/opt/blue-ot/config.json -device=%s -modbus=0.0.0.0:1502 -iec104= -upstream-modbus=127.0.0.1:5020 -http=0.0.0.0:8083\n' \
    "$device" "$device" > "$stage/etc/blue-ot/proxy.env"
  printf 'instance-id: blueot-%s\nlocal-hostname: %s\n' "$SIM" "$SIM" \
    > "$stage/var/lib/cloud/seed/nocloud/meta-data"
  printf '%s\n' "$SIM" > "$stage/opt/blue-ot/.sim"

  mkdir -p "$OUT_DIR"
  tar --numeric-owner --owner=0 --group=0 -C "$stage" -czf "$OUT_DIR/blueot-$SIM.tar.gz" .
  echo "<< $OUT_DIR/blueot-$SIM.tar.gz"
}

main() {
  local target="${1:-}"
  if [ -z "$target" ]; then
    echo "usage: $0 <modsim|blue-station|blue-chp|blue-water|blue-wwtp|all>" >&2
    exit 2
  fi
  command -v go >/dev/null || { echo "go toolchain not found in PATH" >&2; exit 1; }
  WORK="$(mktemp -d)"
  trap 'rm -rf "$WORK"' EXIT
  if [ "$target" = all ]; then
    for s in $SIMS; do build_one "$s"; done
  else
    build_one "$target"
  fi
}

main "$@"
