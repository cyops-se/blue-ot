**Svenska** · [English](README-en.md)

# Blue CHP – simulator för kraftvärmeverk

Ett kraftvärmeverk skrivet i Go (bara standardbiblioteket). Det består av två pannor, en mottrycksturbin med generator och ett fjärrvärmenät med värmeackumulator. Generatorn är ansluten till [blue-station](../blue-station/README.md) i fack G1 på 20 kV-skenans sektion B. Verket kan övervakas och styras via **Modbus TCP** och **IEC 60870-5-104** och har ett webbgränssnitt, precis som blue-station.

```
   [P1 flis 120 MW]   [P2 bioolja 50 MW]       pannor
          |                   |
   =======+===================+=======        ångsamlare 90 bar
          |                        \
      [turbin]--(G)--[Gen.CB]------- 20 kV --> blue-station fack G1
          |                          \
      kondensor <------------------ bypass (reduceringsventil)
          |
   fjärrvärmenät <==> värmeackumulator 400 MWh
     [pump 1] [pump 2]
```

## Köra

```sh
go run ./cmd/blue-chp                    # Modbus :1502, IEC 104 :2405, webb och länk :8082
go run ./cmd/blue-chp -timescale 60      # dygnsprofilen går 60 gånger snabbare
go run ./cmd/blue-chp -points            # skriv ut signallistan
go test ./...
```

| Flagga | Standard | Beskrivning |
|---|---|---|
| `-modbus` | `:1502` | Lyssnaradress för Modbus TCP (tom = av) |
| `-iec104` | `:2405` | Lyssnaradress för IEC 104 (tom = av) |
| `-http` | `:8082` | Webbgränssnitt, JSON-API och länk till blue-station (tom = av) |
| `-ca` | `2` | IEC 104 gemensam adress (CA) |
| `-iec-utc` | `false` | Tidsstämplar (CP56Time2a) i UTC i stället för lokal tid |
| `-tick` | `1s` | Simuleringens tidssteg |
| `-timescale` | `1` | Hastighet på dygnsklockan som styr värmebehovet |
| `-points`, `-points-json` | | Skriv ut signallistan som Markdown eller JSON och avsluta |

Portarna är valda så att blue-chp och blue-station kan köras på samma dator.

## Koppling till blue-station

Kopplingen motsvarar 20 kV-kabeln mellan verket och stationen. Det är inte ett SCADA-gränssnitt. blue-station anropar `POST /api/link` en gång per sekund med läget på anslutningspunkten:

```json
{"busLive": true, "busKV": 20.6, "hz": 50.01, "bayClosed": true}
```

Svaret är generatorns inmatning, som stationen lägger in i lastflödet för sektion B:

```json
{"genOnline": true, "p": 26.0, "q": 4.0}
```

- Om länken saknas eller har varit tyst i mer än 5 s kör verket fristående mot ett stelt nät (20,5 kV, 50 Hz). `Net.Link` visar då FRISTÅENDE.
- Generatorn kan inte driva en spänningslös skena (ingen ödrift). Om facket G1 öppnas eller sektion B blir spänningslös löser nätbortfallsskyddet (LOM) generatorbrytaren.
- blue-station kopplar upp sig mot `http://127.0.0.1:8082` som standard. Använd `-chp ""` för att köra stationen utan kraftvärme.

## Vad som simuleras

- **Pannor:**
  - P1 eldas med flis och har 120 MW bränsleeffekt och verkningsgrad 88 %. P2 eldas med bioolja och har 50 MW och 90 %.
  - Vid start tänds pannan och går upp till minimilast (35 %) på 2 minuter. Därefter ändras lasten med 0,5 %/s.
  - Ångproduktionen följer bränslet med en tidskonstant på 45 s.
  - Varje panna visar bränsleflöde, ångflöde, färskångtemperatur, O₂ och rökgastemperatur.
- **Lastreglering:** pannor i AUTO fördelar ångbehovet mellan sig. Behovet är värmebehovet plus en korrektion mot ackumulatorns börvärde, omräknat via turbinens elverkningsgrad när generatorn är inkopplad. I MANUELL följer pannan sitt lastbörvärde.
- **Ångsamlare:**
  - Turbinen går på inloppstrycksreglering och tar all ånga den kan, upp till 115 MW. Bypass öppnar 3 bar över börvärdet.
  - Larm ges vid 97 bar. Över 102 bar löser pannorna ut, och säkerhetsventilen öppnar vid 105 bar.
