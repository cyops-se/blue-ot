# Blue OT SCADA

En SCADA-liknande applikation med:

- **Backend (Go)**: pollar Modbus TCP-enheter, håller en realtidsdatabas med kort historik, händelselista, REST-API och WebSocket-push.
- **HMI (Vue 3 + Vuetify 3)**: driftbilder med statisk grafik och dynamiska symboler, vy-editor, manöver av brytare och frånskiljare, mätvärdestabell med trender, händelselista och konfiguration av enheter och taggar.
- **Modbus-simulator** (`modsim`): ett ställverk med fyra fack, så att allt kan testas utan riktig utrustning.

```
blue-ot/
├── blue-station/            simulator för transformatorstation (Modbus TCP / IEC 104)
└── scada/
    ├── backend/
    │   ├── cmd/scada/        huvudprogram (HTTP + Modbus-poller + inbäddat HMI)
    │   ├── cmd/modsim/       Modbus TCP-simulator
    │   ├── internal/model    datatyper
    │   ├── internal/store    JSON-lagring (config.json, views/*.json, images/)
    │   ├── internal/realtime processbild, historik, händelser, pub/sub
    │   ├── internal/mbus     Modbus-polling, avkodning, skrivning (manöver)
    │   ├── internal/api      REST + WebSocket
    │   ├── internal/web      inbäddat HMI (go:embed av dist/, byggs av frontend)
    │   └── data/             exempelkonfiguration och vyerna "Station Norra" och "Blue Station"
    └── frontend/             Vuetify-HMI (Vite)
```

## Kom igång

Krav: Go 1.21+ och Node 18+.

### Bygga en exe-fil

Webbgränssnittet byggs in i backendens exe-fil, så att samma program servar både REST-API, WebSocket och HMI.

```bash
cd scada/frontend
npm install
npm run build          # skriver till scada/backend/internal/web/dist

cd ../backend
go build -o bin/ ./cmd/scada ./cmd/modsim
```

`bin/scada` (`scada.exe` på Windows) behöver sedan bara katalogen `data/` bredvid sig, eller en sökväg via `-data`. Kör HMI-bygget före `go build`, eftersom ändringar i frontend märks först när backend byggs om. Om backend byggs utan frontend-bygge startar den ändå, men webbgränssnittet svarar då med ett meddelande om att HMI saknas.

### Köra

```bash
# 1. Simulatorn (Modbus TCP på port 5020)
cd scada/backend
go run ./cmd/modsim

# 2. SCADA (i ett nytt fönster): HMI och API på http://localhost:8080
cd scada/backend
go run ./cmd/scada -data data
```

### Utveckling av HMI

Med Vites dev-server laddas ändringar i webbläsaren direkt. Anrop till `/api`, `/ws` och `/images` skickas vidare till backend på :8080.

```bash
cd scada/frontend
npm run dev            # http://localhost:5173
```

Flaggor för `scada`: `-addr :8080`, `-data data`, `-web <katalog>`. Med `-web` serveras HMI:t från en katalog på disk i stället för det inbäddade bygget, till exempel `-web internal/web/dist`.
Flaggor för `modsim`: `-addr 0.0.0.0:5020`, `-delay 1.5s` (brytarens manövertid).

## Användning

### Driftbild
- **Brytare och frånskiljare**: klicka för att öppna manöverdialogen. Den följer select-before-operate: först väljer du *Frånslag/Tillslag* (*Öppna/Slut*), sedan bekräftar du med *Utför*. Dialogen väntar sedan på att läget ändras. Om enheten avvisar manövern visas felet i dialogen.
- **Mätvärden och staplar**: klicka för att visa en trend (5–60 min ur backendens minneshistorik).
- **Kvalitet**: värden med dålig kvalitet visas lila med `???` och symboler med `?`.

### Vy-editor (`/views/:id/edit`)
- Dra symboler från paletten till ritytan, eller dubbelklicka på en symbol för att lägga den i mitten.
- Markera ett element för att ändra position, storlek, rotation, färger och **taggkoppling** i egenskapspanelen.
- Linjer och samlingsskenor har handtag i båda ändar. Övriga element har ett storlekshandtag nere till höger.
- Kortkommandon: `Del` tar bort, `Ctrl+D` duplicerar, piltangenterna flyttar (`Shift` ger större steg, `Alt` 1 px), `Ctrl+Z` ångrar och `Ctrl+S` sparar.
- Bilder (png, jpg, svg, webp) laddas upp via bildelementet och lagras i `data/images/`.

| Symbol | Konfiguration |
|---|---|
| Linje, samlingsskena, rektangel, ellips, text, transformator | Färg, linjebredd och **dynamisk färg**: tagg och regler (`==`, `>`, `<` …), där första matchande regel gäller |
| Bild, jord | Statisk grafik |
| Brytare / frånskiljare | Lägestagg och värden för öppen/sluten, manövertagg och värden för frånslag/tillslag, färger |
| Mätvärde | Tagg, etikett, enhet och decimaler (som standard från taggen), gränsvärden med larmfärg |
| Indikering | Tagg, aktivt värde, färger, blinkning |
| Stapel | Tagg, min/max. Fylls vågrätt om stapeln är bredare än hög |
| Kommandoknapp | Skrivbar tagg och värde. Med "Fråga efter värde" fungerar den som börvärdesinmatning |
| Omkopplare | Läges- och manövertagg med egna texter för PÅ och AV (t.ex. AUTO/MAN, FEL/AV) |
| Vy-länk | Navigering till en annan vy |

