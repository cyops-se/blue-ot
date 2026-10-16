**Svenska** · [English](README-en.md)

# Blue OT

Blue OT är en labbmiljö för OT, alltså driftteknik och industriella styrsystem. Den består av ett SCADA-system och tre simulatorer som tillsammans bildar en liten kraftsystemanläggning. Allt är skrivet i Go, och SCADA-systemet har ett webbaserat HMI i Vue och Vuetify. Simulatorerna talar riktiga industriprotokoll, Modbus TCP och IEC 60870-5-104, så de kan också användas med andra SCADA-system, protokollanalysatorer eller säkerhetsverktyg.

## Komponenter

| Komponent | Vad det är | Gränssnitt | Dokumentation |
|---|---|---|---|
| **scada** | SCADA-system med Go-backend och webb-HMI. Det pollar enheter över Modbus TCP eller IEC 104, visar driftbilder och mätvärden, loggar händelser och låter operatören manövrera brytare, omkopplare och börvärden. Driftbilderna ritas i en inbyggd vy-editor. Backend och HMI levereras som en enda exe-fil. | HTTP/REST och WebSocket (HMI), Modbus TCP- och IEC 104-klient | [scada/README.md](scada/README.md) |
| **modsim** | En liten Modbus TCP-simulator som ingår i SCADA-projektet. Den simulerar ett 10 kV-ställverk med fyra fack, brytare, frånskiljare med förregling och mätvärden. Den är tänkt för att snabbt testa SCADA utan något annat. | Modbus TCP-server | [scada/README.md](scada/README.md#modsim) |
| **blue-station** | Simulator för en transformatorstation på 220/20 kV. Den har stamnätsmatning, två transformatorer med lindningskopplare och spänningsreglering, en sektionerad 20 kV-skena, fyra avgångar med dygnsprofiler och överströmsskydd, och fack G1 där kraftvärmeverket är anslutet. | Modbus TCP, IEC 60870-5-104, webbgränssnitt med JSON-API | [blue-station/README.md](blue-station/README.md) |
| **blue-chp** | Simulator för ett kraftvärmeverk med två pannor, ångsamlare och bypass, en mottrycksturbin med generator och ett fjärrvärmenät med pumpar och värmeackumulator. Generatorn matar in el i blue-station. | Modbus TCP, IEC 60870-5-104, webbgränssnitt med JSON-API, länk till blue-station | [blue-chp/README.md](blue-chp/README.md) |
| **blue-water** | Simulator för ett dricksvattenverk med råvattenpumpar, kemisk fällning, sedimentering, sandfilter med backspolning, klordesinfektion, rentvattenreservoar och högtryckspumpar mot ett distributionsnät. | Modbus TCP, IEC 60870-5-104, webbgränssnitt med JSON-API | [blue-water/README.md](blue-water/README.md) |
| **blue-wwtp** | Simulator för ett avloppsreningsverk med nivåstyrd inloppspumpstation och bräddning, galler, aktivslam med syrereglerade blåsmaskiner, kemisk fällning och rötkammare med biogas. | Modbus TCP, IEC 60870-5-104, webbgränssnitt med JSON-API | [blue-wwtp/README.md](blue-wwtp/README.md) |
| **blue-proxy** | Transparent Modbus TCP / IEC 104-proxy mellan SCADA och en enhet. Den minns insamlade tillstånd och senaste manövrar och kan per tagg eller typ frysa det rapporterade värdet och blockera manövrar. Ett webbgränssnitt visar insamlat kontra rapporterat och styr lägena. | Modbus TCP, IEC 60870-5-104 (proxy), webbgränssnitt | [blue-proxy/README.md](blue-proxy/README.md) |

## Hur delarna hänger ihop

```
                         ┌────────────────────────────────────────┐
   webbläsare ──HTTP/WS──►  scada  (HMI + REST + WebSocket, :8080) │
                         └───┬───────────────┬────────────────┬───┘
                             │ Modbus TCP    │ Modbus TCP     │ Modbus TCP
                             ▼               ▼                ▼
                      ┌──────────┐   ┌───────────────┐   ┌───────────────┐
                      │  modsim  │   │ blue-station  │   │   blue-chp    │
                      │  :5020   │   │ :502  IEC:2404│   │ :1502 IEC:2405│
                      └──────────┘   │ webb (valfri) │   │ webb :8082    │
                                     └───────┬───────┘   └───────▲───────┘
                                             │  fack G1, 20 kV   │
                                             └──── /api/link ────┘
                                     skenans spänning, frekvens och brytarläge →
                                     ← generatorns P och Q
```

- **SCADA ↔ simulatorerna:** SCADA är Modbus-klient och pollar varje simulator som en egen enhet. Tagg- och vykonfigurationen för blue-station, blue-chp, blue-water och blue-wwtp följer med i `scada/backend/data` och kan genereras om från simulatorernas signallistor.
- **blue-station ↔ blue-chp:** kopplingen motsvarar 20 kV-kabeln mellan verket och stationen. Det är ingen SCADA-kommunikation. Varje sekund skickar stationen läget på anslutningspunkten till kraftvärmeverket och får tillbaka generatorns produktion, som matas in på sektion B. Om facket öppnas eller skenan blir spänningslös löser generatorns nätbortfallsskydd ut. Båda simulatorerna fungerar också var för sig.

## Snabbstart

Krav: Go 1.21 eller senare. För att bygga om HMI behövs dessutom Node 18 eller senare.

Starta varje del i ett eget terminalfönster:

```bash
# Simulatorerna
cd blue-chp     && go run ./cmd/blue-chp                     # Modbus :1502, IEC 104 :2405, webb :8082
cd blue-station && go run ./cmd/blue-station -http :8090     # Modbus :502,  IEC 104 :2404, webb :8090
cd blue-water   && go run ./cmd/blue-water                   # Modbus :1503, IEC 104 :2406, webb :8085
cd blue-wwtp    && go run ./cmd/blue-wwtp                    # Modbus :1504, IEC 104 :2407, webb :8086
cd scada/backend && go run ./cmd/modsim                      # Modbus :5020

# SCADA – bygg HMI en gång, starta sedan backend
cd scada/frontend && npm install && npm run build
cd scada/backend  && go run ./cmd/scada -data data           # http://localhost:8080
```

Öppna http://localhost:8080 och välj någon av vyerna **Station Norra** (modsim), **Blue Station**, **Blue CHP**, **Blue Water** eller **Blue WWTP**. Simulatorernas egna webbgränssnitt, med alla signaler och en händelselogg, finns på http://localhost:8090 (blue-station), http://localhost:8082 (blue-chp), http://localhost:8085 (blue-water) och http://localhost:8086 (blue-wwtp).

blue-station använder också port 8080 för sitt webbgränssnitt som standard. Om SCADA körs på samma dator måste stationens webb därför flyttas med `-http`, som i exemplet ovan. På Linux krävs root eller `CAP_NET_BIND_SERVICE` för Modbus-porten 502. Använd annars t.ex. `-modbus :5021` och ändra porten för enheten `blue-station` i SCADA.

### Portar

| Komponent | Modbus TCP | IEC 60870-5-104 | HTTP |
|---|---|---|---|
| scada | (klient) | – | 8080 |
| modsim | 5020 | – | – |
| blue-station | 502 | 2404 (CA 1) | 8080 som standard, 8090 i exemplet ovan |
| blue-chp | 1502 | 2405 (CA 2) | 8082 (webb och länk till blue-station) |
| blue-water | 1503 | 2406 (CA 3) | 8085 |
| blue-wwtp | 1504 | 2407 (CA 4) | 8086 |

### Verktyg och proxy

- **opctl** (`scada/backend/cmd/opctl`): manövrerar brytare, frånskiljare och pumpar från kommandoraden över Modbus TCP eller IEC 104, med adressering från SCADA:s konfiguration. Se [scada/README.md](scada/README.md#opctl).
- **blue-proxy**: läggs mellan SCADA och en enhet för att frysa rapporterade värden och blockera manövrar. Se [blue-proxy/README.md](blue-proxy/README.md).

## Katalogstruktur

```
blue-ot/
├── scada/
│   ├── backend/        Go: SCADA-server (cmd/scada), modsim (cmd/modsim), opctl (cmd/opctl), data/ med konfiguration och vyer, tools/ med generatorskript
│   └── frontend/       Vue 3 + Vuetify 3: HMI, byggs in i backendens exe-fil
├── blue-station/       Go: simulator för transformatorstation 220/20 kV
├── blue-chp/           Go: simulator för kraftvärmeverk med fjärrvärme
├── blue-water/         Go: simulator för dricksvattenverk
├── blue-wwtp/          Go: simulator för avloppsreningsverk
└── blue-proxy/         Go: transparent Modbus TCP / IEC 104-proxy med frysning och manöverblockering
```

Varje Go-katalog (`scada/backend`, `blue-station`, `blue-chp`, `blue-water`, `blue-wwtp` och `blue-proxy`) är en egen modul. Testerna körs med `go test ./...` i respektive katalog. Simulatorerna och proxyn använder bara Go:s standardbibliotek.

## Säkerhet

Allt i Blue OT är tänkt för labb, utbildning och test. Varken SCADA eller simulatorerna har autentisering eller kryptering, och alla manövrar tas emot från vem som helst som når portarna. Kör dem bara i isolerade nät och anslut aldrig SCADA till riktig utrustning utan att först lägga till behörighetsstyrning.

## Licens

MIT, se [LICENSE](LICENSE).
