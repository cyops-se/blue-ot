**Svenska** · [English](README-en.md)

# Blue Water – simulator för vattenverk

Ett dricksvattenverk skrivet i Go (bara standardbiblioteket). Råvatten pumpas från en sjö, renas genom kemisk fällning, sedimentering och sandfilter, desinficeras med klor och lagras i en rentvattenreservoar. Högtryckspumpar håller trycket i distributionsnätet medan förbrukningen följer en dygnsprofil. Verket kan övervakas och styras via **Modbus TCP** och **IEC 60870-5-104** och har ett webbgränssnitt, precis som [blue-station](../blue-station/README.md) och [blue-chp](../blue-chp/README.md).

```
  sjö ─[RP1/RP2]─► fällning ─► sedimentering ─► [F1/F2 sandfilter] ─► klor
                                                                       │
          nät ◄─[utgående ventil]◄─[HP1/HP2]◄─ rentvattenreservoar 8000 m³
                                                       ▲
                            [Supply.CB] elmatning till hela verket
```

## Köra

```sh
go run ./cmd/blue-water                    # Modbus :1503, IEC 104 :2406, webb :8085
go run ./cmd/blue-water -timescale 60      # dygnsprofilen går 60 gånger snabbare
go run ./cmd/blue-water -points            # skriv ut signallistan (Markdown; -points-json ger JSON)
go test ./...
```

| Flagga | Standard | Beskrivning |
|---|---|---|
| `-modbus` | `:1503` | Lyssnaradress för Modbus TCP (tom = av) |
| `-iec104` | `:2406` | Lyssnaradress för IEC 104 (tom = av) |
| `-http` | `:8085` | Webbgränssnitt och JSON-API (tom = av) |
| `-ca` | `3` | IEC 104 gemensam adress (CA) |
| `-iec-utc` | `false` | Tidsstämplar (CP56Time2a) i UTC i stället för lokal tid |
| `-tick` | `1s` | Simuleringens tidssteg |
| `-timescale` | `1` | Hastighet på dygnsklockan som styr förbrukningen |

## Vad som simuleras

- **Elmatning:** hela verket matas genom brytaren `Supply.CB`. `Power.Available` simulerar avbrott i den externa matningen. Utan matning står alla pumpar.
- **Råvattenintag:** två pumpar på 130 l/s vardera. I AUTO styrs råvattenflödet efter förbrukningen med en korrektion mot reservoarens nivåbörvärde. I MANUELL går de startade pumparna för fullt.
- **Fällning och sedimentering:** fällningskemikalien doseras i AUTO efter råvattnets turbiditet (8 + 3 mg/l per NTU). För låg dos ger sämre avskiljning och högre turbiditet efter sedimenteringen.
- **Sandfilter:** filtermotståndet ökar med flödet och turbiditeten. Över 2,2 m larmar filtret och börjar släppa igenom partiklar. Backspolning (`Fx.Backwash`) tar filtret ur drift i 5 minuter, förbrukar spolvatten från reservoaren och återställer motståndet. Bara ett filter i taget kan backspolas.
- **Desinfektion:** i AUTO doseras klor så att restkloret håller sitt börvärde. Klorförbrukningen ökar med turbiditeten.
- **Reservoar:** 8000 m³ och 6 m djup. Den fylls med behandlat vatten och töms av förbrukning och spolvatten.
- **Distribution:** två högtryckspumpar på 160 l/s vardera håller nättrycket. Räcker inte kapaciteten sjunker trycket. Stängd utgående ventil, tom reservoar eller utan pumpar faller trycket till vattentornets statiska tryck. Pumpeffekten räknas från flöde och tryck.
- **Skydd:** varje pump har motorskydd (felsimulering med `xx.FaultSim`), och högtryckspumparna har torrkörningsskydd vid låg reservoarnivå. En utlöst pump startar inte förrän `Plant.Reset` har körts.

### Förslag på scenarier

1. Sätt `Raw.StormNTU` = 40 och se filtermotståndet stiga tills filtren larmar. Backspola F1 och sedan F2.
2. Ställ `Coag.Auto` = MANUELL med låg dos (`Coag.DoseSet` = 5) under storm. Turbiditeten i utgående vatten larmar. Gå tillbaka till AUTO och backspola filtren.
3. Stoppa `HP2.Run` och höj förbrukningen (`Demand.Scale` = 250). En pump räcker inte och nättrycket larmar.
4. Slå från `Supply.CB`. Alla pumpar stannar, trycket faller och reservoaren slutar fyllas.
5. Sätt `HP1.FaultSim` = FEL. HP1 löser ut. Ta bort felet, kör `Plant.Reset` och se pumpen starta igen.
6. Stäng `Outlet.Valve`. Leveransen upphör och trycket i nätet faller.

## Modbus TCP och IEC 104

Gränssnitten och adresseringen är desamma som i blue-station och blue-chp:

