[Svenska](README.md) · **English**

# Blue OT SCADA

The SCADA part of [Blue OT](../README-en.md) consists of a Go backend that polls field devices over **Modbus TCP or IEC 60870-5-104** and a web-based HMI built with Vue 3 and Vuetify 3. The HMI is embedded in the backend executable, so the same program serves the REST API, the WebSocket and the web interface. The project also includes **modsim**, a small Modbus simulator for testing the system without other equipment.

What it includes:

- **Process displays** with static graphics and dynamic symbols: breakers, disconnectors, switches, pumps, valves, measurements, bars, indicators, buttons and view links. Values update in real time over WebSocket.
- **Operation** of breakers, disconnectors, switches, pumps, valves and setpoints. Each operation requires a selection and then a confirmation (select-before-operate), and the dialog shows when the position has changed.
- **View editor** in the browser. There you draw displays and bind each symbol to the tags and values it should show.
- **Measurement table** with floating-point values, quality and timestamp, a trend per tag, and direct writes to writable tags.
- **Alarms** with acknowledgement: alarms for breakers that open without an operation from the HMI, for alarm signals (protection tripped, overload etc.) and for communication failures. The latest alarms are shown at the top of every page.
- **Event list** with operations, position changes, communication status and configuration changes.
- **Configuration** of devices and tags in the HMI. Tags can use any Modbus area, several data types, any word and byte order, and scale and offset.

The user interface is in Swedish.

## Architecture

```
 browser ── HTTP / WebSocket ──► scada.exe ── Modbus TCP ──► devices (modsim, blue-station, blue-chp …)
                                    │
                                    └── data/ (config.json, views/*.json, images/)
```

