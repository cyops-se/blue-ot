[Svenska](README.md) · **English**

# Blue Station – transformer substation simulator

A simple transformer substation written in Go (standard library only). The substation is fed from the 220 kV transmission grid and supplies four 20 kV distribution feeders. The CHP plant [blue-chp](../blue-chp/README-en.md) is connected as a generating source in bay G1. The substation can be monitored and controlled via **Modbus TCP** and **IEC 60870-5-104**, and also has a web interface.

```
       Transmission grid 220 kV
                |
              [Q0]                    line breaker
       ---------+---------            220 kV busbar
       |                 |
    [T1.HV]           [T2.HV]
     (T1)              (T2)           220/20 kV, 40 MVA, tap changer 1–17 (±1.25 %/step)
    [T1.LV]           [T2.LV]
       |                 |
   ====A====[Coupler]====B=========    20 kV busbar, sections A and B (the bus coupler is normally open)
     |    |            |    |    |
    F1    F2          F3    F4   G1
 Housing Industry  City centre Rural  CHP plant (blue-chp)
```

## Running

```sh
go run ./cmd/blue-station                  # Modbus :502, IEC 104 :2404, web :8080
go run ./cmd/blue-station -timescale 60    # the daily profile runs 60 times faster
go run ./cmd/blue-station -points          # print the signal list
go test ./...
```

| Flag | Default | Description |
|---|---|---|
| `-modbus` | `:502` | Listen address for Modbus TCP (empty = off) |
| `-iec104` | `:2404` | Listen address for IEC 104 (empty = off) |
| `-http` | `:8080` | Web interface and JSON API (empty = off) |
| `-ca` | `1` | IEC 104 common address (CA) |
| `-iec-utc` | `false` | Timestamps (CP56Time2a) in UTC instead of local time |
| `-tick` | `1s` | Simulation time step |
| `-timescale` | `1` | Speed of the daily clock that drives the load profiles |
| `-chp` | `http://127.0.0.1:8082` | Address of blue-chp for bay G1 (empty = no CHP) |

On Linux, port 502 requires root or `CAP_NET_BIND_SERVICE`. Otherwise use e.g. `-modbus :5020`.

## What is simulated

- **Transmission grid:** voltage and frequency vary slowly. `Grid.Available` simulates loss of supply from the transmission grid.
- **Load flow:** each feeder has its own daily profile (housing, industry, city centre, rural), its own power factor and random noise. Transformer losses are included. When the bus coupler is closed, parallel transformers share the load equally.
- **Voltage control (AVR):** the tap changer regulates towards `AVR.USet`, with a deadband and a delay. Manual raise and lower commands are rejected while the controller is in AUTO.
- **Thermal model:** the oil temperature follows the loading. The time constant is shortened to 10 minutes so that the behaviour is visible in a demo. An alarm is raised at 90 °C and the transformer trips at 105 °C.
- **Protection:**
  - The feeders have I> (2 s) and I>> (3×I>, instantaneous).
  - The transformers have a backup overcurrent protection (>150 % for 5 s) and trip on high oil temperature.
  - A trip blocks closing until `Station.Reset` has been executed.
- **CHP (G1):** bay G1 on section B feeds in the power produced by the generator in blue-chp. Once per second the substation sends the section's voltage and frequency and the position of the G1 breaker to blue-chp, and receives the generator's P and Q. The infeed reduces the load on T2. When the plant produces more than the section consumes, power flows back through the transformer towards the transmission grid. The generator cannot keep a busbar energised on its own. If G1 is opened or section B loses voltage, the plant's loss-of-mains protection trips. `G1.Link` shows whether the link to blue-chp is working.
- **Fault simulation:** `Fx.FaultSim` applies a short circuit on the feeder (6 kA). The section voltage drops and the protection trips. The fault remains until it is removed, so closing again trips again.

### Suggested scenarios

1. Apply a fault on F2 (`F2.FaultSim` = ON). F2 trips. Remove the fault, execute `Station.Reset` and close F2 again.
2. Open `T1.LV.CB` so that section A loses voltage. Close `BUS.Coupler.CB`, and T2 supplies the whole substation.
3. Do as in scenario 2 and then set `Load.Scale` = 150 %. Overload and oil temperature alarms follow, and eventually T2 trips.
4. Change `AVR.USet` and watch the tap changers step.
5. Start blue-chp. G1 feeds in about 25 MW, and T2 feeds power back to the transmission grid. Open `G1.CB` and the generator in blue-chp trips on loss of mains. Close it again, reset blue-chp (`Plant.Reset`) and synchronise (`Gen.CB`).

