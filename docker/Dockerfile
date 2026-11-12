# syntax=docker/dockerfile:1
#
# One image per simulator. Each image runs, under systemd:
#   * the simulator, listening for Modbus TCP on 127.0.0.1:5020 (internal)
#   * blue-proxy, taking over Modbus TCP on :1502 and forwarding to :5020
#
# Build a single image with --build-arg SIM=<name>, or build them all with
# docker/docker-compose.yml. SIM is one of:
#   modsim  blue-station  blue-chp  blue-water  blue-wwtp
#
# The build context is the repository root:
#   docker build -f docker/Dockerfile --build-arg SIM=modsim -t blueot/sim-modsim .

ARG GO_VERSION=1.23

# ---- build stage: the selected simulator and blue-proxy ----
FROM golang:${GO_VERSION}-bookworm AS build
ARG SIM
WORKDIR /src
COPY . .
RUN set -eux; \
    case "$SIM" in \
      modsim)       dir=scada/backend; cmd=modsim;       device=sim; \
                    sim_args="-addr=0.0.0.0:5020 -http=0.0.0.0:8081" ;; \
      blue-station) dir=blue-station;  cmd=blue-station;  device=blue-station; \
                    sim_args="-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8080 -chp=" ;; \
      blue-chp)     dir=blue-chp;      cmd=blue-chp;      device=blue-chp; \
                    sim_args="-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8082" ;; \
      blue-water)   dir=blue-water;    cmd=blue-water;    device=blue-water; \
                    sim_args="-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8085" ;; \
      blue-wwtp)    dir=blue-wwtp;     cmd=blue-wwtp;     device=blue-wwtp; \
                    sim_args="-modbus=0.0.0.0:5020 -iec104= -http=0.0.0.0:8086" ;; \
      *) echo "SIM must be one of: modsim blue-station blue-chp blue-water blue-wwtp (got '${SIM}')" >&2; exit 2 ;; \
    esac; \
    mkdir -p /out/env /out/seed; \
    (cd "$dir"     && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-s -w" -o /out/simulator  "./cmd/$cmd"); \
    (cd blue-proxy && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-s -w" -o /out/blue-proxy ./cmd/blue-proxy); \
    printf 'SIM_NAME=%s\nSIM_ARGS=%s\n' "$SIM" "$sim_args" > /out/env/sim.env; \
    printf 'PROXY_DEVICE=%s\nPROXY_ARGS=-config=/opt/blue-ot/config.json -device=%s -modbus=0.0.0.0:1502 -iec104= -upstream-modbus=127.0.0.1:5020 -http=0.0.0.0:8083\n' \
      "$device" "$device" > /out/env/proxy.env; \
    printf 'instance-id: blueot-%s\nlocal-hostname: %s\n' "$SIM" "$SIM" > /out/seed/meta-data

# ---- runtime stage: systemd + OT troubleshooting tools ----
FROM debian:bookworm-slim
ARG SIM
ENV container=docker DEBIAN_FRONTEND=noninteractive
LABEL org.opencontainers.image.title="blue-ot simulator + blue-proxy" \
      org.opencontainers.image.description="A Blue OT simulator fronted by blue-proxy (Modbus TCP :1502 -> :5020), run as systemd services."

# tshark ships the Modbus/TCP and IEC 60870-5-104 dissectors; keep dumpcap
# non-setuid (the container is privileged anyway).
RUN echo "wireshark-common wireshark-common/install-setuid boolean false" | debconf-set-selections \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      systemd systemd-sysv dbus \
      cloud-init \
      tcpdump iptables tshark \
      vim nano \
      iproute2 iputils-ping net-tools less procps ca-certificates \
 && apt-get clean && rm -rf /var/lib/apt/lists/* \
 # drop systemd units that are useless (or harmful) inside a container
 && systemctl mask \
      systemd-udevd.service systemd-udev-trigger.service \
      systemd-udevd-kernel.socket systemd-udevd-control.socket \
      systemd-modules-load.service systemd-firstboot.service \
      systemd-networkd.service systemd-resolved.service \
      getty.target console-getty.service getty@tty1.service \
      dev-hugepages.mount sys-fs-fuse-connections.mount || true

COPY --from=build /out/simulator      /opt/blue-ot/simulator
COPY --from=build /out/blue-proxy     /opt/blue-ot/blue-proxy
COPY --from=build /out/env/sim.env    /etc/blue-ot/sim.env
COPY --from=build /out/env/proxy.env  /etc/blue-ot/proxy.env
COPY scada/backend/data/config.json   /opt/blue-ot/config.json
# cloud-init: pin the NoCloud datasource and ship a per-SIM seed so the
# container provisions at boot without probing cloud metadata services.
COPY docker/cloud-init/90-blueot.cfg   /etc/cloud/cloud.cfg.d/90-blueot.cfg
COPY docker/cloud-init/user-data       /var/lib/cloud/seed/nocloud/user-data
COPY --from=build /out/seed/meta-data  /var/lib/cloud/seed/nocloud/meta-data
COPY docker/systemd/simulator.service  /etc/systemd/system/simulator.service
COPY docker/systemd/blue-proxy.service /etc/systemd/system/blue-proxy.service
COPY docker/bin/blueot-sniff           /usr/local/bin/blueot-sniff

RUN chmod +x /usr/local/bin/blueot-sniff \
 && systemctl enable simulator.service blue-proxy.service

# 1502 = Modbus TCP via blue-proxy, 8083 = blue-proxy web, 5020 = simulator
# (internal), 8080-8086 = simulator web (depending on SIM).
EXPOSE 1502 5020 8080 8081 8082 8083 8085 8086
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
