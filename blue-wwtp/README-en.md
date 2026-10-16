[Svenska](README.md) · **English**

# Blue WWTP – wastewater treatment plant simulator

A municipal wastewater treatment plant written in Go (standard library only). Sewage and stormwater are lifted by a level-controlled inlet pumping station and treated with activated sludge, aeration and chemical precipitation. The sludge is digested, and the biogas heats the digester. The plant has **Modbus TCP**, **IEC 60870-5-104** and a web interface, just like [blue-station](../blue-station/README-en.md) and [blue-chp](../blue-chp/README-en.md).

```
  sewer ─► wet well ─[IP1–IP3]─► screen ─► activated sludge ─► precipitation ─► recipient
              │                                [B1/B2]
          overflow                  sludge ─► digester ─► biogas ─► gas boiler / flare
```

## Running

```sh
go run ./cmd/blue-wwtp                    # Modbus :1504, IEC 104 :2407, web :8086
go run ./cmd/blue-wwtp -timescale 60      # the daily profile runs 60 times faster
go run ./cmd/blue-wwtp -points            # the signal list (Markdown; -points-json gives JSON)
go test ./...
```

| Flag | Default | Description |
|---|---|---|
| `-modbus` | `:1504` | Modbus TCP listen address (empty = off) |
| `-iec104` | `:2407` | IEC 104 listen address (empty = off) |
| `-http` | `:8086` | Web interface and JSON API (empty = off) |
| `-ca` | `4` | IEC 104 common address (CA) |
| `-iec-utc` | `false` | Time stamps in UTC instead of local time |
| `-tick` | `1s` | Simulation time step |
| `-timescale` | `1` | Speed of the daily clock |

## What is simulated

- **Inflow:** sewage with a daily profile (about 180 l/s on average) plus 25 l/s per mm/h of rain. The rainwater dilutes the incoming ammonium and phosphorus.
- **Inlet pumping station:** three pumps of 220 l/s. In AUTO they start and stop on the wet well level. `Run` works as a duty selection: in AUTO only started pumps are available, in MANUAL they run. When the level reaches 3.8 m the wet well overflows, and the overflow volume is totalized.
- **Screen:** a blocked screen (`Screen.FaultSim`) halves the flow to the pumps.
- **Activated sludge:** two blowers of 3000 Nm³/h. In AUTO the air is controlled to keep the dissolved oxygen at its setpoint. Nitrification depends on oxygen and water temperature, so low oxygen or cold water gives high effluent ammonium.
- **Chemical precipitation:** in AUTO the precipitant is dosed after the incoming phosphorus.
- **Effluent:** NH4-N, Ptot and BOD7, with alarms above the limits 5 mg/l NH4 and 0.3 mg/l Ptot.
- **Digestion:** the gas boiler keeps the digester at 37 °C using its own biogas. A cold digester produces less gas. The gas holder holds 1500 Nm³, and excess gas is flared.
- **Power supply and protection:** everything is fed through `Supply.CB`. Pumps and blowers have motor protection (`xx.FaultSim`) that requires `Plant.Reset`.

### Suggested scenarios

1. Set `Rain.Intensity` = 30. All three pumps run and the wet well overflows.
2. Set `Screen.FaultSim` = FAULT during moderate rain. The overflow starts earlier.
3. Stop `B2.Run` and set `B1.FaultSim` = FAULT. The oxygen drops and effluent ammonium rises.
4. Lower `Water.Temp` to 4 °C. Nitrification slows down and ammonium alarms.
5. Set `Prec.Auto` = MANUAL with a low dose. Effluent phosphorus alarms.
6. Stop `Boiler.Run`. The digester cools down and gas production falls.

## Modbus TCP, IEC 104 and web

The interfaces and addressing are the same as in the other simulators. `http://localhost:8086` shows all signals with controls and an event log. `GET /api/state` returns points and events, and `POST /api/control` accepts `{"id": "B2.Run", "on": true}` or `{"id": "Bio.DOSet", "value": 2.5}`.

## Signal list

The descriptions are translated here. The simulator itself reports them in Swedish.

