# Docker — simulatorer med blue-proxy

Varje simulator körs i en egen container tillsammans med **blue-proxy**.
Inuti containern tar blue-proxy över **Modbus TCP på port 1502** och skickar
vidare till simulatorn på **port 5020**. Både simulatorn och blue-proxy startas
som **systemd-tjänster**. Containern provisioneras med **cloud-init** (NoCloud)
och innehåller verktygen `tcpdump`, `iptables`, `vi`/`vim`, `nano` och `tshark`
(med Modbus- och IEC 104-avkodare).

```
                 :1502 (Modbus TCP)              127.0.0.1:5020 (Modbus TCP)
  SCADA  ───────────────────────►  blue-proxy ───────────────────────►  simulator
                                   :8083 (webb)                         :8080–8086 (webb)
          \_________________________ en container, två systemd-tjänster ________________/
```

## Simulatorer

| Container (`SIM`) | Enhet i config | Simulatorns webb | Proxyns webb |
|---|---|---|---|
| `modsim`        | `sim`          | `:8081` | `:8083` |
| `blue-station`  | `blue-station` | `:8080` | `:8083` |
| `blue-chp`      | `blue-chp`     | `:8082` | `:8083` |
| `blue-water`    | `blue-water`   | `:8085` | `:8083` |
| `blue-wwtp`     | `blue-wwtp`    | `:8086` | `:8083` |

Modbus-proxyn lyssnar alltid på `:1502` inuti containern och vidarebefordrar
till `127.0.0.1:5020`. IEC 104 är avstängt (`-iec104=`), setupen är ren Modbus.

## Bygga och köra

Byggkontexten är **repots rot** (`..`), så kör via compose-filen:

```bash
cd docker
docker compose build            # bygger alla fem avbildningar
docker compose up -d            # startar alla containrar
docker compose ps
docker compose logs -f modsim   # följ systemd-boot och tjänsternas utskrift
```

En enskild avbildning:

```bash
docker build -f docker/Dockerfile --build-arg SIM=blue-chp -t blueot/sim-blue-chp .
docker run -d --name blueot-blue-chp --privileged --tmpfs /run --tmpfs /run/lock \
  -p 1522:1502 -p 8383:8083 -p 8082:8082 blueot/sim-blue-chp
```

### Värdportar (från `docker-compose.yml`)

Portarna multiplexas till värden så att alla fem containrar kan köras samtidigt.
På separata värdar/VM:er kan var och en behålla sina standardportar.

| Container | Modbus (proxy) | Proxyns webb | Simulatorns webb |
|---|---|---|---|
| `modsim`       | `1502` | `8183` | `8081` |
| `blue-station` | `1512` | `8283` | `8080` |
| `blue-chp`     | `1522` | `8383` | `8082` |
| `blue-water`   | `1532` | `8483` | `8085` |
| `blue-wwtp`    | `1542` | `8583` | `8086` |

(Simulatorns Modbus på 5020 exponeras inte — den når man genom proxyn.)

## systemd

Simulatorn och proxyn ligger som `/etc/systemd/system/simulator.service` och
`/etc/systemd/system/blue-proxy.service` och startas automatiskt. Argumenten
läses från `/etc/blue-ot/sim.env` respektive `/etc/blue-ot/proxy.env`.

```bash
docker compose exec modsim bash
systemctl status simulator blue-proxy
journalctl -u blue-proxy -f
systemctl restart simulator
```

systemd kör som PID 1. Det kräver en **privilegierad** container och ett
skrivbart `/run` (båda satta i compose-filen). På en cgroup v2-värd räcker det;
om systemd inte startar, lägg till `cgroup: host` på tjänsten (Compose v2.15+)
eller kör `docker run --cgroupns=host …`.

## cloud-init

Avbildningen innehåller **cloud-init** med en **NoCloud**-datakälla. Datakällan
är låst till `NoCloud, None` (`/etc/cloud/cloud.cfg.d/90-blueot.cfg`), så boot
väntar aldrig på moln-metadata (EC2/Azure/GCE). En per-container-seed bakas in i
`/var/lib/cloud/seed/nocloud/` (`meta-data` med `instance-id`/`local-hostname`,
samt `user-data`). Nätverket lämnas åt Docker (`network: {config: disabled}`).

Provisionera en container genom att montera en egen seed över den inbakade:

```bash
# docker/cloud-init/user-data är mallen – kopiera och anpassa den
docker compose run --rm \
  -v "$PWD/cloud-init:/var/lib/cloud/seed/nocloud:ro" modsim
```

eller i `docker-compose.yml` under en tjänst:

```yaml
    volumes:
      - ./cloud-init:/var/lib/cloud/seed/nocloud:ro
```

Med `user-data` kan du lägga till användare, SSH-nycklar, paket, filer och
kommandon vid boot. Verifiera inuti containern:

```bash
cloud-init status --long
cat /etc/blue-ot/cloud-init.txt     # markör som exempel-user-data skriver
```

cloud-init kör bara per `instance-id`. För att provisionera om: ändra
`instance-id` i `meta-data`, eller kör `cloud-init clean --logs` och starta om.

## Felsökningsverktyg

Containern innehåller `tcpdump`, `iptables`, `vi`/`vim`, `nano` och `tshark`.
tshark har inbyggda avkodare för både Modbus/TCP och IEC 60870-5-104.

Hjälpskriptet `blueot-sniff` kopplar avkodarna till de portar simulatorn
använder (1502/5020 är inte Modbus standardport 502):

```bash
docker compose exec blue-station bash
blueot-sniff            # avkoda all Modbus-trafik (proxy + simulator) live
blueot-sniff proxy -V   # bara SCADA ↔ proxy, utförligt
blueot-sniff all -w /tmp/ot.pcap   # spara en capture för Wireshark

# eller direkt:
tshark -i any -f 'tcp port 1502' -d 'tcp.port==1502,mbtcp'
tcpdump -i any -n 'tcp port 1502'
iptables -L -n
```

> Containrarna är privilegierade och har `NET_ADMIN`/`NET_RAW`, så `tcpdump`,
> `tshark` och `iptables` fungerar fullt ut. Detta är avsett för en avskild
> labb-/övningsmiljö, inte för produktion.
