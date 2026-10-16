**Svenska** · [English](README-en.md)

# Blue WWTP – simulator för avloppsverk

Ett kommunalt avloppsreningsverk skrivet i Go (bara standardbiblioteket). Spillvatten och regnvatten lyfts av en nivåstyrd inloppspumpstation och renas i aktivslam med luftning och kemisk fällning. Slammet rötas, och biogasen värmer rötkammaren. Verket har **Modbus TCP**, **IEC 60870-5-104** och ett webbgränssnitt, precis som [blue-station](../blue-station/README.md) och [blue-chp](../blue-chp/README.md).

```
  ledningsnät ─► pumpsump ─[IP1–IP3]─► galler ─► aktivslam ─► fällning ─► recipient
                    │                              [B1/B2]
                bräddning                    slam ─► rötkammare ─► biogas ─► gaspanna / fackla
```

## Köra

```sh
go run ./cmd/blue-wwtp                    # Modbus :1504, IEC 104 :2407, webb :8086
go run ./cmd/blue-wwtp -timescale 60      # dygnsprofilen går 60 gånger snabbare
go run ./cmd/blue-wwtp -points            # signallistan (Markdown; -points-json ger JSON)
go test ./...
```

| Flagga | Standard | Beskrivning |
|---|---|---|
| `-modbus` | `:1504` | Lyssnaradress för Modbus TCP (tom = av) |
| `-iec104` | `:2407` | Lyssnaradress för IEC 104 (tom = av) |
| `-http` | `:8086` | Webbgränssnitt och JSON-API (tom = av) |
| `-ca` | `4` | IEC 104 gemensam adress (CA) |
| `-iec-utc` | `false` | Tidsstämplar i UTC i stället för lokal tid |
| `-tick` | `1s` | Simuleringens tidssteg |
| `-timescale` | `1` | Hastighet på dygnsklockan |

## Vad som simuleras

- **Inflöde:** spillvatten med dygnsprofil (cirka 180 l/s i medel) plus 25 l/s per mm/h regn. Regnvattnet späder inkommande ammonium och fosfor.
- **Inloppspumpstation:** tre pumpar på 220 l/s. I AUTO startas och stoppas de efter nivån i pumpsumpen. `Run` fungerar som driftval: i AUTO är bara startade pumpar tillgängliga, i MANUELL går de. När nivån når 3,8 m bräddar sumpen, och bräddad volym summeras.
- **Galler:** ett igensatt galler (`Screen.FaultSim`) halverar flödet till pumparna.
- **Aktivslam:** två blåsmaskiner på 3000 Nm³/h. I AUTO regleras luften så att löst syre håller börvärdet. Nitrifikationen beror på syrehalt och vattentemperatur, så lågt syre eller kallt vatten ger högt ammonium ut.
- **Kemisk fällning:** i AUTO doseras fällningskemikalien efter inkommande fosfor.
- **Utgående vatten:** NH4-N, Ptot och BOD7, med larm över gränsvärdena 5 mg/l NH4 och 0,3 mg/l Ptot.
- **Rötning:** gaspannan håller rötkammaren vid 37 °C med egen biogas. En kall rötkammare producerar mindre gas. Gasklockan rymmer 1500 Nm³, och överskottsgas facklas.
- **Elmatning och skydd:** allt matas via `Supply.CB`. Pumpar och blåsmaskiner har motorskydd (`xx.FaultSim`) som kräver `Plant.Reset`.

### Förslag på scenarier

1. Sätt `Rain.Intensity` = 30. Alla tre pumparna går och pumpsumpen bräddar.
2. Sätt `Screen.FaultSim` = FEL vid måttligt regn. Bräddningen börjar tidigare.
3. Stoppa `B2.Run` och sätt `B1.FaultSim` = FEL. Syret faller och ammonium ut stiger.
4. Sänk `Water.Temp` till 4 °C. Nitrifikationen avtar och ammonium larmar.
5. Ställ `Prec.Auto` = MANUELL med låg dos. Fosfor ut larmar.
6. Stoppa `Boiler.Run`. Rötkammaren svalnar och gasproduktionen avtar.

## Modbus TCP, IEC 104 och webb

Gränssnitt och adressering är desamma som i de andra simulatorerna. `http://localhost:8086` visar alla signaler med reglage och händelselogg. `GET /api/state` returnerar punkter och händelser, och `POST /api/control` tar emot `{"id": "B2.Run", "on": true}` eller `{"id": "Bio.DOSet", "value": 2.5}`.

## Signallista