- **Modbus:** funktionskoderna 1, 2, 3, 4, 5, 15 och 16, float32 ABCD, och börvärden skrivs med FC16.
- **IEC 104:** generalförfrågan, spontana ändringar med tidsstämpel och kommandon med direct execute eller select/execute.

Brytare från coil 0 / IOA 1001, omkopplare från coil 20 / IOA 2101, pulskommandon från coil 40 / IOA 6001, status från DI 100 / IOA 2001, mätvärden från IR 0 / IOA 3001 och börvärden från HR 0 / IOA 4101 (styrning 8101).

## Webbgränssnitt och API

`http://localhost:8085` visar alla signaler grupperade, med reglage och en händelselogg.

- `GET /api/state` returnerar alla punkter och händelser.
- `POST /api/control` tar emot `{"id": "HP2.Run", "on": false}` eller `{"id": "Net.PressureSet", "value": 5}`.

## Signallista

| ID | Beskrivning | Enhet | Modbus TCP | IEC 104 övervakning | IEC 104 styrning |
|---|---|---|---|---|---|
| `Supply.CB` | Inkommande brytare (elmatning) | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Power.Available` | Simulering: extern elmatning | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `Power.Lost` | Verket spänningslöst | – | DI 100 | M_SP 2001 | – |
| `Power.Total` | Total eleffekt | kW | IR 0–1 | M_ME_NC 3001 | – |
| `Power.Energy` | Förbrukad elenergi | MWh | IR 2–3 | M_ME_NC 3002 | – |
| `RP1.Run` | Råvattenpump 1 start/stopp | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `RP1.Running` | Råvattenpump 1 i drift | – | DI 101 | M_SP 2002 | – |
| `RP1.Trip` | Råvattenpump 1 motorskydd utlöst | – | DI 102 | M_SP 2003 | – |
| `RP1.Speed` | Råvattenpump 1 varvtal | % | IR 4–5 | M_ME_NC 3003 | – |
| `RP1.Flow` | Råvattenpump 1 flöde | l/s | IR 6–7 | M_ME_NC 3004 | – |
| `RP1.Power` | Råvattenpump 1 effekt | kW | IR 8–9 | M_ME_NC 3005 | – |
| `RP1.FaultSim` | Simulering: motorfel RP1 | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `RP2.Run` | Råvattenpump 2 start/stopp | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `RP2.Running` | Råvattenpump 2 i drift | – | DI 103 | M_SP 2004 | – |
| `RP2.Trip` | Råvattenpump 2 motorskydd utlöst | – | DI 104 | M_SP 2005 | – |
| `RP2.Speed` | Råvattenpump 2 varvtal | % | IR 10–11 | M_ME_NC 3006 | – |
| `RP2.Flow` | Råvattenpump 2 flöde | l/s | IR 12–13 | M_ME_NC 3007 | – |
| `RP2.Power` | Råvattenpump 2 effekt | kW | IR 14–15 | M_ME_NC 3008 | – |
| `RP2.FaultSim` | Simulering: motorfel RP2 | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `Raw.Auto` | Råvattenflöde styrs av reservoarnivå | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `Raw.Flow` | Råvattenflöde | l/s | IR 16–17 | M_ME_NC 3009 | – |
| `Raw.Turbidity` | Turbiditet råvatten | NTU | IR 18–19 | M_ME_NC 3010 | – |
| `Res.LevelSet` | Börvärde nivå rentvattenreservoar | m | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `Coag.Auto` | Fällningskemikalie dosering | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `Coag.Dose` | Dos fällningskemikalie | mg/l | IR 20–21 | M_ME_NC 3011 | – |
| `Coag.DoseSet` | Dos fällningskemikalie vid manuell | mg/l | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `Settled.Turbidity` | Turbiditet efter sedimentering | NTU | IR 22–23 | M_ME_NC 3012 | – |
| `F1.Headloss` | F1 filtermotstånd | m | IR 24–25 | M_ME_NC 3013 | – |
| `F1.Flow` | F1 flöde | l/s | IR 26–27 | M_ME_NC 3014 | – |
| `F1.InService` | F1 i drift | – | DI 105 | M_SP 2006 | – |
| `F1.Clogged` | F1 igensatt (>2.2 m) | – | DI 106 | M_SP 2007 | – |
| `F1.Backwash` | F1 starta backspolning | – | Coil 40 | – | C_SC 6001 |
| `F2.Headloss` | F2 filtermotstånd | m | IR 28–29 | M_ME_NC 3015 | – |
| `F2.Flow` | F2 flöde | l/s | IR 30–31 | M_ME_NC 3016 | – |
| `F2.InService` | F2 i drift | – | DI 107 | M_SP 2008 | – |
| `F2.Clogged` | F2 igensatt (>2.2 m) | – | DI 108 | M_SP 2009 | – |
| `F2.Backwash` | F2 starta backspolning | – | Coil 41 | – | C_SC 6002 |
| `Treated.Flow` | Flöde behandlat vatten | l/s | IR 32–33 | M_ME_NC 3017 | – |
| `Treated.Turbidity` | Turbiditet utgående vatten | NTU | IR 34–35 | M_ME_NC 3018 | – |
| `Treated.pH` | pH utgående vatten | pH | IR 36–37 | M_ME_NC 3019 | – |
| `Turb.High` | Hög turbiditet utgående vatten (>0,5 NTU) | – | DI 109 | M_SP 2010 | – |
| `Cl.Auto` | Klordosering | – | Coil 27, DI 27 | M_SP 2108 | C_SC 6108 |
| `Cl.Dose` | Klordos | mg/l | IR 38–39 | M_ME_NC 3020 | – |
| `Cl.DoseSet` | Klordos vid manuell | mg/l | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `Cl.ResidualSet` | Börvärde restklor | mg/l | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `Cl.Residual` | Restklor utgående vatten | mg/l | IR 40–41 | M_ME_NC 3021 | – |
| `Cl.Low` | Låg restklor (<0,1 mg/l) | – | DI 110 | M_SP 2011 | – |
| `Cl.High` | Hög restklor (>1,0 mg/l) | – | DI 111 | M_SP 2012 | – |
| `Res.Level` | Nivå rentvattenreservoar | m | IR 42–43 | M_ME_NC 3022 | – |
| `Res.Volume` | Volym rentvattenreservoar | m³ | IR 44–45 | M_ME_NC 3023 | – |
| `Res.Low` | Låg nivå reservoar (<1,0 m) | – | DI 112 | M_SP 2013 | – |
| `Res.High` | Hög nivå reservoar (bräddning) | – | DI 113 | M_SP 2014 | – |
| `HP1.Run` | Högtryckspump 1 start/stopp | – | Coil 28, DI 28 | M_SP 2109 | C_SC 6109 |
| `HP1.Running` | Högtryckspump 1 i drift | – | DI 114 | M_SP 2015 | – |
| `HP1.Trip` | Högtryckspump 1 motorskydd utlöst | – | DI 115 | M_SP 2016 | – |
| `HP1.Speed` | Högtryckspump 1 varvtal | % | IR 46–47 | M_ME_NC 3024 | – |
| `HP1.Flow` | Högtryckspump 1 flöde | l/s | IR 48–49 | M_ME_NC 3025 | – |
| `HP1.Power` | Högtryckspump 1 effekt | kW | IR 50–51 | M_ME_NC 3026 | – |
| `HP1.FaultSim` | Simulering: motorfel HP1 | – | Coil 29, DI 29 | M_SP 2110 | C_SC 6110 |
| `HP2.Run` | Högtryckspump 2 start/stopp | – | Coil 30, DI 30 | M_SP 2111 | C_SC 6111 |
| `HP2.Running` | Högtryckspump 2 i drift | – | DI 116 | M_SP 2017 | – |
| `HP2.Trip` | Högtryckspump 2 motorskydd utlöst | – | DI 117 | M_SP 2018 | – |
| `HP2.Speed` | Högtryckspump 2 varvtal | % | IR 52–53 | M_ME_NC 3027 | – |
| `HP2.Flow` | Högtryckspump 2 flöde | l/s | IR 54–55 | M_ME_NC 3028 | – |
| `HP2.Power` | Högtryckspump 2 effekt | kW | IR 56–57 | M_ME_NC 3029 | – |
| `HP2.FaultSim` | Simulering: motorfel HP2 | – | Coil 31, DI 31 | M_SP 2112 | C_SC 6112 |
| `Outlet.Valve` | Utgående ventil mot nätet | – | Coil 32, DI 32 | M_SP 2113 | C_SC 6113 |
| `Net.Pressure` | Tryck distributionsnät | bar | IR 58–59 | M_ME_NC 3030 | – |
| `Net.PressureSet` | Börvärde nättryck | bar | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `Net.Flow` | Levererat flöde till nätet | l/s | IR 60–61 | M_ME_NC 3031 | – |
| `Net.Demand` | Förbrukning i nätet | l/s | IR 62–63 | M_ME_NC 3032 | – |
| `Press.Low` | Lågt nättryck (<2,5 bar) | – | DI 118 | M_SP 2019 | – |
| `Raw.StormNTU` | Simulering: extra råvattenturbiditet (regn, storm) | NTU | HR 10–11 | M_ME_NC 4106 | C_SE_NC 8106 |
| `Demand.Scale` | Simulering: skalning förbrukning | % | HR 12–13 | M_ME_NC 4107 | C_SE_NC 8107 |
| `Plant.CommonAlarm` | Summalarm | – | DI 119 | M_SP 2020 | – |
| `Plant.Reset` | Återställ utlösningar | – | Coil 42 | – | C_SC 6003 |

## Struktur

```
cmd/blue-water/    main, flaggor
internal/plant/    processmodell (water.go), signallista (water_points.go), gemensam kärna (core.go)
internal/modbus/   Modbus TCP-server
internal/iec104/   IEC 60870-5-104-server (slav)
internal/web/      webbgränssnitt och JSON-API
```