## Modbus TCP

- Function codes: 1, 2, 3, 4, 5, 15 and 16. All unit ids are accepted.
- **Coils** are breakers (1 = CLOSED), switches and pulse commands (write 1 to execute).
- **Discrete inputs** are breaker positions, switches and alarms.
- **Input registers** are measurements. **Holding registers** are setpoints.
- Analogue values are float32 in two registers, high word first (big endian, ABCD).
- Setpoints must be written as complete register pairs with FC16. FC6 is not supported.
- Exceptions: 02 means illegal address, 03 illegal value and 04 that the command was rejected (e.g. lockout or value out of range).

## IEC 60870-5-104

- Profile: COT 2 octets, CA 2 octets, IOA 3 octets. Defaults k=12, w=8, t1=15 s, t2=10 s, t3=20 s.
- **General interrogation** (`C_IC_NA_1`, QOI 20) is answered with `M_SP_NA_1`, `M_DP_NA_1`, `M_ST_NA_1` and `M_ME_NC_1`.
- **Spontaneous changes** are sent with timestamp (`M_SP_TB_1`, `M_DP_TB_1`, `M_ST_TB_1`, `M_ME_TF_1`). Measurements are only sent when they have changed by more than their deadband.
- **Commands** (`C_SC`, `C_DC`, `C_RC`, `C_SE_NC`) work both with and without timestamp. Both direct execute and select/execute are supported. The responses are ACTCON followed by ACTTERM, or a negative ACTCON if the command is rejected.
- The following are also supported:
  - `C_RD_NA_1` (read)
  - `C_CS_NA_1` (clock sync, acknowledged but does not change the clock)
  - `C_TS_TA_1` (test)
  - `M_EI_NA_1`, sent after the first STARTDT.
- Negative responses: COT 44 for unknown type, 45 for unknown COT, 46 for unknown CA and 47 for unknown IOA.

| IOA | Contents |
|---|---|
| 1001– | Breaker positions `M_DP` |
| 2001– | Alarms and status `M_SP` |
| 2101– | Switches `M_SP` |
| 3001– | Measurements `M_ME_NC` |
| 4001– | Tap positions `M_ST` |
| 4101– | Setpoint read-back `M_ME_NC` |
| 5001– | Breaker commands `C_DC` (1 = OPEN, 2 = CLOSE) |
| 6001– | Pulse commands `C_SC` |
| 6101– | Switches `C_SC` |
| 7001– | Tap changers `C_RC` (1 = lower, 2 = raise) |
| 8101– | Setpoints `C_SE_NC` |

## Web interface and API

`http://localhost:8080` shows all signals grouped by bay, with controls and an event log. The interface and the event texts are in Swedish.

- `GET /api/state` returns all points and events.
- `POST /api/control` accepts `{"id": "F1.CB", "on": false}` or `{"id": "AVR.USet", "value": 20.8}`.

## Signal list

The descriptions are translated here. In the simulator itself they are in Swedish.

