**Svenska** · [English](README-en.md)

# Blue Proxy – Modbus TCP / IEC 104-proxy med fryst rapportering och manöverblockering

Blue Proxy ligger mellan en SCADA-master och en nedströms enhet (till exempel [blue-station](../blue-station/README.md) eller [blue-chp](../blue-chp/README.md)). Den vidarebefordrar trafiken transparent, men kommer ihåg de insamlade tillstånden och de senaste manövrarna, och kan per tagg eller för alla taggar av en viss typ **frysa** det senast rapporterade värdet så att det fortsätter att rapporteras oavsett vad källan skickar, samt **blockera manövrar** så att de inte når enheten. Ett webbgränssnitt visar både det insamlade och det rapporterade värdet och låter dig ställa om lägena.

Det här är ett verktyg för labb: att studera hur ett SCADA-system beter sig när en mellanliggande gateway fryser eller manipulerar telemetrin (till exempel vid test av larm, redundans och driftrutiner).

```
   SCADA-master ──Modbus TCP──►              ──Modbus TCP──►  enhet
                              Blue Proxy                       (blue-station,
   SCADA-master ──IEC 104────►   :1510/:2414  ──IEC 104─────►   blue-chp …)
                                    │
                               webbgränssnitt :8083
                               insamlat vs rapporterat, frys/släpp
```

## Så fungerar den

- **Normalläge (transparent):** varje begäran och svar skickas vidare oförändrat. Proxyn avkodar läsningar för att komma ihåg det insamlade värdet, och skrivningar (manövrar) för att komma ihåg den senaste manövern.
- **Fryst läge (hold):** när en punkt fryses låses det rapporterade värdet vid det senast rapporterade. Proxyn fortsätter att samla in det verkliga värdet från källan, men skriver in det frysta värdet i svaret till mastern. För Modbus skrivs registren eller bitarna om, för IEC 104 skrivs informationsobjektet om (längd och sekvensnummer bevaras, så proxyn är transparent för protokollet).
- **Blockerad manöver (block):** när en punkt blockeras avvisas manövrar från mastern i stället för att skickas vidare till enheten. För Modbus svarar proxyn med undantaget *server device failure* (0x04) utan att vidarebefordra skrivningen. För IEC 104 ogiltigförklaras adressen i kommandot så att enheten själv svarar negativt (längd och sekvensnummer bevaras); kommandot når aldrig processen. Manövern loggas ändå som senaste manöver, markerad som blockerad.
- **Per tagg eller per typ:** frysning och blockering kan slås på eller av var för sig för en enskild tagg, för alla taggar av en typ (brytare, frånskiljare, pump, mätvärde, binär, börvärde) eller för alla punkter.

Taggarna, deras adresser och IEC 104-IOA:er hämtas från SCADA:s `config.json` (samma fil som SCADA och opctl använder). Proxyn kör mot **en** enhet i taget.

## Köra

```bash
cd blue-proxy
go run ./cmd/blue-proxy -device blue-station
go test ./...
```

Som standard läses `../scada/backend/data/config.json`, proxyn lägger sig framför enheten `blue-station` och lyssnar på Modbus `:1510`, IEC 104 `:2414` och webb `:8083`. Uppströms tas från enhetens `host`/`port` respektive `iec104.port` i konfigurationen.

| Flagga | Standard | Beskrivning |
|---|---|---|
| `-config` | `../scada/backend/data/config.json` | SCADA:s config.json (eller katalogen) |
| `-device` | `blue-station` | Enhet i konfigurationen att proxa |
| `-modbus` | `:1510` | Lyssnaradress för Modbus TCP-proxyn (tom = av) |
| `-iec104` | `:2414` | Lyssnaradress för IEC 104-proxyn (tom = av) |
| `-http` | `0.0.0.0:8083` | Webbgränssnitt; lyssnar på alla gränssnitt som standard, ange t.ex. `127.0.0.1:8083` för enbart lokalt (tom = av) |
| `-upstream-modbus` | – | Överskrid uppströms Modbus `host:port` |
| `-upstream-iec104` | – | Överskrid uppströms IEC 104 `host:port` |
| `-timeout` | `5s` | Uppströms tidsgräns |

