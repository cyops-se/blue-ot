# Blue Station – simulator för transformatorstation

En enkel transformatorstation skriven i Go (bara standardbiblioteket). Stationen matas från stamnätet (220 kV) och försörjer fyra distributionsnät (20 kV). Den kan övervakas och styras via **Modbus TCP** och **IEC 60870-5-104**, och har dessutom ett webbgränssnitt.

```
          Stamnät 220 kV
                |
              [Q0]                    linjebrytare
       ---------+---------            220 kV-skena
       |                 |
    [T1.HV]           [T2.HV]
     (T1)              (T2)           220/20 kV, 40 MVA, lindningskopplare 1–17 (±1,25 %/steg)
    [T1.LV]           [T2.LV]
       |                 |
   ====A====[Coupler]====B====        20 kV-skena, sektion A och B (sektioneringen är normalt öppen)
     |    |            |    |
    F1    F2          F3    F4
 Bostad Industri    Centrum Landsbygd
```

## Köra

```sh
go run ./cmd/blue-station                  # Modbus :502, IEC 104 :2404, webb :8080
go run ./cmd/blue-station -timescale 60    # dygnsprofilen går 60 gånger snabbare
go run ./cmd/blue-station -points          # skriv ut signallistan
go test ./...
```

| Flagga | Standard | Beskrivning |
|---|---|---|
| `-modbus` | `:502` | Lyssnaradress för Modbus TCP (tom = av) |
| `-iec104` | `:2404` | Lyssnaradress för IEC 104 (tom = av) |
| `-http` | `:8080` | Webbgränssnitt och JSON-API (tom = av) |
| `-ca` | `1` | IEC 104 gemensam adress (CA) |
| `-iec-utc` | `false` | Tidsstämplar (CP56Time2a) i UTC i stället för lokal tid |
| `-tick` | `1s` | Simuleringens tidssteg |
| `-timescale` | `1` | Hastighet på dygnsklockan som styr lastprofilerna |

På Linux krävs root eller `CAP_NET_BIND_SERVICE` för port 502. Använd annars t.ex. `-modbus :5020`.

## Vad som simuleras

- **Stamnät:** spänningen och frekvensen varierar långsamt. `Grid.Available` simulerar att matningen från stamnätet försvinner.
- **Lastflöde:** varje avgång har en egen dygnsprofil (bostäder, industri, centrum, landsbygd), en egen effektfaktor och slumpmässigt brus. Transformatorförlusterna räknas med. När sektioneringen är sluten delar parallellkopplade transformatorer lika på lasten.
- **Spänningsreglering (AVR):** lindningskopplaren regleras mot `AVR.USet`, med dödband och fördröjning. Manuella höj- och sänkkommandon nekas när regleringen står i AUTO.
- **Termisk modell:** oljetemperaturen följer belastningen. Tidskonstanten är förkortad till 10 minuter så att förloppet syns i en demo. Larm ges vid 90 °C och transformatorn löser ut vid 105 °C.
- **Skydd:**
  - Avgångarna har I> (2 s) och I>> (3×I>, momentant).
  - Transformatorerna har ett reservskydd för överström (>150 % i 5 s) och löser ut på hög oljetemperatur.
  - En utlösning blockerar tillslag tills `Station.Reset` har körts.
- **Felsimulering:** `Fx.FaultSim` lägger en kortslutning på avgången (6 kA). Spänningen på sektionen sjunker och skyddet löser ut. Felet ligger kvar tills det tas bort, så ett nytt tillslag löser ut igen.

### Förslag på scenarier

1. Lägg ett fel på F2 (`F2.FaultSim` = PÅ). F2 löser ut. Ta bort felet, kör `Station.Reset` och slå till F2 igen.
2. Slå från `T1.LV.CB` så att sektion A blir spänningslös. Slå till `BUS.Coupler.CB`, så matar T2 hela stationen.
3. Gör som i scenario 2 och sätt sedan `Load.Scale` = 150 %. Då kommer larm för överlast och oljetemperatur, och till sist löser T2 ut.
4. Ändra `AVR.USet` och se hur lindningskopplarna stegar.

## Modbus TCP

