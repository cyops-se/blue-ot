[Svenska](README.md) · **English**

# Blue CHP – combined heat and power plant simulator

A combined heat and power (CHP) plant written in Go (standard library only). It consists of two boilers, a back-pressure steam turbine with generator and a district heating network with a heat accumulator. The generator is connected to [blue-station](../blue-station/README-en.md) in bay G1 on section B of the 20 kV busbar. The plant can be monitored and controlled via **Modbus TCP** and **IEC 60870-5-104** and has a web interface, just like blue-station.

```
   [P1 wood chips 120 MW]  [P2 bio-oil 50 MW]     boilers
          |                   |
   =======+===================+=======        steam header 90 bar
          |                        \
      [turbine]--(G)--[Gen.CB]------ 20 kV --> blue-station bay G1
          |                          \
      condenser <------------------ bypass (pressure reducing valve)
          |
   district heating <==> heat accumulator 400 MWh
     [pump 1] [pump 2]
```

## Running

```sh
go run ./cmd/blue-chp                    # Modbus :1502, IEC 104 :2405, web and link :8082
go run ./cmd/blue-chp -timescale 60      # the daily profile runs 60 times faster
go run ./cmd/blue-chp -points            # print the signal list
go test ./...
```

| Flag | Default | Description |
|---|---|---|
| `-modbus` | `:1502` | Listen address for Modbus TCP (empty = off) |
| `-iec104` | `:2405` | Listen address for IEC 104 (empty = off) |
| `-http` | `:8082` | Web interface, JSON API and link to blue-station (empty = off) |
| `-ca` | `2` | IEC 104 common address (CA) |
| `-iec-utc` | `false` | Timestamps (CP56Time2a) in UTC instead of local time |
| `-tick` | `1s` | Simulation time step |
| `-timescale` | `1` | Speed of the daily clock that drives the heat demand |
| `-points`, `-points-json` | | Print the signal list as Markdown or JSON and exit |

The ports are chosen so that blue-chp and blue-station can run on the same machine.

## Link to blue-station

The link represents the 20 kV cable between the plant and the substation. It is not a SCADA interface. Once per second blue-station calls `POST /api/link` with the state of the connection point:

```json
{"busLive": true, "busKV": 20.6, "hz": 50.01, "bayClosed": true}
```

The response is the generator output, which the substation adds to the load flow of section B:

```json
{"genOnline": true, "p": 26.0, "q": 4.0}
```

- If the link is missing or has been silent for more than 5 s, the plant runs stand-alone against a stiff grid (20.5 kV, 50 Hz). `Net.Link` then shows FRISTÅENDE (stand-alone).
- The generator cannot energise a dead busbar (no island operation). If bay G1 is opened or section B loses voltage, the loss-of-mains protection (LOM) trips the generator breaker.
- blue-station connects to `http://127.0.0.1:8082` by default. Use `-chp ""` to run the substation without CHP.

## What is simulated

- **Boilers:**
  - P1 burns wood chips and has 120 MW fuel input and 88 % efficiency. P2 burns bio-oil and has 50 MW and 90 %.
  - On start the boiler ignites and reaches minimum load (35 %) in 2 minutes. After that the load changes at 0.5 %/s.
  - Steam production follows the fuel with a time constant of 45 s.
  - Each boiler shows fuel flow, steam flow, live steam temperature, O₂ and flue gas temperature.
- **Load control:** boilers in AUTO share the steam demand. The demand is the heat demand plus a correction towards the accumulator setpoint, converted via the turbine's electrical efficiency when the generator is connected. In MANUAL the boiler follows its load setpoint.
- **Steam header:**
  - The turbine runs in inlet pressure control and takes all the steam it can, up to 115 MW. The bypass opens 3 bar above the setpoint.
  - An alarm is raised at 97 bar. Above 102 bar the boilers trip, and the safety valve opens at 105 bar.
