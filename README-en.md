[Svenska](README.md) · **English**

# Blue OT

Blue OT is a lab environment for OT (operational technology and industrial control systems). It consists of a SCADA system and three simulators that together form a small power system. Everything is written in Go, and the SCADA system has a web-based HMI built with Vue and Vuetify. The simulators speak real industrial protocols, Modbus TCP and IEC 60870-5-104, so they can also be used with other SCADA systems, protocol analysers or security tools.

## Components

| Component | What it is | Interfaces | Documentation |
|---|---|---|---|
| **scada** | SCADA system with a Go backend and a web HMI. It polls devices over Modbus TCP, shows process displays and measurements, logs events and lets the operator operate breakers, switches and setpoints. Displays are drawn in a built-in view editor. Backend and HMI ship as a single executable. | HTTP/REST and WebSocket (HMI), Modbus TCP client | [scada/README-en.md](scada/README-en.md) |
| **modsim** | A small Modbus TCP simulator included in the SCADA project. It simulates a 10 kV switchgear with four bays, breakers, interlocked disconnectors and measurements. It is meant for testing SCADA quickly without anything else. | Modbus TCP server | [scada/README-en.md](scada/README-en.md#modsim) |
| **blue-station** | Simulator of a 220/20 kV transformer substation. It has a transmission grid infeed, two transformers with on-load tap changers and voltage control, a sectionalised 20 kV busbar, four feeders with daily load profiles and overcurrent protection, and bay G1 where the CHP plant is connected. | Modbus TCP, IEC 60870-5-104, web interface with JSON API | [blue-station/README-en.md](blue-station/README-en.md) |
| **blue-chp** | Simulator of a combined heat and power plant with two boilers, a steam header with bypass, a back-pressure turbine with generator, and a district heating network with pumps and a heat accumulator. The generator feeds electricity into blue-station. | Modbus TCP, IEC 60870-5-104, web interface with JSON API, link to blue-station | [blue-chp/README-en.md](blue-chp/README-en.md) |
| **blue-proxy** | Transparent Modbus TCP / IEC 104 proxy between SCADA and a device. It remembers collected states and the last commands and can, per tag or type, freeze the reported value and block commands. A web interface shows collected vs reported and controls the modes. | Modbus TCP, IEC 60870-5-104 (proxy), web interface | [blue-proxy/README-en.md](blue-proxy/README-en.md) |

## How the parts fit together

```
                         ┌────────────────────────────────────────┐
   browser ───HTTP/WS───►  scada  (HMI + REST + WebSocket, :8080) │
                         └───┬───────────────┬────────────────┬───┘
                             │ Modbus TCP    │ Modbus TCP     │ Modbus TCP
                             ▼               ▼                ▼
                      ┌──────────┐   ┌───────────────┐   ┌───────────────┐
                      │  modsim  │   │ blue-station  │   │   blue-chp    │
                      │  :5020   │   │ :502  IEC:2404│   │ :1502 IEC:2405│
                      └──────────┘   │ web (optional)│   │ web :8082     │
                                     └───────┬───────┘   └───────▲───────┘
                                             │  bay G1, 20 kV    │
                                             └──── /api/link ────┘
                                     busbar voltage, frequency and breaker state →
                                     ← generator P and Q
```

- **SCADA ↔ simulators:** SCADA is a Modbus client and polls each simulator as a separate device. The tag and view configuration for blue-station and blue-chp is included in `scada/backend/data` and can be regenerated from the simulators' signal lists.
- **blue-station ↔ blue-chp:** the link represents the 20 kV cable between the plant and the substation. It is not SCADA communication. Every second the substation sends the state of the connection point to the plant and receives the generator output, which is fed into busbar section B. If the bay is opened or the busbar loses voltage, the generator's loss-of-mains protection trips. Both simulators also work on their own.

## Quick start

Requirements: Go 1.21 or later. Rebuilding the HMI also requires Node 18 or later.

Start each part in its own terminal:

```bash
# Simulators
cd blue-chp     && go run ./cmd/blue-chp                     # Modbus :1502, IEC 104 :2405, web :8082
cd blue-station && go run ./cmd/blue-station -http :8090     # Modbus :502,  IEC 104 :2404, web :8090
cd scada/backend && go run ./cmd/modsim                      # Modbus :5020

# SCADA – build the HMI once, then start the backend
cd scada/frontend && npm install && npm run build
cd scada/backend  && go run ./cmd/scada -data data           # http://localhost:8080
```

Open http://localhost:8080 and pick one of the views **Station Norra** (modsim), **Blue Station** or **Blue CHP**. The simulators' own web interfaces, with all signals and an event log, are at http://localhost:8090 (blue-station) and http://localhost:8082 (blue-chp).

blue-station also uses port 8080 for its web interface by default. When SCADA runs on the same machine, the substation's web interface must therefore be moved with `-http`, as in the example above. On Linux, port 502 for Modbus requires root or `CAP_NET_BIND_SERVICE`. Otherwise use e.g. `-modbus :5021` and change the port of the `blue-station` device in SCADA.

### Ports

| Component | Modbus TCP | IEC 60870-5-104 | HTTP |
|---|---|---|---|
| scada | (client) | – | 8080 |
| modsim | 5020 | – | – |
| blue-station | 502 | 2404 (CA 1) | 8080 by default, 8090 in the example above |
| blue-chp | 1502 | 2405 (CA 2) | 8082 (web and link to blue-station) |

### Tools and proxy

- **opctl** (`scada/backend/cmd/opctl`): operates breakers, disconnectors and pumps from the command line over Modbus TCP or IEC 104, with addressing from the SCADA configuration. See [scada/README-en.md](scada/README-en.md#opctl).
- **blue-proxy**: sits between SCADA and a device to freeze reported values and block commands. See [blue-proxy/README-en.md](blue-proxy/README-en.md).

## Directory layout

```
blue-ot/
├── scada/
│   ├── backend/        Go: SCADA server (cmd/scada), modsim (cmd/modsim), opctl (cmd/opctl), data/ with configuration and views, tools/ with generator scripts
│   └── frontend/       Vue 3 + Vuetify 3: HMI, embedded in the backend executable
├── blue-station/       Go: 220/20 kV transformer substation simulator
├── blue-chp/           Go: CHP plant simulator with district heating
└── blue-proxy/         Go: transparent Modbus TCP / IEC 104 proxy with freezing and command blocking
```

Each Go directory (`scada/backend`, `blue-station`, `blue-chp` and `blue-proxy`) is a separate module. Run the tests with `go test ./...` in each directory. The simulators and the proxy use only the Go standard library.

## Security

Everything in Blue OT is meant for lab, training and test use. Neither SCADA nor the simulators have authentication or encryption, and operations are accepted from anyone who can reach the ports. Only run them on isolated networks, and never connect SCADA to real equipment without adding access control first.

The user interfaces, event texts and signal descriptions are in Swedish.

## License

MIT, see [LICENSE](LICENSE).