- Funktionskoder: 1, 2, 3, 4, 5, 15 och 16. Alla unit-id accepteras.
- **Coils** är brytare (1 = TILL), omkopplare och pulskommandon (skriv 1 för att utföra).
- **Discrete inputs** är brytarlägen, omkopplare och larm.
- **Input registers** är mätvärden. **Holding registers** är börvärden.
- Analoga värden är float32 i två register med det högsta ordet först (big endian, ABCD).
- Börvärden måste skrivas som hela registerpar med FC16. FC6 stöds inte.
- Undantag: 02 betyder fel adress, 03 fel värde och 04 att kommandot nekades (t.ex. blockering eller värde utanför intervallet).

## IEC 60870-5-104

- Profil: COT 2 oktetter, CA 2 oktetter, IOA 3 oktetter. Standardvärden k=12, w=8, t1=15 s, t2=10 s, t3=20 s.
- **Generalförfrågan** (`C_IC_NA_1`, QOI 20) besvaras med `M_SP_NA_1`, `M_DP_NA_1`, `M_ST_NA_1` och `M_ME_NC_1`.
- **Spontana ändringar** skickas med tidsstämpel (`M_SP_TB_1`, `M_DP_TB_1`, `M_ST_TB_1`, `M_ME_TF_1`). Mätvärden skickas bara när de har ändrats mer än sitt dödband.
- **Kommandon** (`C_SC`, `C_DC`, `C_RC`, `C_SE_NC`) fungerar både med och utan tidsstämpel. Både direct execute och select/execute stöds. Svaren är ACTCON följt av ACTTERM, eller en negativ ACTCON om kommandot nekas.
- Följande stöds också:
  - `C_RD_NA_1` (läsning)
  - `C_CS_NA_1` (klocksynk, kvitteras men ändrar inte klockan)
  - `C_TS_TA_1` (test)
  - `M_EI_NA_1`, som skickas efter den första STARTDT.
- Negativa svar: COT 44 för okänd typ, 45 för okänd COT, 46 för okänd CA och 47 för okänd IOA.

| IOA | Innehåll |
|---|---|
| 1001– | Brytarlägen `M_DP` |
| 2001– | Larm och status `M_SP` |
| 2101– | Omkopplare `M_SP` |
| 3001– | Mätvärden `M_ME_NC` |
| 4001– | Lindningskopplarläge `M_ST` |
| 4101– | Återläsning av börvärden `M_ME_NC` |
| 5001– | Brytarkommandon `C_DC` (1 = FRÅN, 2 = TILL) |
| 6001– | Pulskommandon `C_SC` |
| 6101– | Omkopplare `C_SC` |
| 7001– | Lindningskopplare `C_RC` (1 = sänk, 2 = höj) |
| 8101– | Börvärden `C_SE_NC` |

## Webbgränssnitt och API

`http://localhost:8080` visar alla signaler grupperade per fack, med reglage och en händelselogg.

- `GET /api/state` returnerar alla punkter och händelser.
- `POST /api/control` tar emot `{"id": "F1.CB", "on": false}` eller `{"id": "AVR.USet", "value": 20.8}`.

## Signallista

