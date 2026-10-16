[Svenska](README.md) · **English**

# Blue Water – drinking water treatment plant simulator

A drinking water treatment plant written in Go (standard library only). Raw water is pumped from a lake, treated by coagulation, settling and sand filtration, disinfected with chlorine and stored in a clear water reservoir. High-lift pumps keep the distribution network pressure while consumption follows a daily profile. The plant can be monitored and controlled via **Modbus TCP** and **IEC 60870-5-104** and has a web interface, just like [blue-station](../blue-station/README-en.md) and [blue-chp](../blue-chp/README-en.md).

```
  lake ─[RP1/RP2]─► coagulation ─► settling ─► [F1/F2 sand filters] ─► chlorine
                                                                        │
       network ◄─[outlet valve]◄─[HP1/HP2]◄─ clear water reservoir 8000 m³
                                                       ▲
                            [Supply.CB] power supply to the whole plant
```

## Running

```sh
go run ./cmd/blue-water                    # Modbus :1503, IEC 104 :2406, web :8085
go run ./cmd/blue-water -timescale 60      # the daily profile runs 60 times faster
go run ./cmd/blue-water -points            # print the signal list (Markdown; -points-json gives JSON)
go test ./...
```

| Flag | Default | Description |
|---|---|---|
| `-modbus` | `:1503` | Modbus TCP listen address (empty = off) |
| `-iec104` | `:2406` | IEC 104 listen address (empty = off) |
| `-http` | `:8085` | Web interface and JSON API (empty = off) |
| `-ca` | `3` | IEC 104 common address (CA) |
| `-iec-utc` | `false` | Time stamps (CP56Time2a) in UTC instead of local time |
| `-tick` | `1s` | Simulation time step |
| `-timescale` | `1` | Speed of the daily clock that drives consumption |

## What is simulated

- **Power supply:** the whole plant is fed through the breaker `Supply.CB`. `Power.Available` simulates a loss of the external supply. Without power all pumps stop.
- **Raw water intake:** two pumps of 130 l/s each. In AUTO the raw water flow follows consumption, corrected towards the reservoir level setpoint. In MANUAL the started pumps run at full speed.
- **Coagulation and settling:** in AUTO the coagulant is dosed after the raw water turbidity (8 + 3 mg/l per NTU). Too low a dose gives poorer separation and higher turbidity after settling.
- **Sand filters:** the filter head loss rises with flow and turbidity. Above 2.2 m the filter alarms and starts to let particles through. Backwashing (`Fx.Backwash`) takes the filter out of service for 5 minutes, uses wash water from the reservoir and resets the head loss. Only one filter at a time can be backwashed.
- **Disinfection:** in AUTO chlorine is dosed to keep the residual chlorine at its setpoint. Chlorine demand rises with turbidity.
- **Reservoir:** 8000 m³ and 6 m deep. It is filled with treated water and drained by consumption and wash water.
- **Distribution:** two high-lift pumps of 160 l/s each keep the network pressure. If the capacity is not enough the pressure drops. With the outlet valve closed, an empty reservoir or no pumps running, the pressure falls to the static pressure of the water tower. Pump power is calculated from flow and pressure.
- **Protection:** each pump has motor protection (fault simulation with `xx.FaultSim`), and the high-lift pumps have dry-run protection at low reservoir level. A tripped pump does not start until `Plant.Reset` has been run.

### Suggested scenarios

1. Set `Raw.StormNTU` = 40 and watch the filter head loss rise until the filters alarm. Backwash F1 and then F2.
2. Set `Coag.Auto` = MANUAL with a low dose (`Coag.DoseSet` = 5) during a storm. The treated water turbidity alarms. Go back to AUTO and backwash the filters.
3. Stop `HP2.Run` and raise consumption (`Demand.Scale` = 250). One pump is not enough and the network pressure alarms.
4. Open `Supply.CB`. All pumps stop, the pressure falls and the reservoir stops filling.
5. Set `HP1.FaultSim` = FAULT. HP1 trips. Clear the fault, run `Plant.Reset` and watch the pump start again.
6. Close `Outlet.Valve`. Delivery stops and the network pressure falls.

## Modbus TCP and IEC 104

The interfaces and addressing are the same as in blue-station and blue-chp:

- **Modbus:** function codes 1, 2, 3, 4, 5, 15 and 16, float32 ABCD, and setpoints are written with FC16.
- **IEC 104:** general interrogation, spontaneous changes with time stamp, and commands with direct execute or select/execute.