Exempel: proxa blue-chp i stället, och peka om uppströms:

```bash
go run ./cmd/blue-proxy -device blue-chp -modbus :1511 -iec104 :2415 -http :8084
```

### Prova med SCADA

1. Starta enheten, t.ex. `cd blue-station && go run ./cmd/blue-station -http :8090`.
2. Starta proxyn: `cd blue-proxy && go run ./cmd/blue-proxy -device blue-station`.
3. Peka SCADA (eller opctl) på proxyns portar i stället för enhetens: sätt enhetens `host`/`port` till proxyn (`127.0.0.1:1510`) i en kopia av datakatalogen, eller använd `opctl -data <kopia>`.
4. Öppna `http://localhost:8083`, frys ett mätvärde och se hur det insamlade värdet fortsätter att ändras medan det rapporterade (och det SCADA ser) står still.

## Webbgränssnitt och API

`http://localhost:8083` visar en tabell med punktens namn och id, typ, protokoll, insamlat värde (från källan), rapporterat värde (ut till mastern), senaste manöver och två knappar per rad: **Rapportering** (frys/transparent) och **Manöver** (blockera/tillåt). Längst upp finns en väljare för att frysa, släppa, blockera eller tillåta en hel grupp, ett filter och val att bara visa frysta eller bara blockerade punkter. Rapporterade värden som skiljer sig från det insamlade markeras, liksom blockerade manövrar.

- `GET /api/state` – alla punkter med insamlat och rapporterat värde, fryst- och blockerat-läge, senaste manöver och kategorier.
- `POST /api/hold` – `{"key": "Grid.U", "on": true}` för en tagg, eller `{"category": "mätvärde", "on": true}` för en typ (`all` för alla).
- `POST /api/block` – samma form som `/api/hold`, men blockerar eller tillåter manövrar.

Samma API kan drivas från kommandoraden med `opctl -proxy <url> …` (se [scada/README.md](../scada/README.md#styra-en-blue-proxy)).

## Vad som stöds

- **Modbus TCP:** funktionskoderna 1, 2, 3, 4 (läsning, insamling och frysning) samt 5, 6, 15, 16 (skrivning, ihågkommen manöver). float32/övriga datatyper avkodas enligt taggens inställningar (skala, offset, ord- och byteordning).
- **IEC 60870-5-104:** övervakning `M_SP`, `M_DP`, `M_ST`, `M_ME_NC` (med och utan tidsstämpel), både enskilda objekt och sekvenser (SQ). Kommandon `C_SC`, `C_DC`, `C_RC`, `C_SE_NC` kommer ihåg som senaste manöver och kan blockeras. Frysning skriver om informationsobjektets värde utan att ändra kvalitetsbitar, tidsstämplar, längd eller sekvensnummer.

## Struktur

```
cmd/blue-proxy/      main, flaggor
internal/config/     laddar SCADA:s config.json, klassificerar taggar, avkodning (codec)
internal/state/      minnet: insamlat, rapporterat, frysning, senaste manöver
internal/mbproxy/    transparent Modbus TCP-proxy
internal/iecproxy/   transparent IEC 60870-5-104-proxy
internal/web/        webbgränssnitt och JSON-API
```

## Begränsningar

- Proxyn kör mot en enhet per instans. Kör flera instanser för flera enheter.
- IEC 104-frysning och -blockering kräver att taggarna har monitor-IOA (`ioa`) respektive styr-IOA (`ctrlIoa`) i konfigurationen; de fylls i av generatorskripten. Enheter utan `iec104` i konfigurationen proxas bara över Modbus.
- Frysning påverkar bara rapporteringen nedströms; blockering påverkar bara manövrar uppströms. De är oberoende och kan kombineras (till exempel frysa en brytares läge och samtidigt blockera manövrar mot den).
- Ingen autentisering eller kryptering, precis som resten av Blue OT. Endast för isolerade labbnät.