| ID | Beskrivning | Enhet | Modbus TCP | IEC 104 övervakning | IEC 104 styrning |
|---|---|---|---|---|---|
| `Grid.Available` | Simulering: matning från stamnätet | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `Grid.VoltagePresent` | Spänning på inkommande ledning | – | DI 100 | M_SP 2001 | – |
| `Grid.U` | Spänning stamnät | kV | IR 0–1 | M_ME_NC 3001 | – |
| `Grid.f` | Frekvens | Hz | IR 2–3 | M_ME_NC 3002 | – |
| `Grid.P` | Aktiv effekt från stamnät | MW | IR 4–5 | M_ME_NC 3003 | – |
| `Grid.Q` | Reaktiv effekt från stamnät | Mvar | IR 6–7 | M_ME_NC 3004 | – |
| `Q0.CB` | Linjebrytare Q0 mot stamnät | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Q0.I` | Ström genom Q0 | A | IR 8–9 | M_ME_NC 3005 | – |
| `T1.HV.CB` | T1 brytare uppsida 220 kV | – | Coil 1, DI 1 | M_DP 1002 | C_DC 5002 |
| `T1.LV.CB` | T1 brytare nedsida 20 kV | – | Coil 2, DI 2 | M_DP 1003 | C_DC 5003 |
| `T1.Tap` | T1 lindningskopplarläge (1–17) | – | IR 200–201 | M_ST 4001 | C_RC 7001 |
| `T1.AVR.Auto` | T1 spänningsreglering | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `T1.Tap.Raise` | T1 höj lindningskopplare (manuell) | – | Coil 40 | – | C_SC 6001 |
| `T1.Tap.Lower` | T1 sänk lindningskopplare (manuell) | – | Coil 41 | – | C_SC 6002 |
| `T1.P` | T1 aktiv effekt | MW | IR 10–11 | M_ME_NC 3006 | – |
| `T1.Q` | T1 reaktiv effekt | Mvar | IR 12–13 | M_ME_NC 3007 | – |
| `T1.I.HV` | T1 ström uppsida | A | IR 14–15 | M_ME_NC 3008 | – |
| `T1.I.LV` | T1 ström nedsida | A | IR 16–17 | M_ME_NC 3009 | – |
| `T1.Loading` | T1 belastning | % | IR 18–19 | M_ME_NC 3010 | – |
| `T1.OilTemp` | T1 oljetemperatur | °C | IR 20–21 | M_ME_NC 3011 | – |
| `T1.Overload` | T1 överlast (>100 %) | – | DI 101 | M_SP 2002 | – |
| `T1.OilAlarm` | T1 hög oljetemperatur (>90 °C) | – | DI 102 | M_SP 2003 | – |
| `T1.Trip` | T1 skyddsutlösning | – | DI 103 | M_SP 2004 | – |
| `T2.HV.CB` | T2 brytare uppsida 220 kV | – | Coil 3, DI 3 | M_DP 1004 | C_DC 5004 |
| `T2.LV.CB` | T2 brytare nedsida 20 kV | – | Coil 4, DI 4 | M_DP 1005 | C_DC 5005 |
| `T2.Tap` | T2 lindningskopplarläge (1–17) | – | IR 202–203 | M_ST 4002 | C_RC 7002 |
| `T2.AVR.Auto` | T2 spänningsreglering | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `T2.Tap.Raise` | T2 höj lindningskopplare (manuell) | – | Coil 42 | – | C_SC 6003 |
| `T2.Tap.Lower` | T2 sänk lindningskopplare (manuell) | – | Coil 43 | – | C_SC 6004 |
| `T2.P` | T2 aktiv effekt | MW | IR 22–23 | M_ME_NC 3012 | – |
| `T2.Q` | T2 reaktiv effekt | Mvar | IR 24–25 | M_ME_NC 3013 | – |
| `T2.I.HV` | T2 ström uppsida | A | IR 26–27 | M_ME_NC 3014 | – |
| `T2.I.LV` | T2 ström nedsida | A | IR 28–29 | M_ME_NC 3015 | – |
| `T2.Loading` | T2 belastning | % | IR 30–31 | M_ME_NC 3016 | – |
| `T2.OilTemp` | T2 oljetemperatur | °C | IR 32–33 | M_ME_NC 3017 | – |
| `T2.Overload` | T2 överlast (>100 %) | – | DI 104 | M_SP 2005 | – |
| `T2.OilAlarm` | T2 hög oljetemperatur (>90 °C) | – | DI 105 | M_SP 2006 | – |
| `T2.Trip` | T2 skyddsutlösning | – | DI 106 | M_SP 2007 | – |
| `BUS.Coupler.CB` | Sektioneringsbrytare A–B | – | Coil 5, DI 5 | M_DP 1006 | C_DC 5006 |
| `BUS.A.Live` | Spänning sektion A | – | DI 107 | M_SP 2008 | – |
| `BUS.A.U` | Spänning sektion A | kV | IR 34–35 | M_ME_NC 3018 | – |
| `BUS.B.Live` | Spänning sektion B | – | DI 108 | M_SP 2009 | – |
| `BUS.B.U` | Spänning sektion B | kV | IR 36–37 | M_ME_NC 3019 | – |
| `F1.CB` | F1 utgående brytare | – | Coil 6, DI 6 | M_DP 1007 | C_DC 5007 |
| `F1.P` | F1 aktiv effekt | MW | IR 38–39 | M_ME_NC 3020 | – |
| `F1.Q` | F1 reaktiv effekt | Mvar | IR 40–41 | M_ME_NC 3021 | – |
| `F1.I` | F1 ström | A | IR 42–43 | M_ME_NC 3022 | – |
| `F1.Energy` | F1 levererad energi | MWh | IR 44–45 | M_ME_NC 3023 | – |
| `F1.Trip` | F1 överströmsskydd utlöst | – | DI 109 | M_SP 2010 | – |
| `F1.FaultSim` | Simulering: kortslutning på F1 | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `F1.OC.Pickup` | F1 inställning I> (I>> = 3×I>) | A | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `F2.CB` | F2 utgående brytare | – | Coil 7, DI 7 | M_DP 1008 | C_DC 5008 |
| `F2.P` | F2 aktiv effekt | MW | IR 46–47 | M_ME_NC 3024 | – |
| `F2.Q` | F2 reaktiv effekt | Mvar | IR 48–49 | M_ME_NC 3025 | – |
| `F2.I` | F2 ström | A | IR 50–51 | M_ME_NC 3026 | – |
| `F2.Energy` | F2 levererad energi | MWh | IR 52–53 | M_ME_NC 3027 | – |
| `F2.Trip` | F2 överströmsskydd utlöst | – | DI 110 | M_SP 2011 | – |
| `F2.FaultSim` | Simulering: kortslutning på F2 | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `F2.OC.Pickup` | F2 inställning I> (I>> = 3×I>) | A | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `F3.CB` | F3 utgående brytare | – | Coil 8, DI 8 | M_DP 1009 | C_DC 5009 |
| `F3.P` | F3 aktiv effekt | MW | IR 54–55 | M_ME_NC 3028 | – |
| `F3.Q` | F3 reaktiv effekt | Mvar | IR 56–57 | M_ME_NC 3029 | – |
| `F3.I` | F3 ström | A | IR 58–59 | M_ME_NC 3030 | – |
| `F3.Energy` | F3 levererad energi | MWh | IR 60–61 | M_ME_NC 3031 | – |
| `F3.Trip` | F3 överströmsskydd utlöst | – | DI 111 | M_SP 2012 | – |
| `F3.FaultSim` | Simulering: kortslutning på F3 | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `F3.OC.Pickup` | F3 inställning I> (I>> = 3×I>) | A | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `F4.CB` | F4 utgående brytare | – | Coil 9, DI 9 | M_DP 1010 | C_DC 5010 |
| `F4.P` | F4 aktiv effekt | MW | IR 62–63 | M_ME_NC 3032 | – |
| `F4.Q` | F4 reaktiv effekt | Mvar | IR 64–65 | M_ME_NC 3033 | – |
| `F4.I` | F4 ström | A | IR 66–67 | M_ME_NC 3034 | – |
| `F4.Energy` | F4 levererad energi | MWh | IR 68–69 | M_ME_NC 3035 | – |
| `F4.Trip` | F4 överströmsskydd utlöst | – | DI 112 | M_SP 2013 | – |
| `F4.FaultSim` | Simulering: kortslutning på F4 | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `F4.OC.Pickup` | F4 inställning I> (I>> = 3×I>) | A | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `AVR.USet` | Börvärde spänning 20 kV | kV | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `AVR.Deadband` | Dödband spänningsreglering | % | HR 10–11 | M_ME_NC 4106 | C_SE_NC 8106 |
| `AVR.Delay` | Fördröjning spänningsreglering | s | HR 12–13 | M_ME_NC 4107 | C_SE_NC 8107 |
| `Load.Scale` | Simulering: lastskalning alla avgångar | % | HR 14–15 | M_ME_NC 4108 | C_SE_NC 8108 |
| `Env.Ambient` | Simulering: omgivningstemperatur | °C | HR 16–17 | M_ME_NC 4109 | C_SE_NC 8109 |
| `Station.CommonAlarm` | Summalarm | – | DI 113 | M_SP 2014 | – |
| `Station.Reset` | Återställ skyddsutlösningar och blockeringar | – | Coil 44 | – | C_SC 6005 |

## Struktur

```
cmd/blue-station/     main, flaggor
internal/station/     stationsmodell, lastflöde, skydd, AVR, signallista
internal/modbus/      Modbus TCP-server
internal/iec104/      IEC 60870-5-104-server (slav)
internal/web/         webbgränssnitt och JSON-API
```