Breakers from coil 0 / IOA 1001, switches from coil 20 / IOA 2101, pulse commands from coil 40 / IOA 6001, status from DI 100 / IOA 2001, measurements from IR 0 / IOA 3001 and setpoints from HR 0 / IOA 4101 (control 8101).

## Web interface and API

`http://localhost:8085` shows all signals grouped, with controls and an event log.

- `GET /api/state` returns all points and events.
- `POST /api/control` accepts `{"id": "HP2.Run", "on": false}` or `{"id": "Net.PressureSet", "value": 5}`.

## Signal list

The descriptions are translated here. The simulator itself reports them in Swedish.

| ID | Description | Unit | Modbus TCP | IEC 104 monitoring | IEC 104 control |
|---|---|---|---|---|---|
| `Supply.CB` | Incoming breaker (power supply) | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Power.Available` | Simulation: external power supply | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `Power.Lost` | Plant de-energized | – | DI 100 | M_SP 2001 | – |
| `Power.Total` | Total electrical power | kW | IR 0–1 | M_ME_NC 3001 | – |
| `Power.Energy` | Electrical energy consumed | MWh | IR 2–3 | M_ME_NC 3002 | – |
| `RP1.Run` | Raw water pump 1 start/stop | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `RP1.Running` | Raw water pump 1 running | – | DI 101 | M_SP 2002 | – |
| `RP1.Trip` | Raw water pump 1 motor protection tripped | – | DI 102 | M_SP 2003 | – |
| `RP1.Speed` | Raw water pump 1 speed | % | IR 4–5 | M_ME_NC 3003 | – |
| `RP1.Flow` | Raw water pump 1 flow | l/s | IR 6–7 | M_ME_NC 3004 | – |
| `RP1.Power` | Raw water pump 1 power | kW | IR 8–9 | M_ME_NC 3005 | – |
| `RP1.FaultSim` | Simulation: motor fault RP1 | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `RP2.Run` | Raw water pump 2 start/stop | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `RP2.Running` | Raw water pump 2 running | – | DI 103 | M_SP 2004 | – |
| `RP2.Trip` | Raw water pump 2 motor protection tripped | – | DI 104 | M_SP 2005 | – |
| `RP2.Speed` | Raw water pump 2 speed | % | IR 10–11 | M_ME_NC 3006 | – |
| `RP2.Flow` | Raw water pump 2 flow | l/s | IR 12–13 | M_ME_NC 3007 | – |
| `RP2.Power` | Raw water pump 2 power | kW | IR 14–15 | M_ME_NC 3008 | – |
| `RP2.FaultSim` | Simulation: motor fault RP2 | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `Raw.Auto` | Raw water flow controlled by reservoir level | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `Raw.Flow` | Raw water flow | l/s | IR 16–17 | M_ME_NC 3009 | – |
| `Raw.Turbidity` | Raw water turbidity | NTU | IR 18–19 | M_ME_NC 3010 | – |
| `Res.LevelSet` | Clear water reservoir level setpoint | m | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `Coag.Auto` | Coagulant dosing | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `Coag.Dose` | Coagulant dose | mg/l | IR 20–21 | M_ME_NC 3011 | – |
| `Coag.DoseSet` | Coagulant dose in manual | mg/l | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `Settled.Turbidity` | Turbidity after settling | NTU | IR 22–23 | M_ME_NC 3012 | – |
| `F1.Headloss` | F1 head loss | m | IR 24–25 | M_ME_NC 3013 | – |
| `F1.Flow` | F1 flow | l/s | IR 26–27 | M_ME_NC 3014 | – |
| `F1.InService` | F1 in service | – | DI 105 | M_SP 2006 | – |
| `F1.Clogged` | F1 clogged (>2.2 m) | – | DI 106 | M_SP 2007 | – |
| `F1.Backwash` | F1 start backwash | – | Coil 40 | – | C_SC 6001 |
| `F2.Headloss` | F2 head loss | m | IR 28–29 | M_ME_NC 3015 | – |
| `F2.Flow` | F2 flow | l/s | IR 30–31 | M_ME_NC 3016 | – |
| `F2.InService` | F2 in service | – | DI 107 | M_SP 2008 | – |
| `F2.Clogged` | F2 clogged (>2.2 m) | – | DI 108 | M_SP 2009 | – |
| `F2.Backwash` | F2 start backwash | – | Coil 41 | – | C_SC 6002 |
| `Treated.Flow` | Treated water flow | l/s | IR 32–33 | M_ME_NC 3017 | – |
| `Treated.Turbidity` | Treated water turbidity | NTU | IR 34–35 | M_ME_NC 3018 | – |
| `Treated.pH` | Treated water pH | pH | IR 36–37 | M_ME_NC 3019 | – |
| `Turb.High` | High treated water turbidity (>0.5 NTU) | – | DI 109 | M_SP 2010 | – |
| `Cl.Auto` | Chlorine dosing | – | Coil 27, DI 27 | M_SP 2108 | C_SC 6108 |
| `Cl.Dose` | Chlorine dose | mg/l | IR 38–39 | M_ME_NC 3020 | – |
| `Cl.DoseSet` | Chlorine dose in manual | mg/l | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `Cl.ResidualSet` | Residual chlorine setpoint | mg/l | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `Cl.Residual` | Residual chlorine in treated water | mg/l | IR 40–41 | M_ME_NC 3021 | – |
| `Cl.Low` | Low residual chlorine (<0.1 mg/l) | – | DI 110 | M_SP 2011 | – |
| `Cl.High` | High residual chlorine (>1.0 mg/l) | – | DI 111 | M_SP 2012 | – |
| `Res.Level` | Clear water reservoir level | m | IR 42–43 | M_ME_NC 3022 | – |
| `Res.Volume` | Clear water reservoir volume | m³ | IR 44–45 | M_ME_NC 3023 | – |
| `Res.Low` | Low reservoir level (<1.0 m) | – | DI 112 | M_SP 2013 | – |
| `Res.High` | High reservoir level (overflow) | – | DI 113 | M_SP 2014 | – |
| `HP1.Run` | High-lift pump 1 start/stop | – | Coil 28, DI 28 | M_SP 2109 | C_SC 6109 |
| `HP1.Running` | High-lift pump 1 running | – | DI 114 | M_SP 2015 | – |
| `HP1.Trip` | High-lift pump 1 motor protection tripped | – | DI 115 | M_SP 2016 | – |
| `HP1.Speed` | High-lift pump 1 speed | % | IR 46–47 | M_ME_NC 3024 | – |
| `HP1.Flow` | High-lift pump 1 flow | l/s | IR 48–49 | M_ME_NC 3025 | – |
| `HP1.Power` | High-lift pump 1 power | kW | IR 50–51 | M_ME_NC 3026 | – |
| `HP1.FaultSim` | Simulation: motor fault HP1 | – | Coil 29, DI 29 | M_SP 2110 | C_SC 6110 |
| `HP2.Run` | High-lift pump 2 start/stop | – | Coil 30, DI 30 | M_SP 2111 | C_SC 6111 |
| `HP2.Running` | High-lift pump 2 running | – | DI 116 | M_SP 2017 | – |
| `HP2.Trip` | High-lift pump 2 motor protection tripped | – | DI 117 | M_SP 2018 | – |
| `HP2.Speed` | High-lift pump 2 speed | % | IR 52–53 | M_ME_NC 3027 | – |
| `HP2.Flow` | High-lift pump 2 flow | l/s | IR 54–55 | M_ME_NC 3028 | – |
| `HP2.Power` | High-lift pump 2 power | kW | IR 56–57 | M_ME_NC 3029 | – |
| `HP2.FaultSim` | Simulation: motor fault HP2 | – | Coil 31, DI 31 | M_SP 2112 | C_SC 6112 |
| `Outlet.Valve` | Outlet valve to the network | – | Coil 32, DI 32 | M_SP 2113 | C_SC 6113 |
| `Net.Pressure` | Distribution network pressure | bar | IR 58–59 | M_ME_NC 3030 | – |
| `Net.PressureSet` | Network pressure setpoint | bar | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `Net.Flow` | Flow delivered to the network | l/s | IR 60–61 | M_ME_NC 3031 | – |
| `Net.Demand` | Network consumption | l/s | IR 62–63 | M_ME_NC 3032 | – |
| `Press.Low` | Low network pressure (<2.5 bar) | – | DI 118 | M_SP 2019 | – |
| `Raw.StormNTU` | Simulation: extra raw water turbidity (rain, storm) | NTU | HR 10–11 | M_ME_NC 4106 | C_SE_NC 8106 |
| `Demand.Scale` | Simulation: consumption scaling | % | HR 12–13 | M_ME_NC 4107 | C_SE_NC 8107 |
| `Plant.CommonAlarm` | Common alarm | – | DI 119 | M_SP 2020 | – |
| `Plant.Reset` | Reset trips | – | Coil 42 | – | C_SC 6003 |

## Layout

```
cmd/blue-water/    main, flags
internal/plant/    process model (water.go), signal list (water_points.go), shared core (core.go)
internal/modbus/   Modbus TCP server
internal/iec104/   IEC 60870-5-104 server (slave)
internal/web/      web interface and JSON API
```