- **Turbin och generator:**
  - Mottrycksturbinen gör 30 % av ångans energi till el och resten till fjärrvärme, upp till drygt 33 MW el.
  - Uppstart till 3000 rpm tar 60 s. Generatorbrytaren kan bara slås till på nominellt varvtal och med spänning på anslutningspunkten (synkronisering).
  - Reaktiv effekt följer ett börvärde.
  - Turbinen löser ut vid ångtryck under 50 bar och vid snabbstopp.
- **Fjärrvärmenät:**
  - Värmebehovet beror på utetemperaturen (15 MW + 3,2 MW per grad under 17 °C) och en dygnsprofil med morgon- och kvällstopp.
  - Framledningens börvärde följer en utetemperaturkurva mellan 75 och 110 °C.
  - Flödet ger returtemperaturen. Två pumpar ger 550 kg/s vardera, och för lite pumpkapacitet ger lågt differenstryck och sämre leverans.
- **Värmeackumulator:** 400 MWh och ±60 MW. Den tar upp skillnaden mellan produktion och leverans och håller framledningstemperaturen. När den är full eller tom avviker temperaturen.
- **Utlösningar:**
  - Panna vid bränsle- eller brännarfel och vid högt ångtryck.
  - Turbin vid lågt tryck och vid snabbstopp.
  - Generator vid nätbortfall.
  - En utlösning blockerar start eller tillslag tills `Plant.Reset` har körts.

### Förslag på scenarier

1. Sätt `P1.FaultSim` = FEL. P1 löser ut. Starta P2 (`P2.Run`) och följ hur ackumulatorn laddas ur medan P2 går upp i last.
2. Kör turbinens snabbstopp (`Turbine.EmergencyStop`). Bypass tar över, elproduktionen upphör och värmeleveransen fortsätter. I blue-station går inmatningen i G1 till 0 och T2 tar hela lasten.
3. Gör som i scenario 2 men ställ först `Bypass.Auto` = MANUELL (stängd). Trycket stiger och pannorna löser ut på högt ångtryck.
4. Slå från `G1.CB` i blue-station. Nätbortfallsskyddet löser generatorn. Slå till G1, kör `Plant.Reset` och synkronisera med `Gen.CB`.
5. Sänk utetemperaturen (`Env.Outdoor` = −20 °C). Värmebehovet ökar, P1 når full last och P2 behöver startas.
6. Stoppa en fjärrvärmepump vid högt värmebehov. Differenstrycket faller och leveransen begränsas.

## Modbus TCP och IEC 104

Gränssnitten är desamma som i blue-station:

- **Modbus:** funktionskoderna 1, 2, 3, 4, 5, 15 och 16, float32 ABCD, och börvärden skrivs med FC16.
- **IEC 104:** generalförfrågan, spontana ändringar med tidsstämpel och kommandon med direct execute eller select/execute.

Adresserna följer samma uppdelning: brytare från coil 0 / IOA 1001, omkopplare från coil 20 / IOA 2101, pulskommandon från coil 40 / IOA 6001, status från DI 100 / IOA 2001, mätvärden från IR 0 / IOA 3001 och börvärden från HR 0 / IOA 4101 (styrning 8101).

## Webbgränssnitt och API

`http://localhost:8082` visar alla signaler grupperade, med reglage och en händelselogg.

- `GET /api/state` returnerar alla punkter och händelser.
- `POST /api/control` tar emot `{"id": "P2.Run", "on": true}` eller `{"id": "Env.Outdoor", "value": -15}`.
- `POST /api/link` är kopplingen till blue-station (se ovan).

## Signallista

