**Svenska** · [English](README-en.md)

# Blue OT SCADA

SCADA-delen av [Blue OT](../README.md) består av en Go-backend som pollar Modbus TCP-enheter och ett webbaserat HMI i Vue 3 och Vuetify 3. HMI:t byggs in i backendens exe-fil, så samma program servar REST-API, WebSocket och webbgränssnitt. Till projektet hör också **modsim**, en liten Modbus-simulator för att testa systemet utan annan utrustning.

Detta ingår:

- **Driftbilder** med statisk grafik och dynamiska symboler: brytare, frånskiljare, omkopplare, mätvärden, staplar, indikeringar, knappar och vy-länkar. Värdena uppdateras i realtid över WebSocket.
- **Manöver** av brytare, frånskiljare, omkopplare och börvärden. Varje manöver kräver ett val och sedan en bekräftelse (select-before-operate), och dialogen visar när läget har ändrats.
- **Vy-editor** i webbläsaren. Där ritar du bilder och kopplar varje symbol till de taggar och värden den ska visa.
- **Mätvärdestabell** med flyttal, kvalitet och tidsstämpel, trend per tagg och direkt skrivning till skrivbara taggar.
- **Händelselista** med manövrar, lägesändringar, kommunikationsstatus och konfigurationsändringar.
- **Konfiguration** av enheter och taggar i HMI:t. Taggar kan ha alla Modbus-områden, flera datatyper, valfri ordning på ord och byte samt skala och offset.

## Arkitektur

```
 webbläsare ── HTTP / WebSocket ──► scada.exe ── Modbus TCP ──► enheter (modsim, blue-station, blue-chp …)
                                       │
                                       └── data/ (config.json, views/*.json, images/)
```