| ID | Description | Unit | Modbus TCP | IEC 104 monitoring | IEC 104 control |
|---|---|---|---|---|---|
| `Grid.Available` | Simulation: supply from the transmission grid | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `Grid.VoltagePresent` | Voltage on incoming line | – | DI 100 | M_SP 2001 | – |
| `Grid.U` | Transmission grid voltage | kV | IR 0–1 | M_ME_NC 3001 | – |
| `Grid.f` | Frequency | Hz | IR 2–3 | M_ME_NC 3002 | – |
| `Grid.P` | Active power from transmission grid | MW | IR 4–5 | M_ME_NC 3003 | – |
| `Grid.Q` | Reactive power from transmission grid | Mvar | IR 6–7 | M_ME_NC 3004 | – |
| `Q0.CB` | Line breaker Q0 to transmission grid | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Q0.I` | Current through Q0 | A | IR 8–9 | M_ME_NC 3005 | – |
| `T1.HV.CB` | T1 HV breaker 220 kV | – | Coil 1, DI 1 | M_DP 1002 | C_DC 5002 |
| `T1.LV.CB` | T1 LV breaker 20 kV | – | Coil 2, DI 2 | M_DP 1003 | C_DC 5003 |
| `T1.Tap` | T1 tap position (1–17) | – | IR 200–201 | M_ST 4001 | C_RC 7001 |
| `T1.AVR.Auto` | T1 voltage control | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `T1.Tap.Raise` | T1 raise tap (manual) | – | Coil 40 | – | C_SC 6001 |
| `T1.Tap.Lower` | T1 lower tap (manual) | – | Coil 41 | – | C_SC 6002 |
| `T1.P` | T1 active power | MW | IR 10–11 | M_ME_NC 3006 | – |
| `T1.Q` | T1 reactive power | Mvar | IR 12–13 | M_ME_NC 3007 | – |
| `T1.I.HV` | T1 HV current | A | IR 14–15 | M_ME_NC 3008 | – |
| `T1.I.LV` | T1 LV current | A | IR 16–17 | M_ME_NC 3009 | – |
| `T1.Loading` | T1 loading | % | IR 18–19 | M_ME_NC 3010 | – |
| `T1.OilTemp` | T1 oil temperature | °C | IR 20–21 | M_ME_NC 3011 | – |
| `T1.Overload` | T1 overload (>100 %) | – | DI 101 | M_SP 2002 | – |
| `T1.OilAlarm` | T1 high oil temperature (>90 °C) | – | DI 102 | M_SP 2003 | – |
| `T1.Trip` | T1 protection trip | – | DI 103 | M_SP 2004 | – |
| `T2.HV.CB` | T2 HV breaker 220 kV | – | Coil 3, DI 3 | M_DP 1004 | C_DC 5004 |
| `T2.LV.CB` | T2 LV breaker 20 kV | – | Coil 4, DI 4 | M_DP 1005 | C_DC 5005 |
| `T2.Tap` | T2 tap position (1–17) | – | IR 202–203 | M_ST 4002 | C_RC 7002 |
| `T2.AVR.Auto` | T2 voltage control | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `T2.Tap.Raise` | T2 raise tap (manual) | – | Coil 42 | – | C_SC 6003 |
| `T2.Tap.Lower` | T2 lower tap (manual) | – | Coil 43 | – | C_SC 6004 |
| `T2.P` | T2 active power | MW | IR 22–23 | M_ME_NC 3012 | – |
| `T2.Q` | T2 reactive power | Mvar | IR 24–25 | M_ME_NC 3013 | – |
| `T2.I.HV` | T2 HV current | A | IR 26–27 | M_ME_NC 3014 | – |
| `T2.I.LV` | T2 LV current | A | IR 28–29 | M_ME_NC 3015 | – |
| `T2.Loading` | T2 loading | % | IR 30–31 | M_ME_NC 3016 | – |
| `T2.OilTemp` | T2 oil temperature | °C | IR 32–33 | M_ME_NC 3017 | – |
| `T2.Overload` | T2 overload (>100 %) | – | DI 104 | M_SP 2005 | – |
| `T2.OilAlarm` | T2 high oil temperature (>90 °C) | – | DI 105 | M_SP 2006 | – |
| `T2.Trip` | T2 protection trip | – | DI 106 | M_SP 2007 | – |
| `BUS.Coupler.CB` | Bus coupler A–B | – | Coil 5, DI 5 | M_DP 1006 | C_DC 5006 |
| `BUS.A.Live` | Voltage section A | – | DI 107 | M_SP 2008 | – |
| `BUS.A.U` | Voltage section A | kV | IR 34–35 | M_ME_NC 3018 | – |
| `BUS.B.Live` | Voltage section B | – | DI 108 | M_SP 2009 | – |
| `BUS.B.U` | Voltage section B | kV | IR 36–37 | M_ME_NC 3019 | – |
| `F1.CB` | F1 feeder breaker | – | Coil 6, DI 6 | M_DP 1007 | C_DC 5007 |
| `F1.P` | F1 active power | MW | IR 38–39 | M_ME_NC 3020 | – |
| `F1.Q` | F1 reactive power | Mvar | IR 40–41 | M_ME_NC 3021 | – |
| `F1.I` | F1 current | A | IR 42–43 | M_ME_NC 3022 | – |
| `F1.Energy` | F1 delivered energy | MWh | IR 44–45 | M_ME_NC 3023 | – |
| `F1.Trip` | F1 overcurrent protection tripped | – | DI 109 | M_SP 2010 | – |
| `F1.FaultSim` | Simulation: short circuit on F1 | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `F1.OC.Pickup` | F1 I> setting (I>> = 3×I>) | A | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `F2.CB` | F2 feeder breaker | – | Coil 7, DI 7 | M_DP 1008 | C_DC 5008 |
| `F2.P` | F2 active power | MW | IR 46–47 | M_ME_NC 3024 | – |
| `F2.Q` | F2 reactive power | Mvar | IR 48–49 | M_ME_NC 3025 | – |
| `F2.I` | F2 current | A | IR 50–51 | M_ME_NC 3026 | – |
| `F2.Energy` | F2 delivered energy | MWh | IR 52–53 | M_ME_NC 3027 | – |
| `F2.Trip` | F2 overcurrent protection tripped | – | DI 110 | M_SP 2011 | – |
| `F2.FaultSim` | Simulation: short circuit on F2 | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `F2.OC.Pickup` | F2 I> setting (I>> = 3×I>) | A | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `F3.CB` | F3 feeder breaker | – | Coil 8, DI 8 | M_DP 1009 | C_DC 5009 |
| `F3.P` | F3 active power | MW | IR 54–55 | M_ME_NC 3028 | – |
| `F3.Q` | F3 reactive power | Mvar | IR 56–57 | M_ME_NC 3029 | – |
| `F3.I` | F3 current | A | IR 58–59 | M_ME_NC 3030 | – |
| `F3.Energy` | F3 delivered energy | MWh | IR 60–61 | M_ME_NC 3031 | – |
| `F3.Trip` | F3 overcurrent protection tripped | – | DI 111 | M_SP 2012 | – |
| `F3.FaultSim` | Simulation: short circuit on F3 | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `F3.OC.Pickup` | F3 I> setting (I>> = 3×I>) | A | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `F4.CB` | F4 feeder breaker | – | Coil 9, DI 9 | M_DP 1010 | C_DC 5010 |
| `F4.P` | F4 active power | MW | IR 62–63 | M_ME_NC 3032 | – |
| `F4.Q` | F4 reactive power | Mvar | IR 64–65 | M_ME_NC 3033 | – |
| `F4.I` | F4 current | A | IR 66–67 | M_ME_NC 3034 | – |
| `F4.Energy` | F4 delivered energy | MWh | IR 68–69 | M_ME_NC 3035 | – |
| `F4.Trip` | F4 overcurrent protection tripped | – | DI 112 | M_SP 2013 | – |
| `F4.FaultSim` | Simulation: short circuit on F4 | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `F4.OC.Pickup` | F4 I> setting (I>> = 3×I>) | A | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `AVR.USet` | Voltage setpoint 20 kV | kV | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `AVR.Deadband` | Voltage control deadband | % | HR 10–11 | M_ME_NC 4106 | C_SE_NC 8106 |
| `AVR.Delay` | Voltage control delay | s | HR 12–13 | M_ME_NC 4107 | C_SE_NC 8107 |
| `Load.Scale` | Simulation: load scaling of all feeders | % | HR 14–15 | M_ME_NC 4108 | C_SE_NC 8108 |
| `Env.Ambient` | Simulation: ambient temperature | °C | HR 16–17 | M_ME_NC 4109 | C_SE_NC 8109 |
| `Station.CommonAlarm` | Common alarm | – | DI 113 | M_SP 2014 | – |
| `Station.Reset` | Reset protection trips and lockouts | – | Coil 44 | – | C_SC 6005 |
| `G1.CB` | G1 breaker CHP plant | – | Coil 10, DI 10 | M_DP 1011 | C_DC 5011 |
| `G1.P` | G1 active power (infeed) | MW | IR 70–71 | M_ME_NC 3036 | – |
| `G1.Q` | G1 reactive power (infeed) | Mvar | IR 72–73 | M_ME_NC 3037 | – |
| `G1.I` | G1 current | A | IR 74–75 | M_ME_NC 3038 | – |
| `G1.Energy` | G1 energy fed in | MWh | IR 76–77 | M_ME_NC 3039 | – |
| `G1.GenOnline` | G1 generator in the CHP plant connected | – | DI 114 | M_SP 2015 | – |
| `G1.Link` | G1 link to the CHP plant | – | DI 115 | M_SP 2016 | – |

The G1 points are last in the list, so all earlier addresses are unchanged.

## Structure

```
cmd/blue-station/     main, flags
internal/station/     substation model, load flow, protection, AVR, signal list
internal/chplink/     link from bay G1 to blue-chp
internal/modbus/      Modbus TCP server
internal/iec104/      IEC 60870-5-104 server (slave)
internal/web/         web interface and JSON API
```
