[Svenska](README.md) · **English**

# Blue OT SCADA

The SCADA part of [Blue OT](../README-en.md) consists of a Go backend that polls Modbus TCP devices and a web-based HMI built with Vue 3 and Vuetify 3. The HMI is embedded in the backend executable, so the same program serves the REST API, the WebSocket and the web interface. The project also includes **modsim**, a small Modbus simulator for testing the system without other equipment.

What it includes:

- **Process displays** with static graphics and dynamic symbols: breakers, disconnectors, switches, measurements, bars, indicators, buttons and view links. Values update in real time over WebSocket.
- **Operation** of breakers, disconnectors, switches and setpoints. Each operation requires a selection and then a confirmation (select-before-operate), and the dialog shows when the position has changed.
- **View editor** in the browser. There you draw displays and bind each symbol to the tags and values it should show.
- **Measurement table** with floating-point values, quality and timestamp, a trend per tag, and direct writes to writable tags.
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
| `backend/internal/mbus` | One poller per device. It merges nearby addresses into few reads, decodes data types, reconnects on errors and writes for operations |
| `backend/internal/realtime` | Process image, in-memory history (3600 values per tag), event list (last 1000) and fan-out to WebSocket clients |
| `backend/internal/store` | JSON storage of devices, tags, views and uploaded images |
| `backend/internal/api` | REST API and WebSocket |
| `backend/internal/web` | Embedded HMI (`go:embed` of `dist/`, written by the frontend build) |
| `backend/tools` | Scripts that generate devices, tags and views for blue-station and blue-chp |
| `frontend/` | HMI in Vue 3, Vuetify 3, Pinia and Vite |

## Building and running

Requirements: Go 1.21 or later and Node 18 or later.

```bash
cd frontend
npm install
npm run build                    # writes to backend/internal/web/dist

cd ../backend
go build -o bin/ ./cmd/scada ./cmd/modsim
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

- **Breakers, disconnectors and switches:** click to open the operation dialog. Select *Frånslag/Tillslag* (open/close; or *Öppna/Slut*, *PÅ/AV*) and confirm with *Utför* (execute). The dialog waits for the position to change and then closes. If the device rejects the operation, e.g. because of an interlock or lockout, the error is shown in the dialog.
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
| Measurement | Tag, label, unit and decimals (taken from the tag by default), limits with alarm colour |
| Indicator | Tag, active value, colours, blinking |
| Bar | Tag, min and max. Fills horizontally if the bar is wider than it is tall |
| Command button | Writable tag and value. With "Fråga efter värde" (ask for value) it works as a setpoint input |
| View link | Navigation to another view |

### Measurements, events and configuration

- **Measurements (`/values`):** all tags with value, quality and time of last change. Click a row to see the trend. Writable tags have a button for writing a value.
- **Events (`/events`):** filtered by operations, position changes, communication and system. Position changes are logged for tags with "Logga ändringar" (log changes) and are shown with the tag's state texts, e.g. "Till → Från".
- **Configuration (`/config`):**
  - **Devices:** IP address and port, unit id, poll interval, timeout and enabled/disabled.
  - **Tags:** area (coil, discrete, input, holding), address (0-based), data type (`bool`, `uint16`, `int16`, `uint32`, `int32`, `float32`, `float64`), word and byte swap, scale and offset, bit in register, writable, event logging and state texts.
  - Changes are saved to `data/config.json`, and the poller restarts immediately.

## Included devices and views

| View | Device | Address | Tags | Description |
|---|---|---|---|---|
| Station Norra | `sim` | 127.0.0.1:5020 | 31 | 10 kV switchgear with four bays (modsim) |
| Blue Station | `blue-station` | 127.0.0.1:502 | 107 | 220/20 kV transformer substation with bay G1 for CHP, see [blue-station](../blue-station/README-en.md) |
| Blue CHP | `blue-chp` | 127.0.0.1:1502 | 87 (prefix `CHP.`) | CHP plant with district heating, see [blue-chp](../blue-chp/README-en.md) |

Tags and views for blue-station and blue-chp are generated by scripts in `backend/tools/`. Run the scripts in `backend/`:

```bash
python tools/gen_blue_station.py   # device blue-station, tags and the Blue Station view
python tools/gen_blue_chp.py       # device blue-chp, tags and the Blue CHP view (reads the signal list via go run … -points-json)
```

The scripts replace the tags of their device in `config.json` and overwrite the view file. Changes you have made to those views in the editor are therefore lost if you run the script again.

Tag conventions for the simulators:

- Breakers and switches have two tags: the position tag reads a discrete input (`F1.CB`) and the control tag writes a coil (`F1.CB.Cmd`).
- Pulse commands, e.g. `Station.Reset` and `CHP.Turbine.EmergencyStop`, are coils written with 1.
- Measurements and setpoints are float32 (ABCD) in input and holding registers respectively.

## modsim

`modsim` simulates a 10 kV switchgear with four bays. Each bay has a breaker Q1–Q4, a disconnector QS1–QS4 and measurements of U, I and P. There are also frequency, busbar voltage and a transformer oil temperature. The simulator shows two common ways of representing switching devices in Modbus:

- **Breakers:** the position is a double-point value in a holding register (0 = intermediate, 1 = open, 2 = closed, 3 = faulty), and the command is given by writing a command register.
- **Disconnectors:** the position is a discrete input, and the command is given through a coil.

The disconnectors are interlocked: they can only be operated when the bay's breaker is open. Otherwise the simulator replies with the exception *illegal data value*.

```bash
go run ./cmd/modsim                         # listens on 0.0.0.0:5020
go run ./cmd/modsim -addr :1503 -delay 3s   # other port, breaker operating time 3 s
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
| POST | `/api/commands` | Operation: `{"tag":"Q1_CMD","value":1,"source":"Q1","text":"Frånslag"}` |
| GET/POST | `/api/views` | List or create views |
| GET/PUT/DELETE | `/api/views/{id}` | Read, save or delete a view |
| GET/POST | `/api/images` | List or upload images (multipart, field `file`) |
| WS | `/ws` | First a `snapshot` with all values and devices, then `values`, `device` and `event` on changes |

Values have the form `{"tag": "...", "v": 10.5, "q": "good" | "bad", "ts": "..."}`. Booleans are sent as 0 and 1.

## Limitations

- There is no login or access control, and all operations are logged as "operatör" (operator). Add authentication before the system is used with real equipment.
- History is kept in memory only, 1 hour per tag at 1 s polling. Longer storage requires a time-series database.
- There is no alarm handling with acknowledgement. Limits on measurement symbols only change the colour.
- Only Modbus TCP is supported as a client protocol. The simulators also offer IEC 60870-5-104, but SCADA does not use it.
