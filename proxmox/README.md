# Proxmox — simulatorer som LXC-containrar

Proxmox VE kör **LXC** med systemd som PID 1, vilket passar den här uppsättningen
direkt: simulatorn och **blue-proxy** körs som systemd-tjänster, och OT-verktygen
(`tcpdump`, `tshark`, `iptables`) fungerar i containern. Inuti varje container tar
blue-proxy över **Modbus TCP på :1502** och vidarebefordrar till simulatorn på
**:5020** — samma modell som Docker-uppsättningen i [`../docker`](../docker).

Arbetsgången är två steg:

1. **Bygg ett rootfs-paket** per simulator på en dator med Go (`build-bundle.sh`).
2. **Installera** på Proxmox-värden med `pct` (`install.sh`), som lägger paketet
   över en Debian-LXC, installerar verktygen och startar tjänsterna.

Ingen Docker behövs. `build-bundle.sh` kräver bara Go; `install.sh` kräver bara
en Proxmox-värd (`pct`, `pveam`).

## 1. Bygg paketet

På en dator med Go (behöver inte vara Proxmox-värden):

```bash
./proxmox/build-bundle.sh modsim        # en simulator
./proxmox/build-bundle.sh all           # alla fem
```

Resultatet hamnar i `proxmox/dist/blueot-<sim>.tar.gz`. Paketet innehåller
simulator- och blue-proxy-binärerna, systemd-enheterna, env-filerna, `config.json`,
`blueot-sniff` och cloud-init-seeden — allt utom själva operativsystemet.

Simulatorer: `modsim`, `blue-station`, `blue-chp`, `blue-water`, `blue-wwtp`
(bygg för annan arkitektur med `ARCH=arm64 ./proxmox/build-bundle.sh ...`).

## 2. Installera på Proxmox-värden

Kopiera paketet (och `install.sh`) till värden och kör som **root**:

```bash
scp proxmox/dist/blueot-modsim.tar.gz proxmox/install.sh root@pve:/root/
ssh root@pve
./install.sh --bundle blueot-modsim.tar.gz
```

Skriptet hämtar vid behov Debian 12-mallen, skapar en LXC, lägger in paketet,
installerar verktygen och startar `simulator` + `blue-proxy`. Till sist skrivs
containerns IP, Modbus-port och proxyns webbadress ut.

### Vanliga flaggor

```bash
./install.sh --bundle blueot-blue-chp.tar.gz \
  --ctid 210 \
  --hostname blue-chp \
  --storage local-lvm \          # rootfs-lagring
  --bridge vmbr0 \
  --ip 192.168.10.42/24 --gw 192.168.10.1
```

| Flagga | Standard | Beskrivning |
|---|---|---|
| `--bundle PATH` | — | rootfs-paketet (krävs) |
| `--sim NAME` | härleds ur filnamnet | simulatornamn |
| `--ctid N` | nästa lediga | container-id |
| `--hostname NAME` | `blueot-<sim>` | värdnamn |
| `--storage NAME` | `local-lvm` | lagring för rootfs |
| `--template-storage N` | `local` | lagring för OS-mallen |
| `--template NAME\|VOLID` | `debian-12-standard` | delsträng eller full volid |
| `--bridge NAME` | `vmbr0` | nätverksbrygga |
| `--ip dhcp\|CIDR` | `dhcp` | IP, t.ex. `10.0.0.5/24` |
| `--gw IP` | — | gateway för statisk IP |
| `--cores/--memory/--swap/--disk` | 1 / 512 / 256 / 4 | resurser (MB resp. GB) |
| `--unprivileged` | privilegierad | skapa oprivilegierad container |
| `--no-onboot` | starta vid boot | hoppa över autostart |
| `--cloud-init` | av | installera cloud-init också |

### Privilegierad vs oprivilegierad

Standard är en **privilegierad** container så att `tcpdump`, `tshark` och
`iptables` har de capabilities de behöver (`NET_RAW`/`NET_ADMIN`) — avsett för en
avskild labbmiljö. `--unprivileged` är säkrare men vissa verktyg kan då sakna
rättigheter. Containern skapas med `nesting=1,keyctl=1` i båda fallen.

### cloud-init

Paketet innehåller samma cloud-init-seed som Docker-avbildningen
(`/var/lib/cloud/seed/nocloud/`). `install.sh` installerar och provisionerar
cloud-init **som standard** (NoCloud-datakällan) och verifiering görs direkt vid
installationen. Eftersom Proxmox äger LXC:ns värdnamn och nät läggs en drop-in
(`/etc/cloud/cloud.cfg.d/91-proxmox.cfg`, `preserve_hostname: true`) till så att
cloud-init inte döper om containern — den sköter resten (användare, SSH-nycklar,
paket, filer, kommandon) från seeden. Anpassa `docker/cloud-init/user-data` före
bygget. Hoppa över cloud-init med `--no-cloud-init`.

Verifiera i containern:

```bash
pct exec <CTID> -- cloud-init status --long
pct exec <CTID> -- cat /etc/blue-ot/cloud-init.txt   # markör från exempel-user-data
```

## Drift

```bash
pct enter <CTID>
systemctl status simulator blue-proxy
journalctl -u blue-proxy -f
blueot-sniff                 # avkoda Modbus live (proxy + simulator)
blueot-sniff all -w /tmp/ot.pcap
```

Flera simulatorer blir var sin container (olika CTID och IP); på samma brygga kan
alla behålla standardportarna 1502/5020/8083 eftersom de har skilda adresser.

## Alternativ: återanvänd Docker-avbildningen som LXC-mall

Har du redan byggt Docker-avbildningen kan du exportera dess rootfs till en
Proxmox-mall i stället för att använda paketet ovan:

```bash
docker build -f docker/Dockerfile --build-arg SIM=modsim -t blueot/sim-modsim .
cid=$(docker create blueot/sim-modsim)
docker export "$cid" | zstd > blueot-modsim.tar.zst
docker rm "$cid"
# lägg blueot-modsim.tar.zst i Proxmox under local:vztmpl och:
pct create <CTID> local:vztmpl/blueot-modsim.tar.zst \
  --hostname modsim --net0 name=eth0,bridge=vmbr0,ip=dhcp \
  --features nesting=1 --unprivileged 0 --ostype debian
pct start <CTID>
```

Den mallen har redan verktygen, tjänsterna och cloud-init inbakade; `install.sh`
behövs då inte.