| Part | Contents |
|---|---|
| `backend/cmd/scada` | Main program: starts the poller, the HTTP server and the embedded HMI |
| `backend/cmd/modsim` | Modbus TCP simulator (see [modsim](#modsim)) |
| `backend/cmd/opctl` | Command-line tool to operate breakers and disconnectors (see [opctl](#opctl)) |
| `backend/internal/mbus` | One poller per device. It merges nearby addresses into few reads, decodes data types, reconnects on errors and writes for operations |
| `backend/internal/realtime` | Process image, in-memory history (3600 values per tag), event list (last 1000), alarm list and fan-out to WebSocket clients |
| `backend/internal/alarm` | Alarm engine: raises and clears alarms from tag values, operations and communication status |
| `backend/internal/store` | JSON storage of devices, tags, views and uploaded images |
| `backend/internal/api` | REST API and WebSocket |
| `backend/internal/web` | Embedded HMI (`go:embed` of `dist/`, written by the frontend build) |
| `backend/tools` | Scripts that generate devices, tags and views for blue-station, blue-chp, blue-water and blue-wwtp |
| `frontend/` | HMI in Vue 3, Vuetify 3, Pinia and Vite |

## Building and running

Requirements: Go 1.21 or later and Node 18 or later.

```bash
cd frontend
npm install
npm run build                    # writes to backend/internal/web/dist

cd ../backend
go build -o bin/ ./cmd/scada ./cmd/modsim ./cmd/opctl
./bin/modsim                     # Modbus simulator on :5020
./bin/scada -data data           # HMI and API on http://localhost:8080
```

The `scada` executable only needs the `data/` directory. Build the HMI before `go build`, since frontend changes are only included when the backend is rebuilt. If the backend is built without the HMI it still starts, but the web interface replies that the HMI is missing.

**HMI development:** `npm run dev` in `frontend/` starts Vite's dev server on http://localhost:5173, which reloads changes immediately. Requests to `/api`, `/ws` and `/images` are forwarded to the backend on :8080, and the `BACKEND` environment variable sets another address.

| Flag (`scada`) | Default | Description |
|---|---|---|
| `-addr` | `:8080` | HTTP address |
| `-data` | `data` | Data directory with `config.json`, `views/` and `images/` |
| `-web` | – | Serve the HMI from a directory on disk instead of the embedded build |

## Usage

### Process display (`/views/:id`)

- **Breakers, disconnectors, switches, pumps and valves:** click to open the operation dialog. Select *Frånslag/Tillslag* (open/close; or *Öppna/Slut*, *PÅ/AV*, *START/STOPP*, *ÖPPNA/STÄNG*) and confirm with *Utför* (execute). The dialog waits for the position to change and then closes. If the device rejects the operation, e.g. because of an interlock or lockout, the error is shown in the dialog.
- **Command buttons:** write a fixed value, or ask for a value (setpoint).
- **Measurements and bars:** click to see a trend for the last 5–60 minutes.
- **Quality:** values from a device without communication are shown in purple as `???`, and symbols are shown with `?`.

### View editor (`/views/:id/edit`)

- Drag symbols from the palette onto the canvas, or double-click a symbol to place it in the centre.
- Select an element to change position, size, rotation, colours and **tag binding** in the properties panel. The tag is chosen from a searchable list, and for operations only writable tags are shown.
- Lines and busbars have handles at both ends. Other elements have a resize handle at the bottom right.
- Shortcuts: `Del` deletes, `Ctrl+D` duplicates, the arrow keys move (`Shift` for larger steps, `Alt` for 1 px), `Ctrl+Z` undoes and `Ctrl+S` saves.
- Images (png, jpg, svg, webp) are uploaded through the image element and stored in `data/images/`.

| Symbol | Configuration |
|---|---|
| Line, busbar, rectangle, ellipse, text, transformer | Colour, line width and **dynamic colour**: a tag and rules (`==`, `!=`, `>`, `>=`, `<`, `<=`), where the first matching rule applies |
| Image, earth | Static graphics |
| Breaker, disconnector | Position tag and values for open and closed, control tag and values for open and close commands, colours |
| Switch | Like a breaker, but with custom ON and OFF texts (e.g. AUTO/MAN, FAULT/OFF) |
| Pump, valve | Like a switch, but drawn as a pump or a valve in the colour of its state (running/stopped, open/closed) |
| Measurement | Tag, label, unit and decimals (taken from the tag by default), limits with alarm colour |
| Indicator | Tag, active value, colours, blinking |
| Bar | Tag, min and max. Fills horizontally if the bar is wider than it is tall |
| Command button | Writable tag and value. With "Fråga efter värde" (ask for value) it works as a setpoint input |
| View link | Navigation to another view |

### Alarms

The alarm list contains every alarm that is active or not yet acknowledged:

| State | Display |
|---|---|
| Active, unacknowledged | Red text, blinking bell |
| Active, acknowledged | Dark yellow text, steady dark yellow bell. Stays until the fault has gone |
| Returned, unacknowledged | White text, blinking grey bell. Stays until acknowledged |
| Returned, acknowledged | Leaves the list |

- **Alarm banner:** the top of every page shows the three latest unacknowledged alarms. When all are acknowledged, it shows the three latest standing alarms instead. Alarms are acknowledged one at a time or with *Kvittera alla* (acknowledge all). Each alarm shows which station it comes from, and clicking it opens the view that shows the object.
- **Alarm list (`/alarms`):** all alarms with time, type, station, object, state and who acknowledged them. The station is a link to the view, and clicking the row opens it.
- **Alarm sources:**
  - **Tags with an alarm:** the alarm is active while the raw value is one of the tag's alarm values, e.g. `1` for "Utlöst" (tripped).
  - **Uncommanded position change:** for tags with *Bara utan manöver* (only without operation), the alarm is raised only when the tag changes to an alarm value without the same value having been operated from the HMI in the last 30 seconds. This is used for breakers: an operator opening a breaker raises no alarm, but a breaker that trips, or is operated by someone else (e.g. with `opctl` or by an attacker talking directly to the device), does. The alarm stands until the breaker is closed again. The command tag is `<id>.Cmd`, `<id>_CMD` or, for `<x>_POS`, `<x>_CMD`, unless another one is set.
  - **Communication failure:** one alarm per device while it does not respond.
- Alarms, returns to normal and acknowledgements are logged in the event list.

### Measurements, events and configuration

- **Measurements (`/values`):** all tags with value, quality and time of last change. Click a row to see the trend. Writable tags have a button for writing a value.
- **Events (`/events`):** filtered by alarms, operations, position changes, communication and system. Position changes are logged for tags with "Logga ändringar" (log changes) and are shown with the tag's state texts, e.g. "Till → Från".
- **Configuration (`/config`):**
  - **Devices:** protocol (Modbus TCP or IEC 60870-5-104), IP address and port, then unit id (Modbus) or common address CA (IEC 104), poll interval, timeout and enabled/disabled.
  - **Modbus tags:** area (coil, discrete, input, holding), address (0-based), data type (`bool`, `uint16`, `int16`, `uint32`, `int32`, `float32`, `float64`), word and byte swap, scale and offset, bit in register, writable, event logging, state texts and alarm (alarm values, only without operation, alarm text and command tag).
  - **IEC 104 tags:** monitoring IOA, data type (`bool` for M_SP/M_DP, numeric for M_ME/M_ST), control IOA and command type (`C_DC` breaker, `C_SC` switch, `C_RC` step, `C_SE_NC` setpoint) for operable tags, scale, offset, unit, decimals, event logging and state texts. A tag becomes writable when it has a control IOA.
  - Changes are saved to `data/config.json`, and the poller restarts immediately.

## Included devices and views

| View | Device | Address | Tags | Description |
|---|---|---|---|---|
| Station Norra | `sim` | 127.0.0.1:5020 | 31 | 10 kV switchgear with four bays (modsim) |
| Blue Station | `blue-station` | 127.0.0.1:502 | 107 | 220/20 kV transformer substation with bay G1 for CHP, see [blue-station](../blue-station/README-en.md) |
| Blue CHP | `blue-chp` | 127.0.0.1:1502 | 87 (prefix `CHP.`) | CHP plant with district heating, see [blue-chp](../blue-chp/README-en.md) |
| Blue Water | `blue-water` | 127.0.0.1:1503 | 90 (prefix `WTR.`) | Drinking water treatment plant, see [blue-water](../blue-water/README-en.md) |
| Blue WWTP | `blue-wwtp` | 127.0.0.1:1504 | 91 (prefix `WWT.`) | Wastewater treatment plant, see [blue-wwtp](../blue-wwtp/README-en.md) |

Tags and views for the simulators are generated by scripts in `backend/tools/`. Run the scripts in `backend/`:

```bash
python tools/gen_blue_station.py   # device blue-station, tags and the Blue Station view
python tools/gen_blue_chp.py       # device blue-chp, tags and the Blue CHP view (reads the signal list via go run … -points-json)
python tools/gen_blue_water.py     # device blue-water, tags and the Blue Water view
python tools/gen_blue_wwtp.py      # device blue-wwtp, tags and the Blue WWTP view
```

The scripts replace the tags of their device in `config.json` and overwrite the view file. Changes you have made to those views in the editor are therefore lost if you run the script again. gen_blue_water.py and gen_blue_wwtp.py keep an existing device entry, e.g. a changed host or port, and share the helpers for tags and drawing in `tools/scadagen.py`.

Tag conventions for the simulators:

- Breakers and switches have two tags: the position tag reads a discrete input (`F1.CB`) and the control tag writes a coil (`F1.CB.Cmd`).
- Pulse commands, e.g. `Station.Reset` and `CHP.Turbine.EmergencyStop`, are coils written with 1.
- Measurements and setpoints are float32 (ABCD) in input and holding registers respectively.

## modsim

`modsim` simulates a 10 kV switchgear with four bays. Each bay has a breaker Q1–Q4, a disconnector QS1–QS4 and measurements of U, I and P. There are also frequency, busbar voltage and a transformer oil temperature. The simulator shows two common ways of representing switching devices in Modbus:

- **Breakers:** the position is a double-point value in a holding register (0 = intermediate, 1 = open, 2 = closed, 3 = faulty), and the command is given by writing a command register.
- **Disconnectors:** the position is a discrete input, and the command is given through a coil.

The disconnectors are interlocked: they can only be operated when the bay's breaker is open. Otherwise the simulator replies with the exception *illegal data value*.

Like blue-station and blue-chp, modsim has a local **web interface** (`-http`, default `0.0.0.0:8081`). It shows every point per bay with controls and an event log, and at the top a **single-line diagram** of the busbar and the four bays: each bay has its disconnector QS and breaker Q with U/I/P. Closed breakers are drawn as a filled green symbol and open ones as a green outline, the disconnectors as knife switches — the same symbols as in the SCADA view — and bays with an open breaker dim. The interface drives the simulator directly (so a breaker can be opened or closed without going through the SCADA), which is handy for provoking alarms in the HMI, e.g. an uncommanded breaker opening. The interface and its texts are in Swedish.

```bash
go run ./cmd/modsim                         # Modbus on 0.0.0.0:5020, web on :8081
go run ./cmd/modsim -addr :1503 -delay 3s   # other port, breaker operating time 3 s
go run ./cmd/modsim -http ""                # without the web interface
```

| Area | Address | Contents |
|---|---|---|
| Holding | 100–103 | Position of breakers Q1–Q4 (0 = intermediate, 1 = open, 2 = closed, 3 = faulty) |
| Holding | 200–203 | Command for Q1–Q4 (write 1 = open, 2 = close) |
| Discrete | 0–3 | Position of disconnectors QS1–QS4 (1 = closed) |
| Coil | 0–3 | Command for QS1–QS4 (1 = close, 0 = open), interlocked with the breaker |
| Input | 6n, 6n+2, 6n+4 | Bay n (0–3): U kV, I A, P MW (float32, ABCD) |
| Input | 30 / 32 | Frequency Hz / busbar voltage kV (float32) |
| Input | 40 | Oil temperature T1 (int16, scale 0.1 °C) |

The unit id is ignored. The **Station Norra** view and the `sim` device in `data/config.json` are configured for modsim.

## opctl

`opctl` operates a breaker or disconnector from the command line. The tag and the desired state are given as arguments, and everything else (host, port, Modbus address or IEC 104 control IOA) is read from the SCADA configuration file. Both **Modbus TCP** and **IEC 60870-5-104** are supported.

```bash
opctl [flags] <tag> <open|close>
opctl [flags] <all|type:TYP> <open|close>
opctl [flags] -category <TYP> <open|close>
opctl -list [brytare|frånskiljare|pump]
```

- **Tag:** the position tag (`G1.CB`, `Q1_POS`) or the command tag (`G1.CB.Cmd`, `Q1_CMD`). The tool finds the other of the two and the device from the configuration.
- **State:** `open`/`close`, `från`/`till` (open/close), `öppna`/`slut`, `0`/`1`. For pumps, `close` = start (running) and `open` = stop.
- **Whole types:** instead of a tag you can give `all`, `type:<type>` or `-category <type>`, which operates every operable point of the type (`brytare`, `frånskiljare`, `pump`) over the chosen protocol, the same way `-proxy` freezes and blocks per type. Each point is operated as a single manoeuvre, and a count of how many succeeded, failed and were skipped is printed at the end. If any point cannot be operated, the command exits with an error.
- **Connection failure per device:** if a point fails to connect to a device (`host:port`), the remaining points on that same device are skipped straight away, with a clear message, instead of waiting out the timeout again; the manoeuvre continues with the next device. A single point that does reach the device but is rejected (e.g. an interlock) does not skip the rest.

```bash
cd backend
opctl -list                            # list all breakers, disconnectors and pumps
opctl -list pump                       # pumps only
opctl G1.CB close                      # Modbus TCP, reads data/config.json
opctl -protocol iec104 Q0.CB open      # IEC 104
opctl -data ../other/data QS1_POS open # other data directory
opctl type:brytare open                # open every breaker
opctl -category frånskiljare close     # close every disconnector
opctl all open                         # every operable point
opctl -dry-run type:brytare open       # show what would be done
```

With `-list` the tool prints a table of the known breakers, disconnectors and pumps, which device each belongs to, which protocols can operate it and its command tag. The type is guessed from the tag id and name (the configuration has no explicit type). An optional filter (`brytare`, `frånskiljare` or `pump`) narrows the list. The same types apply to the per-type manoeuvre. When operating per type, any `-host`/`-port`/`-unit` overrides apply to every point, so use them only against a single device.

| Flag | Default | Description |
|---|---|---|
| `-list` | `false` | List breakers, disconnectors and pumps (optional filter as argument) |
| `-proxy` | – | Address of a blue-proxy for freeze/block actions (see below) |
| `-category` | – | Apply the action to a whole type, both when operating and with `-proxy` (must precede the state/action) |
| `-protocol` | `modbus` | `modbus` or `iec104` |
| `-data` | `data` | Data directory or path directly to `config.json` |
| `-timeout` | `5s` | Timeout per call |
| `-host` | – | Override the IP address/hostname from the configuration |
| `-port` | – | Override the port from the configuration (Modbus or IEC 104 depending on `-protocol`) |
| `-open-value` / `-close-value` | – | Override the Modbus value written (default: coil 0/1, holding 1/2) |
| `-unit` | – | Override the unit id from the configuration |
| `-verify` | `true` | Read the position tag back after the operation (Modbus) and wait until the position is reached |
| `-dry-run` | `false` | Show what would be done without connecting |

**Where the information comes from:**

- **Modbus:** the device's `host`/`port`/`unitId` and the command tag's area and address. A breaker or disconnector has a position tag and a command tag (`…​.Cmd` or `…_CMD`); the tool writes to the command tag. The defaults follow the simulators' conventions: coil 0 = open, 1 = close; holding register 1 = open, 2 = close. After the operation the position tag is read until the position is reached.
- **IEC 104:** the device's `iec104` block (`port`, `ca`) and the position tag's `ctrlIoa` and `cmd` (`C_DC` for breakers, `C_SC` for switches). The tool connects, performs STARTDT, sends the command with direct execute and waits for ACTCON and ACTTERM.

`iec104`, `ctrlIoa` and `cmd` in `config.json` are filled in by the generator scripts for the simulators (see above). The backend only polls over Modbus and ignores the fields. modsim has no IEC 104 interface, so the `sim` tags can only be operated over Modbus.

### Driving a blue-proxy

With `-proxy <url>` opctl instead uses the HTTP API of a [blue-proxy](../blue-proxy/README-en.md) to freeze reported values and block commands, just like the proxy's web interface.

```bash
opctl -proxy http://127.0.0.1:8083 status                 # show all points, mode and last command
opctl -proxy http://127.0.0.1:8083 freeze Grid.U          # freeze a tag
opctl -proxy http://127.0.0.1:8083 unfreeze Grid.U        # release
opctl -proxy http://127.0.0.1:8083 block Q0.CB            # block commands to a tag
opctl -proxy http://127.0.0.1:8083 unblock Q0.CB          # allow again
opctl -proxy http://127.0.0.1:8083 block type:brytare     # a whole type
opctl -proxy http://127.0.0.1:8083 unfreeze all           # every point
```

Actions: `freeze`, `unfreeze`, `block`, `unblock` and `status`. The target is a tag, `all` or `type:<type>` (e.g. `type:mätvärde`). Alternatively give `-category <type>` before the action.

## REST API and WebSocket

| Method | Path | Description |
|---|---|---|
| GET | `/api/config` | Devices and tags |
| GET/POST | `/api/devices` | List or create devices |
| PUT/DELETE | `/api/devices/{id}` | Update or delete a device |
| GET/POST | `/api/tags` | List or create tags |
| PUT/DELETE | `/api/tags/{id}` | Update or delete a tag |
| GET | `/api/values` | Snapshot of all values |
| GET | `/api/status` | Communication status per device |
| GET | `/api/history/{tag}?minutes=10` | History `[{t, v}]` |
| GET | `/api/events` | Event list, newest first |
| GET | `/api/alarms` | Alarm list, newest first |
| POST | `/api/alarms/ack` | Acknowledge alarms: `{"ids":["F1.CB"],"user":"operatör"}`. Without `ids`, all are acknowledged |
| POST | `/api/commands` | Operation: `{"tag":"Q1_CMD","value":1,"source":"Q1","text":"Frånslag"}` |
| GET/POST | `/api/views` | List or create views |
| GET/PUT/DELETE | `/api/views/{id}` | Read, save or delete a view |
| GET/POST | `/api/images` | List or upload images (multipart, field `file`) |
| WS | `/ws` | First a `snapshot` with all values and devices, and alarms, then `values`, `device`, `event` and `alarms` (the whole alarm list) on changes |

Values have the form `{"tag": "...", "v": 10.5, "q": "good" | "bad", "ts": "..."}`. Booleans are sent as 0 and 1.

## Limitations

- There is no login or access control, and all operations are logged as "operatör" (operator). Add authentication before the system is used with real equipment.
- History is kept in memory only, 1 hour per tag at 1 s polling. Longer storage requires a time-series database.
- The alarm list is kept in memory only and is emptied when the server restarts. Limits on measurement symbols only change the colour and raise no alarms.
- Modbus TCP and IEC 60870-5-104 are both supported as client protocols (per device). The IEC 104 poller performs STARTDT and a general interrogation on connect, receives spontaneous changes and reconnects automatically. Monitoring types M_SP, M_DP, M_ST and M_ME (normalized, scaled and short float, with and without time tag) are decoded; commands are sent as C_SC, C_DC, C_RC or C_SE_NC with direct execute.