- **Turbine and generator:**
  - The back-pressure turbine converts 30 % of the steam energy into electricity and the rest into district heating, up to just over 33 MW electric.
  - Run-up to 3000 rpm takes 60 s. The generator breaker can only be closed at rated speed and with voltage at the connection point (synchronisation).
  - Reactive power follows a setpoint.
  - The turbine trips at steam pressure below 50 bar and on emergency stop.
- **District heating network:**
  - The heat demand depends on the outdoor temperature (15 MW + 3.2 MW per degree below 17 °C) and a daily profile with morning and evening peaks.
  - The supply temperature setpoint follows an outdoor temperature curve between 75 and 110 °C.
  - The flow gives the return temperature. Two pumps provide 550 kg/s each, and too little pump capacity gives low differential pressure and reduced delivery.
- **Heat accumulator:** 400 MWh and ±60 MW. It absorbs the difference between production and delivery and holds the supply temperature. When it is full or empty the temperature deviates.
- **Trips:**
  - Boiler on fuel feed or burner fault and on high steam pressure.
  - Turbine on low pressure and on emergency stop.
  - Generator on loss of mains.
  - A trip blocks starting or closing until `Plant.Reset` has been executed.

### Suggested scenarios

1. Set `P1.FaultSim` = FEL (fault). P1 trips. Start P2 (`P2.Run`) and watch the accumulator discharge while P2 ramps up.
2. Execute the turbine emergency stop (`Turbine.EmergencyStop`). The bypass takes over, electricity production stops and heat delivery continues. In blue-station the infeed in G1 drops to 0 and T2 takes the whole load.
3. Do as in scenario 2 but first set `Bypass.Auto` = MANUELL (manual, closed). The pressure rises and the boilers trip on high steam pressure.
4. Open `G1.CB` in blue-station. The loss-of-mains protection trips the generator. Close G1, execute `Plant.Reset` and synchronise with `Gen.CB`.
5. Lower the outdoor temperature (`Env.Outdoor` = −20 °C). The heat demand increases, P1 reaches full load and P2 needs to be started.
6. Stop one district heating pump at high heat demand. The differential pressure drops and delivery is limited.

## Modbus TCP and IEC 104

The interfaces are the same as in blue-station:

- **Modbus:** function codes 1, 2, 3, 4, 5, 15 and 16, float32 ABCD, and setpoints are written with FC16.
- **IEC 104:** general interrogation, spontaneous changes with timestamp, and commands with direct execute or select/execute.

The addresses follow the same layout: breakers from coil 0 / IOA 1001, switches from coil 20 / IOA 2101, pulse commands from coil 40 / IOA 6001, status from DI 100 / IOA 2001, measurements from IR 0 / IOA 3001, and setpoints from HR 0 / IOA 4101 (control 8101).

## Web interface and API

`http://localhost:8082` shows all signals grouped, with controls and an event log. The interface and the event texts are in Swedish.

- `GET /api/state` returns all points and events.
- `POST /api/control` accepts `{"id": "P2.Run", "on": true}` or `{"id": "Env.Outdoor", "value": -15}`.
- `POST /api/link` is the link to blue-station (see above).

## Signal list

The descriptions are translated here. In the simulator itself they are in Swedish.