| Del | Innehåll |
|---|---|
| `backend/cmd/scada` | Huvudprogram: startar pollern, HTTP-servern och det inbäddade HMI:t |
| `backend/cmd/modsim` | Modbus TCP-simulator (se [modsim](#modsim)) |
| `backend/cmd/opctl` | Kommandoradsverktyg för att manövrera brytare och frånskiljare (se [opctl](#opctl)) |
| `backend/internal/mbus` | En poller per enhet. Den slår ihop närliggande adresser till få läsningar, avkodar datatyper, återansluter vid fel och skriver vid manöver |
| `backend/internal/realtime` | Processbild, historik i minnet (3600 värden per tagg), händelselista (1000 senaste) och utskick till WebSocket-klienter |
| `backend/internal/store` | JSON-lagring av enheter, taggar, vyer och uppladdade bilder |
| `backend/internal/api` | REST-API och WebSocket |
| `backend/internal/web` | Inbäddat HMI (`go:embed` av `dist/`, som skrivs av frontend-bygget) |
| `backend/tools` | Skript som genererar enheter, taggar och vyer för blue-station och blue-chp |
| `frontend/` | HMI i Vue 3, Vuetify 3, Pinia och Vite |

## Bygga och köra

Krav: Go 1.21 eller senare och Node 18 eller senare.

```bash
cd frontend
npm install
npm run build                    # skriver till backend/internal/web/dist

cd ../backend
go build -o bin/ ./cmd/scada ./cmd/modsim ./cmd/opctl
./bin/modsim                     # Modbus-simulator på :5020
./bin/scada -data data           # HMI och API på http://localhost:8080
```

Exe-filen `scada` behöver bara katalogen `data/`. Bygg HMI:t före `go build`, eftersom ändringar i frontend kommer med först när backend byggs om. Byggs backend utan HMI startar den ändå, men webbgränssnittet svarar då med ett meddelande om att HMI saknas.

**Utveckling av HMI:** `npm run dev` i `frontend/` startar Vites dev-server på http://localhost:5173, som laddar om ändringar direkt. Anrop till `/api`, `/ws` och `/images` skickas vidare till backend på :8080, och med miljövariabeln `BACKEND` kan du ange en annan adress.

| Flagga (`scada`) | Standard | Beskrivning |
|---|---|---|
| `-addr` | `:8080` | HTTP-adress |
| `-data` | `data` | Datakatalog med `config.json`, `views/` och `images/` |
| `-web` | – | Servera HMI från en katalog på disk i stället för det inbäddade bygget |

## Användning

### Driftbild (`/views/:id`)

- **Brytare, frånskiljare och omkopplare:** klicka för att öppna manöverdialogen. Välj *Frånslag/Tillslag* (eller *Öppna/Slut*, *PÅ/AV*) och bekräfta med *Utför*. Dialogen väntar på att läget ändras och stängs då. Om enheten avvisar manövern, t.ex. vid förregling eller blockering, visas felet i dialogen.
- **Kommandoknappar:** skriver ett fast värde, eller frågar efter ett värde (börvärde).
- **Mätvärden och staplar:** klicka för att se en trend över de senaste 5–60 minuterna.
- **Kvalitet:** värden från en enhet utan kommunikation visas lila med `???`, och symboler visas med `?`.

### Vy-editor (`/views/:id/edit`)

- Dra symboler från paletten till ritytan, eller dubbelklicka på en symbol för att lägga den i mitten.
- Markera ett element för att ändra position, storlek, rotation, färger och **taggkoppling** i egenskapspanelen. Taggen väljs i en sökbar lista, och för manöver visas bara skrivbara taggar.
- Linjer och samlingsskenor har handtag i båda ändar. Övriga element har ett storlekshandtag nere till höger.
- Kortkommandon: `Del` tar bort, `Ctrl+D` duplicerar, piltangenterna flyttar (`Shift` ger större steg, `Alt` 1 px), `Ctrl+Z` ångrar och `Ctrl+S` sparar.
- Bilder (png, jpg, svg, webp) laddas upp via bildelementet och lagras i `data/images/`.

| Symbol | Konfiguration |
|---|---|
| Linje, samlingsskena, rektangel, ellips, text, transformator | Färg, linjebredd och **dynamisk färg**: en tagg och regler (`==`, `!=`, `>`, `>=`, `<`, `<=`), där första matchande regel gäller |
| Bild, jord | Statisk grafik |
| Brytare, frånskiljare | Lägestagg och värden för öppen och sluten, manövertagg och värden för frånslag och tillslag, färger |
| Omkopplare | Som brytare, men med egna texter för PÅ och AV (t.ex. AUTO/MAN, FEL/AV) |
| Mätvärde | Tagg, etikett, enhet och decimaler (som standard från taggen), gränsvärden med larmfärg |
| Indikering | Tagg, aktivt värde, färger, blinkning |
| Stapel | Tagg, min och max. Fylls vågrätt om stapeln är bredare än hög |
| Kommandoknapp | Skrivbar tagg och värde. Med "Fråga efter värde" fungerar den som börvärdesinmatning |
| Vy-länk | Navigering till en annan vy |

### Mätvärden, händelser och konfiguration

- **Mätvärden (`/values`):** alla taggar med värde, kvalitet och tid för senaste ändring. Klicka på en rad för att se trenden. Skrivbara taggar har en knapp för att skriva ett värde.
- **Händelser (`/events`):** filtreras på manövrar, lägesändringar, kommunikation och system. Lägesändringar loggas för taggar med "Logga ändringar" och visas med taggens lägestexter, t.ex. "Till → Från".
- **Konfiguration (`/config`):**
  - **Enheter:** IP-adress och port, unit id, pollintervall, timeout och aktiv/inaktiv.
  - **Taggar:** område (coil, discrete, input, holding), adress (0-baserad), datatyp (`bool`, `uint16`, `int16`, `uint32`, `int32`, `float32`, `float64`), ord- och bytebyte, skala och offset, bit i register, skrivbar, händelseloggning och lägestexter.
  - Ändringar sparas i `data/config.json`, och pollern startas om direkt.

## Medföljande enheter och vyer

| Vy | Enhet | Adress | Taggar | Beskrivning |
|---|---|---|---|---|
| Station Norra | `sim` | 127.0.0.1:5020 | 31 | 10 kV-ställverk med fyra fack (modsim) |
| Blue Station | `blue-station` | 127.0.0.1:502 | 107 | Transformatorstation 220/20 kV med fack G1 för kraftvärme, se [blue-station](../blue-station/README.md) |
| Blue CHP | `blue-chp` | 127.0.0.1:1502 | 87 (prefix `CHP.`) | Kraftvärmeverk med fjärrvärme, se [blue-chp](../blue-chp/README.md) |

Taggar och vyer för blue-station och blue-chp genereras av skript i `backend/tools/`. Skripten körs i `backend/`:

```bash
python tools/gen_blue_station.py   # enheten blue-station, taggar och vyn Blue Station
python tools/gen_blue_chp.py       # enheten blue-chp, taggar och vyn Blue CHP (läser signallistan via go run … -points-json)
```

Skripten ersätter respektive enhets taggar i `config.json` och skriver över vyfilen. Ändringar du gjort i de vyerna i editorn försvinner alltså om du kör skriptet igen.

Taggkonventioner för simulatorerna:

- Brytare och omkopplare har två taggar: lägestaggen läser discrete input (`F1.CB`) och manövertaggen skriver coil (`F1.CB.Cmd`).
- Pulskommandon, t.ex. `Station.Reset` och `CHP.Turbine.EmergencyStop`, är coils som skrivs med 1.
- Mätvärden och börvärden är float32 (ABCD) i input- respektive holdingregister.

## modsim

`modsim` simulerar ett 10 kV-ställverk med fyra fack. Varje fack har en brytare Q1–Q4, en frånskiljare QS1–QS4 och mätvärden för U, I och P. Dessutom finns frekvens, spänning på samlingsskenan och oljetemperatur för en transformator. Simulatorn visar två vanliga sätt att representera kopplingsapparater i Modbus:

- **Brytare:** läget är ett tvåbitsvärde i ett holdingregister (0 = mellanläge, 1 = öppen, 2 = sluten, 3 = fel), och manövern sker genom att ett kommandoregister skrivs.
- **Frånskiljare:** läget är en discrete input, och manövern sker via en coil.

Frånskiljarna är förreglade: de kan bara manövreras när facket har öppen brytare, annars svarar simulatorn med undantaget *illegal data value*.

```bash
go run ./cmd/modsim                         # lyssnar på 0.0.0.0:5020
go run ./cmd/modsim -addr :1503 -delay 3s   # annan port, brytarens manövertid 3 s
```

| Område | Adress | Innehåll |
|---|---|---|
| Holding | 100–103 | Läge för brytare Q1–Q4 (0 = mellanläge, 1 = öppen, 2 = sluten, 3 = fel) |
| Holding | 200–203 | Manöver för Q1–Q4 (skriv 1 = frånslag, 2 = tillslag) |
| Discrete | 0–3 | Läge för frånskiljare QS1–QS4 (1 = sluten) |
| Coil | 0–3 | Manöver för QS1–QS4 (1 = slut, 0 = öppna), förreglad mot brytaren |
| Input | 6n, 6n+2, 6n+4 | Fack n (0–3): U kV, I A, P MW (float32, ABCD) |
| Input | 30 / 32 | Frekvens Hz / spänning på samlingsskenan kV (float32) |
| Input | 40 | Oljetemperatur T1 (int16, skala 0,1 °C) |

Unit id ignoreras. Vyn **Station Norra** och enheten `sim` i `data/config.json` är konfigurerade för modsim.

## opctl

`opctl` manövrerar en brytare eller frånskiljare från kommandoraden. Taggen och det önskade tillståndet anges som argument, och allt annat (värd, port, Modbus-adress eller IEC 104-styr-IOA) läses från SCADA:s konfigurationsfil. Både **Modbus TCP** och **IEC 60870-5-104** stöds.

```bash
opctl [flaggor] <tagg> <open|close>
opctl -list [brytare|frånskiljare|pump]
```

- **Tagg:** lägestaggen (`G1.CB`, `Q1_POS`) eller manövertaggen (`G1.CB.Cmd`, `Q1_CMD`). Verktyget hittar den andra av de två och enheten utifrån konfigurationen.
- **Tillstånd:** `open`/`close`, `från`/`till`, `öppna`/`slut`, `0`/`1`. För pumpar betyder `close` = starta (drift) och `open` = stoppa.

```bash
cd backend
opctl -list                            # lista alla brytare, frånskiljare och pumpar
opctl -list pump                       # bara pumpar
opctl G1.CB close                      # Modbus TCP, läser data/config.json
opctl -protocol iec104 Q0.CB open      # IEC 104
opctl -data ../annan/data QS1_POS open # annan datakatalog
```

Med `-list` skrivs en tabell över kända brytare, frånskiljare och pumpar, vilken enhet de hör till, vilka protokoll de kan manövreras med och deras manövertagg. Typen gissas från taggens id och namn (konfigurationen har ingen uttrycklig typ). Ett valfritt filter (`brytare`, `frånskiljare` eller `pump`) begränsar listan.

| Flagga | Standard | Beskrivning |
|---|---|---|
| `-list` | `false` | Lista brytare, frånskiljare och pumpar (valfritt filter som argument) |
| `-proxy` | – | Adress till en blue-proxy för frys/block-åtgärder (se nedan) |
| `-category` | – | Med `-proxy`: tillämpa åtgärden på en hel typ (måste stå före åtgärden) |
| `-protocol` | `modbus` | `modbus` eller `iec104` |
| `-data` | `data` | Datakatalog eller sökväg direkt till `config.json` |
| `-timeout` | `5s` | Tidsgräns per anrop |
| `-host` | – | Överskrid IP-adressen/värdnamnet från konfigurationen |
| `-port` | – | Överskrid porten från konfigurationen (Modbus eller IEC 104 beroende på `-protocol`) |
| `-open-value` / `-close-value` | – | Överskrid Modbus-värdet som skrivs (standard: coil 0/1, holding 1/2) |
| `-unit` | – | Överskrid unit id från konfigurationen |
| `-verify` | `true` | Läs tillbaka lägestaggen efter manövern (Modbus) och vänta tills läget nåtts |
| `-dry-run` | `false` | Visa vad som skulle göras utan att ansluta |

**Hur informationen hämtas:**

- **Modbus:** enhetens `host`/`port`/`unitId` och manövertaggens område och adress. En brytare eller frånskiljare har en lägestagg och en manövertagg (`…​.Cmd` eller `…_CMD`); verktyget skriver till manövertaggen. Standardvärden följer simulatorernas konventioner: coil 0 = öppna, 1 = slut; holdingregister 1 = frånslag, 2 = tillslag. Efter manövern läses lägestaggen tills läget har nåtts.
- **IEC 104:** enhetens `iec104`-block (`port`, `ca`) och lägestaggens `ctrlIoa` och `cmd` (`C_DC` för brytare, `C_SC` för omkopplare). Verktyget ansluter, gör STARTDT, skickar kommandot med direct execute och väntar på ACTCON och ACTTERM.

`iec104`, `ctrlIoa` och `cmd` i `config.json` fylls i av generatorskripten för blue-station och blue-chp (se ovan). Backenden pollar bara över Modbus och bryr sig inte om fälten. modsim har inget IEC 104-gränssnitt, så `sim`-taggarna kan bara manövreras över Modbus.

### Styra en blue-proxy

Med `-proxy <url>` använder opctl i stället HTTP-API:et hos en [blue-proxy](../blue-proxy/README.md) för att frysa rapporterade värden och blockera manövrar, på samma sätt som proxyns webbgränssnitt.

```bash
opctl -proxy http://127.0.0.1:8083 status                 # visa alla punkter, läge och senaste manöver
opctl -proxy http://127.0.0.1:8083 freeze Grid.U          # frys en tagg
opctl -proxy http://127.0.0.1:8083 unfreeze Grid.U        # släpp
opctl -proxy http://127.0.0.1:8083 block Q0.CB            # blockera manövrar mot en tagg
opctl -proxy http://127.0.0.1:8083 unblock Q0.CB          # tillåt igen
opctl -proxy http://127.0.0.1:8083 block type:brytare     # hela typen
opctl -proxy http://127.0.0.1:8083 unfreeze all           # alla punkter
```

Åtgärder: `freeze`, `unfreeze`, `block`, `unblock` och `status`. Målet är en tagg, `all` eller `type:<typ>` (t.ex. `type:mätvärde`). Alternativt anges `-category <typ>` före åtgärden.

## REST-API och WebSocket

| Metod | Sökväg | Beskrivning |
|---|---|---|
| GET | `/api/config` | Enheter och taggar |
| GET/POST | `/api/devices` | Lista eller skapa enheter |
| PUT/DELETE | `/api/devices/{id}` | Ändra eller ta bort en enhet |
| GET/POST | `/api/tags` | Lista eller skapa taggar |
| PUT/DELETE | `/api/tags/{id}` | Ändra eller ta bort en tagg |
| GET | `/api/values` | Ögonblicksbild av alla värden |
| GET | `/api/status` | Kommunikationsstatus per enhet |
| GET | `/api/history/{tag}?minutes=10` | Historik `[{t, v}]` |
| GET | `/api/events` | Händelselista, nyast först |
| POST | `/api/commands` | Manöver: `{"tag":"Q1_CMD","value":1,"source":"Q1","text":"Frånslag"}` |
| GET/POST | `/api/views` | Lista eller skapa vyer |
| GET/PUT/DELETE | `/api/views/{id}` | Läs, spara eller ta bort en vy |
| GET/POST | `/api/images` | Lista eller ladda upp bilder (multipart, fältet `file`) |
| WS | `/ws` | Först `snapshot` med alla värden och enheter, därefter `values`, `device` och `event` vid ändringar |

Värden har formen `{"tag": "...", "v": 10.5, "q": "good" | "bad", "ts": "..."}`. Booleska värden skickas som 0 och 1.

## Begränsningar

- Det finns ingen inloggning eller behörighetsstyrning, och alla manövrar loggas som "operatör". Lägg till autentisering innan systemet används mot riktig utrustning.
- Historiken finns bara i minnet, 1 timme per tagg vid 1 s polling. För längre lagring behövs en tidsseriedatabas.
- Det finns ingen larmhantering med kvittering. Gränsvärden på mätvärdessymboler påverkar bara färgen.
- Bara Modbus TCP stöds som klientprotokoll. Simulatorerna har även IEC 60870-5-104, men SCADA använder det inte.