| ID | Beskrivning | Enhet | Modbus TCP | IEC 104 övervakning | IEC 104 styrning |
|---|---|---|---|---|---|
| `Supply.CB` | Inkommande brytare (elmatning) | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Power.Available` | Simulering: extern elmatning | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `Power.Lost` | Verket spänningslöst | – | DI 100 | M_SP 2001 | – |
| `Power.Total` | Total eleffekt | kW | IR 0–1 | M_ME_NC 3001 | – |
| `Power.Energy` | Förbrukad elenergi | MWh | IR 2–3 | M_ME_NC 3002 | – |
| `Inflow.Flow` | Inkommande flöde från ledningsnätet | l/s | IR 4–5 | M_ME_NC 3003 | – |
| `Well.Level` | Nivå pumpsump | m | IR 6–7 | M_ME_NC 3004 | – |
| `Well.High` | Hög nivå pumpsump (>3,3 m) | – | DI 101 | M_SP 2002 | – |
| `Inlet.Auto` | Nivåstyrning inloppspumpar | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `IP1.Run` | Inloppspump 1 driftval (MANUELL: kör, AUTO: tillgänglig) | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `IP1.Running` | Inloppspump 1 i drift | – | DI 102 | M_SP 2003 | – |
| `IP1.Trip` | Inloppspump 1 motorskydd utlöst | – | DI 103 | M_SP 2004 | – |
| `IP1.Flow` | Inloppspump 1 flöde | l/s | IR 8–9 | M_ME_NC 3005 | – |
| `IP1.Power` | Inloppspump 1 effekt | kW | IR 10–11 | M_ME_NC 3006 | – |
| `IP1.FaultSim` | Simulering: motorfel IP1 | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `IP2.Run` | Inloppspump 2 driftval (MANUELL: kör, AUTO: tillgänglig) | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `IP2.Running` | Inloppspump 2 i drift | – | DI 104 | M_SP 2005 | – |
| `IP2.Trip` | Inloppspump 2 motorskydd utlöst | – | DI 105 | M_SP 2006 | – |
| `IP2.Flow` | Inloppspump 2 flöde | l/s | IR 12–13 | M_ME_NC 3007 | – |
| `IP2.Power` | Inloppspump 2 effekt | kW | IR 14–15 | M_ME_NC 3008 | – |
| `IP2.FaultSim` | Simulering: motorfel IP2 | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `IP3.Run` | Inloppspump 3 driftval (MANUELL: kör, AUTO: tillgänglig) | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `IP3.Running` | Inloppspump 3 i drift | – | DI 106 | M_SP 2007 | – |
| `IP3.Trip` | Inloppspump 3 motorskydd utlöst | – | DI 107 | M_SP 2008 | – |
| `IP3.Flow` | Inloppspump 3 flöde | l/s | IR 16–17 | M_ME_NC 3009 | – |
| `IP3.Power` | Inloppspump 3 effekt | kW | IR 18–19 | M_ME_NC 3010 | – |
| `IP3.FaultSim` | Simulering: motorfel IP3 | – | Coil 27, DI 27 | M_SP 2108 | C_SC 6108 |
| `Pumped.Flow` | Pumpat flöde till reningen | l/s | IR 20–21 | M_ME_NC 3011 | – |
| `Overflow.Active` | Bräddning pågår | – | DI 108 | M_SP 2009 | – |
| `Overflow.Flow` | Bräddat flöde | l/s | IR 22–23 | M_ME_NC 3012 | – |
| `Overflow.Volume` | Bräddad volym (ackumulerad) | m³ | IR 24–25 | M_ME_NC 3013 | – |
| `Screen.Running` | Galler i drift | – | DI 109 | M_SP 2010 | – |
| `Screen.Blocked` | Galler igensatt | – | DI 110 | M_SP 2011 | – |
| `Screen.FaultSim` | Simulering: igensatt galler | – | Coil 28, DI 28 | M_SP 2109 | C_SC 6109 |
| `Blower.Auto` | Syrereglering blåsmaskiner | – | Coil 29, DI 29 | M_SP 2110 | C_SC 6110 |
| `B1.Run` | Blåsmaskin 1 driftval (MANUELL: kör, AUTO: tillgänglig) | – | Coil 30, DI 30 | M_SP 2111 | C_SC 6111 |
| `B1.Running` | Blåsmaskin 1 i drift | – | DI 111 | M_SP 2012 | – |
| `B1.Trip` | Blåsmaskin 1 motorskydd utlöst | – | DI 112 | M_SP 2013 | – |
| `B1.Air` | Blåsmaskin 1 flöde | Nm³/h | IR 26–27 | M_ME_NC 3014 | – |
| `B1.Power` | Blåsmaskin 1 effekt | kW | IR 28–29 | M_ME_NC 3015 | – |
| `B1.FaultSim` | Simulering: motorfel B1 | – | Coil 31, DI 31 | M_SP 2112 | C_SC 6112 |
| `B2.Run` | Blåsmaskin 2 driftval (MANUELL: kör, AUTO: tillgänglig) | – | Coil 32, DI 32 | M_SP 2113 | C_SC 6113 |
| `B2.Running` | Blåsmaskin 2 i drift | – | DI 113 | M_SP 2014 | – |
| `B2.Trip` | Blåsmaskin 2 motorskydd utlöst | – | DI 114 | M_SP 2015 | – |
| `B2.Air` | Blåsmaskin 2 flöde | Nm³/h | IR 30–31 | M_ME_NC 3016 | – |
| `B2.Power` | Blåsmaskin 2 effekt | kW | IR 32–33 | M_ME_NC 3017 | – |
| `B2.FaultSim` | Simulering: motorfel B2 | – | Coil 33, DI 33 | M_SP 2114 | C_SC 6114 |
| `Air.Flow` | Total luftmängd | Nm³/h | IR 34–35 | M_ME_NC 3018 | – |
| `Bio.Flow` | Flöde genom bassängerna | l/s | IR 36–37 | M_ME_NC 3019 | – |
| `Bio.DO` | Löst syre i luftningsbassäng | mg/l | IR 38–39 | M_ME_NC 3020 | – |
| `Bio.DOSet` | Börvärde löst syre | mg/l | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `Bio.DOLow` | Lågt syre (<1,0 mg/l) | – | DI 115 | M_SP 2016 | – |
| `Prec.Auto` | Dosering fällningskemikalie | – | Coil 34, DI 34 | M_SP 2115 | C_SC 6115 |
| `Prec.Dose` | Dos fällningskemikalie | mg/l | IR 40–41 | M_ME_NC 3021 | – |
| `Prec.DoseSet` | Dos fällningskemikalie vid manuell | mg/l | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `In.NH4` | Ammoniumkväve inkommande | mg/l | IR 42–43 | M_ME_NC 3022 | – |
| `In.Ptot` | Totalfosfor inkommande | mg/l | IR 44–45 | M_ME_NC 3023 | – |
| `Eff.Flow` | Utgående flöde till recipient | l/s | IR 46–47 | M_ME_NC 3024 | – |
| `Eff.NH4` | Ammoniumkväve utgående | mg/l | IR 48–49 | M_ME_NC 3025 | – |
| `Eff.Ptot` | Totalfosfor utgående | mg/l | IR 50–51 | M_ME_NC 3026 | – |
| `Eff.BOD7` | BOD7 utgående | mg/l | IR 52–53 | M_ME_NC 3027 | – |
| `Eff.NH4High` | Ammonium över gränsvärde (>5 mg/l) | – | DI 116 | M_SP 2017 | – |
| `Eff.PtotHigh` | Fosfor över gränsvärde (>0,3 mg/l) | – | DI 117 | M_SP 2018 | – |
| `Digester.Temp` | Temperatur rötkammare | °C | IR 54–55 | M_ME_NC 3028 | – |
| `Digester.TempLow` | Låg temperatur rötkammare (<33 °C) | – | DI 118 | M_SP 2019 | – |
| `Boiler.Run` | Gaspanna (värmer rötkammaren) | – | Coil 35, DI 35 | M_SP 2116 | C_SC 6116 |
| `Biogas.Flow` | Biogasproduktion | Nm³/h | IR 56–57 | M_ME_NC 3029 | – |
| `Gasholder.Level` | Nivå gasklocka | % | IR 58–59 | M_ME_NC 3030 | – |
| `Gas.Flaring` | Fackla brinner (överskottsgas) | – | DI 119 | M_SP 2020 | – |
| `Rain.Intensity` | Simulering: regnintensitet | mm/h | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `Load.Scale` | Simulering: skalning spillvattenbelastning | % | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `Water.Temp` | Simulering: vattentemperatur | °C | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `Plant.CommonAlarm` | Summalarm | – | DI 120 | M_SP 2021 | – |
| `Plant.Reset` | Återställ utlösningar | – | Coil 40 | – | C_SC 6001 |

## Struktur

```
cmd/blue-wwtp/     main, flaggor
internal/plant/    processmodell (wwtp.go), signallista (wwtp_points.go), gemensam kärna (core.go)
internal/modbus/   Modbus TCP-server
internal/iec104/   IEC 60870-5-104-server (slav)
internal/web/      webbgränssnitt och JSON-API
```