| ID | Description | Unit | Modbus TCP | IEC 104 monitoring | IEC 104 control |
|---|---|---|---|---|---|
| `P1.Run` | P1 start/stop | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `P1.Auto` | P1 load control | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `P1.Running` | P1 in operation | – | DI 100 | M_SP 2001 | – |
| `P1.Trip` | P1 boiler protection tripped | – | DI 101 | M_SP 2002 | – |
| `P1.State` | P1 operating state (0 stopped, 1 starting, 2 running, 3 stopping) | – | IR 0–1 | M_ME_NC 3001 | – |
| `P1.Load` | P1 load | % | IR 2–3 | M_ME_NC 3002 | – |
| `P1.FuelPower` | P1 fuel input | MW | IR 4–5 | M_ME_NC 3003 | – |
| `P1.FuelFlow` | P1 fuel flow | t/h | IR 6–7 | M_ME_NC 3004 | – |
| `P1.SteamFlow` | P1 steam flow | t/h | IR 8–9 | M_ME_NC 3005 | – |
| `P1.SteamTemp` | P1 live steam temperature | °C | IR 10–11 | M_ME_NC 3006 | – |
| `P1.O2` | P1 flue gas oxygen | % | IR 12–13 | M_ME_NC 3007 | – |
| `P1.FlueTemp` | P1 flue gas temperature | °C | IR 14–15 | M_ME_NC 3008 | – |
| `P1.LoadSet` | P1 load setpoint (manual) | % | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `P1.FaultSim` | Simulation: fuel feed fault P1 | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `P2.Run` | P2 start/stop | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `P2.Auto` | P2 load control | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `P2.Running` | P2 in operation | – | DI 102 | M_SP 2003 | – |
| `P2.Trip` | P2 boiler protection tripped | – | DI 103 | M_SP 2004 | – |
| `P2.State` | P2 operating state (0 stopped, 1 starting, 2 running, 3 stopping) | – | IR 16–17 | M_ME_NC 3009 | – |
| `P2.Load` | P2 load | % | IR 18–19 | M_ME_NC 3010 | – |
| `P2.FuelPower` | P2 fuel input | MW | IR 20–21 | M_ME_NC 3011 | – |
| `P2.FuelFlow` | P2 fuel flow | t/h | IR 22–23 | M_ME_NC 3012 | – |
| `P2.SteamFlow` | P2 steam flow | t/h | IR 24–25 | M_ME_NC 3013 | – |
| `P2.SteamTemp` | P2 live steam temperature | °C | IR 26–27 | M_ME_NC 3014 | – |
| `P2.O2` | P2 flue gas oxygen | % | IR 28–29 | M_ME_NC 3015 | – |
| `P2.FlueTemp` | P2 flue gas temperature | °C | IR 30–31 | M_ME_NC 3016 | – |
| `P2.LoadSet` | P2 load setpoint (manual) | % | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `P2.FaultSim` | Simulation: burner fault P2 | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `Steam.Pressure` | Steam header pressure | bar | IR 32–33 | M_ME_NC 3017 | – |
| `Steam.Flow` | Total steam production | t/h | IR 34–35 | M_ME_NC 3018 | – |
| `Steam.HighPressure` | High steam pressure (>97 bar) | – | DI 104 | M_SP 2005 | – |
| `Steam.PressureSet` | Steam pressure setpoint | bar | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `Bypass.Auto` | Bypass valve control | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `Bypass.Position` | Bypass valve position | % | IR 36–37 | M_ME_NC 3019 | – |
| `Bypass.Flow` | Bypass steam flow | t/h | IR 38–39 | M_ME_NC 3020 | – |
| `Bypass.ManualPos` | Bypass valve position in manual | % | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `Turbine.Run` | Turbine start/stop | – | Coil 27, DI 27 | M_SP 2108 | C_SC 6108 |
| `Turbine.Speed` | Turbine speed | rpm | IR 40–41 | M_ME_NC 3021 | – |
| `Turbine.SteamFlow` | Turbine steam flow | t/h | IR 42–43 | M_ME_NC 3022 | – |
| `Turbine.AtSpeed` | Turbine at rated speed | – | DI 105 | M_SP 2006 | – |
| `Turbine.Trip` | Turbine protection tripped | – | DI 106 | M_SP 2007 | – |
| `Turbine.EmergencyStop` | Turbine emergency stop | – | Coil 40 | – | C_SC 6001 |
| `Gen.CB` | Generator breaker (synchronises on close) | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Gen.P` | Generator active power | MW | IR 44–45 | M_ME_NC 3023 | – |
| `Gen.Q` | Generator reactive power | Mvar | IR 46–47 | M_ME_NC 3024 | – |
| `Gen.U` | Generator voltage | kV | IR 48–49 | M_ME_NC 3025 | – |
| `Gen.f` | Generator frequency | Hz | IR 50–51 | M_ME_NC 3026 | – |
| `Gen.I` | Generator current | A | IR 52–53 | M_ME_NC 3027 | – |
| `Gen.Energy` | Electrical energy produced | MWh | IR 54–55 | M_ME_NC 3028 | – |
| `Gen.Trip` | Generator protection tripped (loss of mains) | – | DI 107 | M_SP 2008 | – |
| `Gen.QSet` | Reactive power setpoint | Mvar | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `Net.Link` | Link to blue-station | – | DI 108 | M_SP 2009 | – |
| `Net.Live` | Voltage at the connection point | – | DI 109 | M_SP 2010 | – |
| `Net.U` | Connection point voltage | kV | IR 56–57 | M_ME_NC 3029 | – |
| `Net.f` | Connection point frequency | Hz | IR 58–59 | M_ME_NC 3030 | – |
| `DH.Demand` | Network heat demand (incl. losses) | MW | IR 60–61 | M_ME_NC 3031 | – |
| `DH.Production` | Heat production (condenser + bypass) | MW | IR 62–63 | M_ME_NC 3032 | – |
| `DH.Delivered` | Heat delivered to the network | MW | IR 64–65 | M_ME_NC 3033 | – |
| `DH.SupplyTemp` | Supply temperature | °C | IR 66–67 | M_ME_NC 3034 | – |
| `DH.SupplyTempSet` | Supply temperature setpoint (outdoor temperature curve) | °C | IR 68–69 | M_ME_NC 3035 | – |
| `DH.ReturnTemp` | Return temperature | °C | IR 70–71 | M_ME_NC 3036 | – |
| `DH.Flow` | District heating flow | m³/h | IR 72–73 | M_ME_NC 3037 | – |
| `DH.DiffPressure` | Differential pressure at critical customer | bar | IR 74–75 | M_ME_NC 3038 | – |
| `DH.Pump1` | District heating pump 1 | – | Coil 28, DI 28 | M_SP 2109 | C_SC 6109 |
| `DH.Pump2` | District heating pump 2 | – | Coil 29, DI 29 | M_SP 2110 | C_SC 6110 |
| `DH.LowDP` | Low network differential pressure (<0.6 bar) | – | DI 110 | M_SP 2011 | – |
| `DH.SupplyLow` | Low supply temperature (>8 °C below setpoint) | – | DI 111 | M_SP 2012 | – |
| `DH.SupplyHigh` | High supply temperature (>120 °C) | – | DI 112 | M_SP 2013 | – |
| `Acc.Energy` | Stored energy | MWh | IR 76–77 | M_ME_NC 3039 | – |
| `Acc.Level` | State of charge | % | IR 78–79 | M_ME_NC 3040 | – |
| `Acc.Power` | Charging power (+ charging, − discharging) | MW | IR 80–81 | M_ME_NC 3041 | – |
| `Acc.LevelSet` | State of charge setpoint | % | HR 10–11 | M_ME_NC 4106 | C_SE_NC 8106 |
| `Env.Outdoor` | Simulation: outdoor temperature | °C | HR 12–13 | M_ME_NC 4107 | C_SE_NC 8107 |
| `Heat.Scale` | Simulation: heat demand scaling | % | HR 14–15 | M_ME_NC 4108 | C_SE_NC 8108 |
| `Plant.CommonAlarm` | Common alarm | – | DI 113 | M_SP 2014 | – |
| `Plant.Reset` | Reset trips and lockouts | – | Coil 41 | – | C_SC 6002 |

## Structure

```
cmd/blue-chp/      main, flags
internal/plant/    process model: boilers, steam header, turbine, generator, district heating, signal list
internal/modbus/   Modbus TCP server
internal/iec104/   IEC 60870-5-104 server (slave)
internal/web/      web interface, JSON API and link to blue-station
```
