[Svenska](README.md) · **English**

# Blue Proxy – Modbus TCP / IEC 104 proxy with frozen reporting and command blocking

Blue Proxy sits between a SCADA master and a downstream device (for example [blue-station](../blue-station/README-en.md) or [blue-chp](../blue-chp/README-en.md)). It forwards traffic transparently, but remembers the collected states and the last commands, and per tag or for all tags of a type it can **freeze** the last reported value so it keeps being reported regardless of what the source sends, and **block commands** so they never reach the device. A web interface shows both the collected and the reported value and lets you switch the modes.

This is a lab tool: for studying how a SCADA system behaves when an intermediate gateway freezes or manipulates telemetry (for example when testing alarms, redundancy and operating procedures).

```
   SCADA master ──Modbus TCP──►              ──Modbus TCP──►  device
                              Blue Proxy                       (blue-station,
   SCADA master ──IEC 104────►   :1510/:2414  ──IEC 104─────►   blue-chp …)
                                    │
                               web interface :8083
                               collected vs reported, freeze/release
```

## How it works

- **Normal mode (transparent):** every request and response is forwarded unchanged. The proxy decodes reads to remember the collected value, and writes (commands) to remember the last command.
- **Frozen mode (hold):** when a point is frozen the reported value is locked at the last reported value. The proxy keeps collecting the real value from the source, but writes the frozen value into the response to the master. For Modbus the registers or bits are rewritten, for IEC 104 the information object is rewritten (length and sequence numbers are preserved, so the proxy is transparent to the protocol).
- **Blocked command (block):** when a point is blocked, commands from the master are rejected instead of being forwarded to the device. For Modbus the proxy replies with the *server device failure* exception (0x04) without forwarding the write. For IEC 104 the address in the command is invalidated so the device itself replies negatively (length and sequence numbers preserved); the command never reaches the process. The command is still logged as the last command, marked as blocked.
- **Per tag or per type:** freezing and blocking can each be turned on or off for a single tag, for all tags of a type (breaker, disconnector, pump, measurement, binary, setpoint) or for every point.

The tags, their addresses and IEC 104 IOAs come from the SCADA `config.json` (the same file SCADA and opctl use). The proxy runs in front of **one** device at a time.

## Running

```bash
cd blue-proxy
go run ./cmd/blue-proxy -device blue-station
go test ./...
```

By default it reads `../scada/backend/data/config.json`, sits in front of the `blue-station` device and listens on Modbus `:1510`, IEC 104 `:2414` and web `:8083`. The upstream is taken from the device's `host`/`port` and `iec104.port` in the configuration.

| Flag | Default | Description |
|---|---|---|
| `-config` | `../scada/backend/data/config.json` | SCADA's config.json (or its directory) |
| `-device` | `blue-station` | Device in the configuration to proxy |
| `-modbus` | `:1510` | Listen address for the Modbus TCP proxy (empty = off) |
| `-iec104` | `:2414` | Listen address for the IEC 104 proxy (empty = off) |
| `-http` | `0.0.0.0:8083` | Web interface; listens on all interfaces by default, give e.g. `127.0.0.1:8083` for local only (empty = off) |
| `-upstream-modbus` | – | Override the upstream Modbus `host:port` |
| `-upstream-iec104` | – | Override the upstream IEC 104 `host:port` |
| `-timeout` | `5s` | Upstream timeout |

Example: proxy blue-chp instead, on other ports:

```bash
go run ./cmd/blue-proxy -device blue-chp -modbus :1511 -iec104 :2415 -http :8084
```

### Try it with SCADA

1. Start the device, e.g. `cd blue-station && go run ./cmd/blue-station -http :8090`.
2. Start the proxy: `cd blue-proxy && go run ./cmd/blue-proxy -device blue-station`.
3. Point SCADA (or opctl) at the proxy ports instead of the device's: set the device's `host`/`port` to the proxy (`127.0.0.1:1510`) in a copy of the data directory, or use `opctl -data <copy>`.
4. Open `http://localhost:8083`, freeze a measurement and watch the collected value keep changing while the reported value (and what SCADA sees) stands still.

## Web interface and API

`http://localhost:8083` shows a table with the point's name and id, type, protocol, collected value (from the source), reported value (out to the master), last command and two buttons per row: **Rapportering** (freeze/transparent) and **Manöver** (block/allow). At the top there is a selector to freeze, release, block or allow a whole group, a filter and options to show only frozen or only blocked points. Reported values that differ from the collected one are highlighted, as are blocked commands.

- `GET /api/state` – all points with collected and reported value, hold and block mode, last command and categories.
- `POST /api/hold` – `{"key": "Grid.U", "on": true}` for a tag, or `{"category": "mätvärde", "on": true}` for a type (`all` for every point).
- `POST /api/block` – same shape as `/api/hold`, but blocks or allows commands.

The same API can be driven from the command line with `opctl -proxy <url> …` (see [scada/README-en.md](../scada/README-en.md#driving-a-blue-proxy)).

## What is supported

- **Modbus TCP:** function codes 1, 2, 3, 4 (reads, collection and freezing) and 5, 6, 15, 16 (writes, remembered command). float32 and the other data types are decoded according to the tag's settings (scale, offset, word and byte order).
- **IEC 60870-5-104:** monitoring `M_SP`, `M_DP`, `M_ST`, `M_ME_NC` (with and without time tag), both single objects and sequences (SQ). Commands `C_SC`, `C_DC`, `C_RC`, `C_SE_NC` are remembered as the last command and can be blocked. Freezing rewrites the information object's value without changing quality bits, time tags, length or sequence numbers.

The interface and the point types are in Swedish.

## Structure

```
cmd/blue-proxy/      main, flags
internal/config/     loads SCADA's config.json, classifies tags, decoding (codec)
internal/state/      the memory: collected, reported, hold, last command
internal/mbproxy/    transparent Modbus TCP proxy
internal/iecproxy/   transparent IEC 60870-5-104 proxy
internal/web/        web interface and JSON API
```

## Limitations

- The proxy runs in front of one device per instance. Run several instances for several devices.
- IEC 104 freezing and blocking require the tags to have a monitor IOA (`ioa`) and control IOA (`ctrlIoa`) respectively in the configuration; these are filled in by the generator scripts. Devices without `iec104` in the configuration are proxied over Modbus only.
- Freezing only affects the downstream reporting; blocking only affects upstream commands. They are independent and can be combined (for example freezing a breaker's position while blocking commands to it).
- No authentication or encryption, like the rest of Blue OT. For isolated lab networks only.