### Konfiguration (`/config`)
- **Enheter**: IP-adress/port, unit id, pollintervall och timeout.
- **Taggar**: område (coil, discrete, input, holding), adress (0-baserad), datatyp (`bool`, `uint16`, `int16`, `uint32`, `int32`, `float32`, `float64`), ord- och bytebyte, skala och offset, bit i register, skrivbar, händelseloggning och lägestexter.

Ändringar sparas i `data/config.json` och pollern startas om direkt.

## Simulatorns registerkarta

| Område | Adress | Innehåll |
|---|---|---|
| Holding | 100–103 | Läge för brytare Q1–Q4 (0 = mellanläge, 1 = öppen, 2 = sluten, 3 = fel) |
| Holding | 200–203 | Manöver för Q1–Q4 (skriv 1 = frånslag, 2 = tillslag) |
| Discrete | 0–3 | Läge för frånskiljare QS1–QS4 (1 = sluten) |
| Coil | 0–3 | Manöver för QS1–QS4 (1 = slut, 0 = öppna). Förreglad: avvisas om brytaren inte är öppen |
| Input | 6n, 6n+2, 6n+4 | Fack n (0–3): U kV, I A, P MW (float32, ABCD) |
| Input | 30 / 32 | Frekvens Hz / spänning på samlingsskenan kV (float32) |
| Input | 40 | T1 oljetemperatur (int16, skala 0,1) |

## Vyn Blue Station

Vyn **Blue Station** (`data/views/blue-station.json`) är byggd på signallistan i [`blue-station/README.md`](blue-station/README.md). Den visar en transformatorstation 220/20 kV med följande delar:

- Inkommande stamnät med linjebrytare Q0, mätvärden U, f, P, Q och ström genom Q0.
- Transformatorerna T1 och T2. Varje transformator har brytare på upp- och nedsidan, P, Q, strömmar, belastning (med stapel) och oljetemperatur, med larmgräns vid 90 °C. Där finns också lindningskopplarläge, knappar för att höja och sänka, omkopplare för AVR auto/manuell samt indikeringar för överlast, hög oljetemperatur och skyddsutlösning.
- 20 kV-skenans sektioner A och B med sektioneringsbrytare och spänningsmätning. Skenor och ledningar färgas efter om de är spänningssatta.
- Avgångarna F1–F4. Varje avgång har brytare, P, Q, I, energi, indikering för överströmsutlösning, inställning av I> (börvärde) och omkopplare för felsimulering.
- En panel för station och spänningsreglering med summalarm, Återställ (`Station.Reset`) och börvärden för AVR, och en simuleringspanel med matning från stamnätet, lastskalning och omgivningstemperatur.

Enheten `blue-station` (127.0.0.1:502) och dess 99 taggar finns i `data/config.json`. Brytare och omkopplare har två taggar: lägestaggen läser discrete input (till exempel `F1.CB`) och manövertaggen skriver coil (`F1.CB.Cmd`). Pulskommandon (`T1.Tap.Raise`, `Station.Reset` …) är skrivbara coils som skrivs med 1, och börvärden är float32 i holdingregister. Konfigurationen och vyn genereras av `backend/tools/gen_blue_station.py`. Kör `python tools/gen_blue_station.py` i `scada/backend/` för att skapa dem på nytt.

```bash
cd blue-station && go run ./cmd/blue-station -http :8090   # Modbus :502; webben flyttas från :8080
cd scada/backend && go run ./cmd/scada -data data
```

Scenarierna i blue-stations README (fel på avgång → utlösning → Återställ → tillslag, sektionering, överlast, AVR) kan köras direkt från vyn.

## REST-API

| Metod | Sökväg | Beskrivning |
|---|---|---|
| GET | `/api/config` | Enheter och taggar |
| POST/PUT/DELETE | `/api/devices[/{id}]` | Hantera enheter |
| POST/PUT/DELETE | `/api/tags[/{id}]` | Hantera taggar |
| GET | `/api/values` | Ögonblicksbild av alla värden |
| GET | `/api/status` | Kommunikationsstatus per enhet |
| GET | `/api/history/{tag}?minutes=10` | Historik `[{t, v}]` |
| GET | `/api/events` | Händelselista (nyast först) |
| POST | `/api/commands` | `{"tag":"Q1_CMD","value":1,"source":"Q1","text":"Frånslag"}` |
| GET/POST | `/api/views` | Lista eller skapa vyer |
| GET/PUT/DELETE | `/api/views/{id}` | Läs, spara eller ta bort en vy |
| GET/POST | `/api/images` | Lista eller ladda upp bilder (multipart, fält `file`) |
| WS | `/ws` | `snapshot` vid anslutning, därefter `values`, `device` och `event` |

## Begränsningar och nästa steg
- Ingen inloggning eller behörighetsstyrning. Manövrar loggas som "operatör". Lägg till autentisering innan systemet används mot riktig utrustning.
- Historiken finns bara i minnet (1 h per tagg vid 1 s polling). För längre lagring behövs en tidsseriedatabas.
- Ingen larmhantering med kvittering. Gränsvärden på mätvärdessymboler är enbart visuella.