| ID | Description | Unit | Modbus TCP | IEC 104 monitoring | IEC 104 control |
|---|---|---|---|---|---|
| `Supply.CB` | Incoming breaker (power supply) | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Power.Available` | Simulation: external power supply | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `Power.Lost` | Plant de-energized | – | DI 100 | M_SP 2001 | – |
| `Power.Total` | Total electrical power | kW | IR 0–1 | M_ME_NC 3001 | – |
| `Power.Energy` | Electrical energy consumed | MWh | IR 2–3 | M_ME_NC 3002 | – |
| `Inflow.Flow` | Inflow from the sewer network | l/s | IR 4–5 | M_ME_NC 3003 | – |
| `Well.Level` | Wet well level | m | IR 6–7 | M_ME_NC 3004 | – |
| `Well.High` | High wet well level (>3.3 m) | – | DI 101 | M_SP 2002 | – |
| `Inlet.Auto` | Level control of inlet pumps | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `IP1.Run` | Inlet pump 1 duty (MANUAL: run, AUTO: available) | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `IP1.Running` | Inlet pump 1 running | – | DI 102 | M_SP 2003 | – |
| `IP1.Trip` | Inlet pump 1 motor protection tripped | – | DI 103 | M_SP 2004 | – |
| `IP1.Flow` | Inlet pump 1 flow | l/s | IR 8–9 | M_ME_NC 3005 | – |
| `IP1.Power` | Inlet pump 1 power | kW | IR 10–11 | M_ME_NC 3006 | – |
| `IP1.FaultSim` | Simulation: motor fault IP1 | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `IP2.Run` | Inlet pump 2 duty (MANUAL: run, AUTO: available) | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `IP2.Running` | Inlet pump 2 running | – | DI 104 | M_SP 2005 | – |
| `IP2.Trip` | Inlet pump 2 motor protection tripped | – | DI 105 | M_SP 2006 | – |
| `IP2.Flow` | Inlet pump 2 flow | l/s | IR 12–13 | M_ME_NC 3007 | – |
| `IP2.Power` | Inlet pump 2 power | kW | IR 14–15 | M_ME_NC 3008 | – |
| `IP2.FaultSim` | Simulation: motor fault IP2 | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `IP3.Run` | Inlet pump 3 duty (MANUAL: run, AUTO: available) | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `IP3.Running` | Inlet pump 3 running | – | DI 106 | M_SP 2007 | – |
| `IP3.Trip` | Inlet pump 3 motor protection tripped | – | DI 107 | M_SP 2008 | – |
| `IP3.Flow` | Inlet pump 3 flow | l/s | IR 16–17 | M_ME_NC 3009 | – |
| `IP3.Power` | Inlet pump 3 power | kW | IR 18–19 | M_ME_NC 3010 | – |
| `IP3.FaultSim` | Simulation: motor fault IP3 | – | Coil 27, DI 27 | M_SP 2108 | C_SC 6108 |
| `Pumped.Flow` | Flow pumped to treatment | l/s | IR 20–21 | M_ME_NC 3011 | – |
| `Overflow.Active` | Overflow in progress | – | DI 108 | M_SP 2009 | – |
| `Overflow.Flow` | Overflow rate | l/s | IR 22–23 | M_ME_NC 3012 | – |
| `Overflow.Volume` | Overflow volume (totalized) | m³ | IR 24–25 | M_ME_NC 3013 | – |
| `Screen.Running` | Screen running | – | DI 109 | M_SP 2010 | – |
| `Screen.Blocked` | Screen blocked | – | DI 110 | M_SP 2011 | – |
| `Screen.FaultSim` | Simulation: blocked screen | – | Coil 28, DI 28 | M_SP 2109 | C_SC 6109 |
| `Blower.Auto` | Oxygen control of blowers | – | Coil 29, DI 29 | M_SP 2110 | C_SC 6110 |
| `B1.Run` | Blower 1 duty (MANUAL: run, AUTO: available) | – | Coil 30, DI 30 | M_SP 2111 | C_SC 6111 |
| `B1.Running` | Blower 1 running | – | DI 111 | M_SP 2012 | – |
| `B1.Trip` | Blower 1 motor protection tripped | – | DI 112 | M_SP 2013 | – |
| `B1.Air` | Blower 1 air flow | Nm³/h | IR 26–27 | M_ME_NC 3014 | – |
| `B1.Power` | Blower 1 power | kW | IR 28–29 | M_ME_NC 3015 | – |
| `B1.FaultSim` | Simulation: motor fault B1 | – | Coil 31, DI 31 | M_SP 2112 | C_SC 6112 |
| `B2.Run` | Blower 2 duty (MANUAL: run, AUTO: available) | – | Coil 32, DI 32 | M_SP 2113 | C_SC 6113 |
| `B2.Running` | Blower 2 running | – | DI 113 | M_SP 2014 | – |
| `B2.Trip` | Blower 2 motor protection tripped | – | DI 114 | M_SP 2015 | – |
| `B2.Air` | Blower 2 air flow | Nm³/h | IR 30–31 | M_ME_NC 3016 | – |
| `B2.Power` | Blower 2 power | kW | IR 32–33 | M_ME_NC 3017 | – |
| `B2.FaultSim` | Simulation: motor fault B2 | – | Coil 33, DI 33 | M_SP 2114 | C_SC 6114 |
| `Air.Flow` | Total air flow | Nm³/h | IR 34–35 | M_ME_NC 3018 | – |
| `Bio.Flow` | Flow through the basins | l/s | IR 36–37 | M_ME_NC 3019 | – |
| `Bio.DO` | Dissolved oxygen in aeration basin | mg/l | IR 38–39 | M_ME_NC 3020 | – |
| `Bio.DOSet` | Dissolved oxygen setpoint | mg/l | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `Bio.DOLow` | Low oxygen (<1.0 mg/l) | – | DI 115 | M_SP 2016 | – |
| `Prec.Auto` | Precipitant dosing | – | Coil 34, DI 34 | M_SP 2115 | C_SC 6115 |
| `Prec.Dose` | Precipitant dose | mg/l | IR 40–41 | M_ME_NC 3021 | – |
| `Prec.DoseSet` | Precipitant dose in manual | mg/l | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `In.NH4` | Influent ammonium nitrogen | mg/l | IR 42–43 | M_ME_NC 3022 | – |
| `In.Ptot` | Influent total phosphorus | mg/l | IR 44–45 | M_ME_NC 3023 | – |
| `Eff.Flow` | Effluent flow to recipient | l/s | IR 46–47 | M_ME_NC 3024 | – |
| `Eff.NH4` | Effluent ammonium nitrogen | mg/l | IR 48–49 | M_ME_NC 3025 | – |
| `Eff.Ptot` | Effluent total phosphorus | mg/l | IR 50–51 | M_ME_NC 3026 | – |
| `Eff.BOD7` | Effluent BOD7 | mg/l | IR 52–53 | M_ME_NC 3027 | – |
| `Eff.NH4High` | Ammonium above limit (>5 mg/l) | – | DI 116 | M_SP 2017 | – |
| `Eff.PtotHigh` | Phosphorus above limit (>0.3 mg/l) | – | DI 117 | M_SP 2018 | – |
| `Digester.Temp` | Digester temperature | °C | IR 54–55 | M_ME_NC 3028 | – |
| `Digester.TempLow` | Low digester temperature (<33 °C) | – | DI 118 | M_SP 2019 | – |
| `Boiler.Run` | Gas boiler (heats the digester) | – | Coil 35, DI 35 | M_SP 2116 | C_SC 6116 |
| `Biogas.Flow` | Biogas production | Nm³/h | IR 56–57 | M_ME_NC 3029 | – |
| `Gasholder.Level` | Gas holder level | % | IR 58–59 | M_ME_NC 3030 | – |
| `Gas.Flaring` | Flare burning (excess gas) | – | DI 119 | M_SP 2020 | – |
| `Rain.Intensity` | Simulation: rain intensity | mm/h | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `Load.Scale` | Simulation: sewage load scaling | % | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `Water.Temp` | Simulation: water temperature | °C | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `Plant.CommonAlarm` | Common alarm | – | DI 120 | M_SP 2021 | – |
| `Plant.Reset` | Reset trips | – | Coil 40 | – | C_SC 6001 |

## Layout

```
cmd/blue-wwtp/     main, flags
internal/plant/    process model (wwtp.go), signal list (wwtp_points.go), shared core (core.go)
internal/modbus/   Modbus TCP server
internal/iec104/   IEC 60870-5-104 server (slave)
internal/web/      web interface and JSON API
```