| ID | Beskrivning | Enhet | Modbus TCP | IEC 104 övervakning | IEC 104 styrning |
|---|---|---|---|---|---|
| `P1.Run` | P1 start/stopp | – | Coil 20, DI 20 | M_SP 2101 | C_SC 6101 |
| `P1.Auto` | P1 lastreglering | – | Coil 21, DI 21 | M_SP 2102 | C_SC 6102 |
| `P1.Running` | P1 i drift | – | DI 100 | M_SP 2001 | – |
| `P1.Trip` | P1 pannskydd utlöst | – | DI 101 | M_SP 2002 | – |
| `P1.State` | P1 driftläge (0 stopp, 1 start, 2 drift, 3 stoppar) | – | IR 0–1 | M_ME_NC 3001 | – |
| `P1.Load` | P1 last | % | IR 2–3 | M_ME_NC 3002 | – |
| `P1.FuelPower` | P1 tillförd bränsleeffekt | MW | IR 4–5 | M_ME_NC 3003 | – |
| `P1.FuelFlow` | P1 bränsleflöde | t/h | IR 6–7 | M_ME_NC 3004 | – |
| `P1.SteamFlow` | P1 ångflöde | t/h | IR 8–9 | M_ME_NC 3005 | – |
| `P1.SteamTemp` | P1 färskångtemperatur | °C | IR 10–11 | M_ME_NC 3006 | – |
| `P1.O2` | P1 syrehalt rökgas | % | IR 12–13 | M_ME_NC 3007 | – |
| `P1.FlueTemp` | P1 rökgastemperatur | °C | IR 14–15 | M_ME_NC 3008 | – |
| `P1.LoadSet` | P1 lastbörvärde (manuell) | % | HR 0–1 | M_ME_NC 4101 | C_SE_NC 8101 |
| `P1.FaultSim` | Simulering: bränslematningsfel P1 | – | Coil 22, DI 22 | M_SP 2103 | C_SC 6103 |
| `P2.Run` | P2 start/stopp | – | Coil 23, DI 23 | M_SP 2104 | C_SC 6104 |
| `P2.Auto` | P2 lastreglering | – | Coil 24, DI 24 | M_SP 2105 | C_SC 6105 |
| `P2.Running` | P2 i drift | – | DI 102 | M_SP 2003 | – |
| `P2.Trip` | P2 pannskydd utlöst | – | DI 103 | M_SP 2004 | – |
| `P2.State` | P2 driftläge (0 stopp, 1 start, 2 drift, 3 stoppar) | – | IR 16–17 | M_ME_NC 3009 | – |
| `P2.Load` | P2 last | % | IR 18–19 | M_ME_NC 3010 | – |
| `P2.FuelPower` | P2 tillförd bränsleeffekt | MW | IR 20–21 | M_ME_NC 3011 | – |
| `P2.FuelFlow` | P2 bränsleflöde | t/h | IR 22–23 | M_ME_NC 3012 | – |
| `P2.SteamFlow` | P2 ångflöde | t/h | IR 24–25 | M_ME_NC 3013 | – |
| `P2.SteamTemp` | P2 färskångtemperatur | °C | IR 26–27 | M_ME_NC 3014 | – |
| `P2.O2` | P2 syrehalt rökgas | % | IR 28–29 | M_ME_NC 3015 | – |
| `P2.FlueTemp` | P2 rökgastemperatur | °C | IR 30–31 | M_ME_NC 3016 | – |
| `P2.LoadSet` | P2 lastbörvärde (manuell) | % | HR 2–3 | M_ME_NC 4102 | C_SE_NC 8102 |
| `P2.FaultSim` | Simulering: brännarfel P2 | – | Coil 25, DI 25 | M_SP 2106 | C_SC 6106 |
| `Steam.Pressure` | Tryck ångsamlare | bar | IR 32–33 | M_ME_NC 3017 | – |
| `Steam.Flow` | Total ångproduktion | t/h | IR 34–35 | M_ME_NC 3018 | – |
| `Steam.HighPressure` | Högt ångtryck (>97 bar) | – | DI 104 | M_SP 2005 | – |
| `Steam.PressureSet` | Börvärde ångtryck | bar | HR 4–5 | M_ME_NC 4103 | C_SE_NC 8103 |
| `Bypass.Auto` | Bypassventil reglering | – | Coil 26, DI 26 | M_SP 2107 | C_SC 6107 |
| `Bypass.Position` | Bypassventil läge | % | IR 36–37 | M_ME_NC 3019 | – |
| `Bypass.Flow` | Ångflöde bypass | t/h | IR 38–39 | M_ME_NC 3020 | – |
| `Bypass.ManualPos` | Bypassventil läge vid manuell | % | HR 6–7 | M_ME_NC 4104 | C_SE_NC 8104 |
| `Turbine.Run` | Turbin start/stopp | – | Coil 27, DI 27 | M_SP 2108 | C_SC 6108 |
| `Turbine.Speed` | Turbinvarvtal | rpm | IR 40–41 | M_ME_NC 3021 | – |
| `Turbine.SteamFlow` | Ångflöde turbin | t/h | IR 42–43 | M_ME_NC 3022 | – |
| `Turbine.AtSpeed` | Turbin på nominellt varvtal | – | DI 105 | M_SP 2006 | – |
| `Turbine.Trip` | Turbinskydd utlöst | – | DI 106 | M_SP 2007 | – |
| `Turbine.EmergencyStop` | Turbin snabbstopp | – | Coil 40 | – | C_SC 6001 |
| `Gen.CB` | Generatorbrytare (synkronisering vid tillslag) | – | Coil 0, DI 0 | M_DP 1001 | C_DC 5001 |
| `Gen.P` | Generator aktiv effekt | MW | IR 44–45 | M_ME_NC 3023 | – |
| `Gen.Q` | Generator reaktiv effekt | Mvar | IR 46–47 | M_ME_NC 3024 | – |
| `Gen.U` | Generatorspänning | kV | IR 48–49 | M_ME_NC 3025 | – |
| `Gen.f` | Generatorfrekvens | Hz | IR 50–51 | M_ME_NC 3026 | – |
| `Gen.I` | Generatorström | A | IR 52–53 | M_ME_NC 3027 | – |
| `Gen.Energy` | Producerad elenergi | MWh | IR 54–55 | M_ME_NC 3028 | – |
| `Gen.Trip` | Generatorskydd utlöst (nätbortfall) | – | DI 107 | M_SP 2008 | – |
| `Gen.QSet` | Börvärde reaktiv effekt | Mvar | HR 8–9 | M_ME_NC 4105 | C_SE_NC 8105 |
| `Net.Link` | Länk till blue-station | – | DI 108 | M_SP 2009 | – |
| `Net.Live` | Spänning på anslutningspunkten | – | DI 109 | M_SP 2010 | – |
| `Net.U` | Spänning anslutningspunkt | kV | IR 56–57 | M_ME_NC 3029 | – |
| `Net.f` | Frekvens anslutningspunkt | Hz | IR 58–59 | M_ME_NC 3030 | – |
| `DH.Demand` | Värmebehov nät (inkl. förluster) | MW | IR 60–61 | M_ME_NC 3031 | – |
| `DH.Production` | Värmeproduktion (kondensor + bypass) | MW | IR 62–63 | M_ME_NC 3032 | – |
| `DH.Delivered` | Levererad värme till nätet | MW | IR 64–65 | M_ME_NC 3033 | – |
| `DH.SupplyTemp` | Framledningstemperatur | °C | IR 66–67 | M_ME_NC 3034 | – |
| `DH.SupplyTempSet` | Börvärde framledning (utetemperaturkurva) | °C | IR 68–69 | M_ME_NC 3035 | – |
| `DH.ReturnTemp` | Returtemperatur | °C | IR 70–71 | M_ME_NC 3036 | – |
| `DH.Flow` | Flöde fjärrvärmenät | m³/h | IR 72–73 | M_ME_NC 3037 | – |
| `DH.DiffPressure` | Differenstryck kritisk kund | bar | IR 74–75 | M_ME_NC 3038 | – |
| `DH.Pump1` | Fjärrvärmepump 1 | – | Coil 28, DI 28 | M_SP 2109 | C_SC 6109 |
| `DH.Pump2` | Fjärrvärmepump 2 | – | Coil 29, DI 29 | M_SP 2110 | C_SC 6110 |
| `DH.LowDP` | Lågt differenstryck i nätet (<0,6 bar) | – | DI 110 | M_SP 2011 | – |
| `DH.SupplyLow` | Låg framledningstemperatur (>8 °C under börvärde) | – | DI 111 | M_SP 2012 | – |
| `DH.SupplyHigh` | Hög framledningstemperatur (>120 °C) | – | DI 112 | M_SP 2013 | – |
| `Acc.Energy` | Lagrad energi | MWh | IR 76–77 | M_ME_NC 3039 | – |
| `Acc.Level` | Laddningsgrad | % | IR 78–79 | M_ME_NC 3040 | – |
| `Acc.Power` | Laddeffekt (+ laddning, − urladdning) | MW | IR 80–81 | M_ME_NC 3041 | – |
| `Acc.LevelSet` | Börvärde laddningsgrad | % | HR 10–11 | M_ME_NC 4106 | C_SE_NC 8106 |
| `Env.Outdoor` | Simulering: utetemperatur | °C | HR 12–13 | M_ME_NC 4107 | C_SE_NC 8107 |
| `Heat.Scale` | Simulering: skalning värmebehov | % | HR 14–15 | M_ME_NC 4108 | C_SE_NC 8108 |
| `Plant.CommonAlarm` | Summalarm | – | DI 113 | M_SP 2014 | – |
| `Plant.Reset` | Återställ utlösningar och blockeringar | – | Coil 41 | – | C_SC 6002 |
## Struktur

```
cmd/blue-chp/      main, flaggor
internal/plant/    processmodell: pannor, ångsamlare, turbin, generator, fjärrvärme, signallista
internal/modbus/   Modbus TCP-server
internal/iec104/   IEC 60870-5-104-server (slav)
internal/web/      webbgränssnitt, JSON-API och länk till blue-station
```
