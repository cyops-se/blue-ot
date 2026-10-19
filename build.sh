#!/usr/bin/env sh
# Builds the HMI and all Go programs in Blue OT into bin/<os>-<arch>/.
#
#   ./build.sh                                # for this machine
#   ./build.sh --all                          # Windows and Linux (amd64) in one go
#   ./build.sh --os windows                   # one other target, optionally --arch arm64
#   ./build.sh --target linux/arm64,windows/amd64   # any list of GOOS/GOARCH pairs
#   ./build.sh --skip-frontend                # reuse the HMI already in scada/backend/internal/web/dist
#   ./build.sh --test                         # run go test in every module first
#
# Requires Go 1.21+ and, unless --skip-frontend, Node 18+.
set -eu

ROOT=$(cd "$(dirname "$0")" && pwd)
OUT="$ROOT/bin"
OS_ARG=""
ARCH_ARG=""
TARGETS=""
FRONTEND=1
TEST=0

while [ $# -gt 0 ]; do
  case "$1" in
    --os) OS_ARG=$2; shift 2 ;;
    --arch) ARCH_ARG=$2; shift 2 ;;
    --target) TARGETS="$TARGETS $(echo "$2" | tr ',' ' ')"; shift 2 ;;
    --all) TARGETS="$TARGETS windows/amd64 linux/amd64"; shift ;;
    --out) OUT=$2; shift 2 ;;
    --skip-frontend) FRONTEND=0; shift ;;
    --test) TEST=1; shift ;;
    -h|--help) sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "okänd flagga: $1" >&2; exit 2 ;;
  esac
done

# --os/--arch, or this machine, when no --target/--all is given
if [ -z "$TARGETS" ]; then
  TARGETS="${OS_ARG:-$(go env GOOS)}/${ARCH_ARG:-$(go env GOARCH)}"
fi

# module directory and the commands in it
MODULES="scada/backend:scada,modsim,opctl
blue-station:blue-station
blue-chp:blue-chp
blue-water:blue-water
blue-wwtp:blue-wwtp
blue-proxy:blue-proxy"

if [ "$FRONTEND" = 1 ]; then
  echo "==> HMI (scada/frontend)"
  cd "$ROOT/scada/frontend"
  [ -d node_modules ] || npm ci
  npm run build
fi

if [ "$TEST" = 1 ]; then
  echo "$MODULES" | while IFS=: read -r dir _; do
    echo "==> go test $dir"
    (cd "$ROOT/$dir" && go test ./...)
  done
fi

for target in $TARGETS; do
  os=${target%/*}
  arch=${target#*/}
  if [ -z "$os" ] || [ -z "$arch" ] || [ "$os" = "$target" ]; then
    echo "ogiltigt mål: $target (ange GOOS/GOARCH, t.ex. linux/amd64)" >&2; exit 2
  fi
  ext=""
  [ "$os" = windows ] && ext=".exe"
  dest="$OUT/$os-$arch"
  mkdir -p "$dest"

  echo "$MODULES" | while IFS=: read -r dir cmds; do
    for cmd in $(echo "$cmds" | tr ',' ' '); do
      echo "==> $cmd ($os/$arch)"
      (cd "$ROOT/$dir" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -trimpath -ldflags "-s -w" -o "$dest/$cmd$ext" "./cmd/$cmd")
    done
  done

  # scada needs a data directory; copy the bundled one once, never overwrite edits
  if [ ! -d "$dest/data" ]; then
    cp -R "$ROOT/scada/backend/data" "$dest/data"
    echo "==> kopierade scada/backend/data till $dest/data"
  fi
  echo "Klart: $dest"
done
