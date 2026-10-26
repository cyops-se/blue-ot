# BlueLock – kioskläge för bildspel

En Windows-app (WinForms, .NET 9) som visar bilder i helskärm på alla skärmar och byter bild med jämna mellanrum. Användaren kan varken stänga appen eller komma åt resten av Windows. Appen stängs bara med en hemlig sekvens på 5 tangenttryckningar.

## Bygga och köra

```powershell
dotnet publish -c Release -r win-x64 --self-contained -o publish
.\publish\BlueLock.exe
```

Lägg bilderna (`.jpg`, `.png`, `.bmp`, `.gif`, `.tif`) i mappen `Images` bredvid exe-filen. Mappen läses om efter varje varv, så du kan lägga till och ta bort bilder medan appen kör.

## Avsluta

Tryck tangenterna i `ExitSequence` i tur och ordning. Som standard är det:

**F12 → F9 → F11 → F10 → Esc**

Det får gå högst `SequenceTimeoutSeconds` sekunder mellan två tryck. **Byt sekvens innan du driftsätter.**

## Konfiguration (`appsettings.json`)

| Inställning | Beskrivning |
|---|---|
| `ImageFolder` | Bildmappen, absolut eller relativ till exe-filen |
| `IntervalSeconds` | Sekunder mellan bildbyten (minst 1) |
| `Shuffle` | Slumpa ordningen för varje varv |
| `BackgroundColor` | Färgnamn eller `#RRGGBB` |
| `ExitSequence` | Exakt 5 tangentnamn enligt [`System.Windows.Forms.Keys`](https://learn.microsoft.com/dotnet/api/system.windows.forms.keys), t.ex. `F12`, `A`, `D7`, `NumPad3`, `Escape`, `ShiftKey` |
| `SequenceTimeoutSeconds` | Maxpaus mellan två tryck i sekvensen |
| `ApplyPolicies` | Lås Ctrl+Alt+Del-menyn medan appen kör (se nedan) |

Om sekvensen är ogiltig startar appen inte, så att du aldrig kan låsa ute dig själv.

## Så låser appen

- **Fönster:** ett kantlöst fönster per skärm som alltid ligger överst. Om något annat fönster får fokus tar appen tillbaka det inom 250 ms. Muspekaren döljs. Alt+F4 och WM_CLOSE ignoreras.
- **Tangentbord:** en global lågnivåhook (`WH_KEYBOARD_LL`) sväljer *alla* tangenter: Win, Alt+Tab, Ctrl+Esc, Ctrl+Shift+Esc och så vidare.
- **Ctrl+Alt+Del** kan ingen vanlig app fånga. Därför sätter appen tillfälligt registerpolicys (`DisableTaskMgr`, `DisableLockWorkstation`, `DisableChangePassword`, `NoLogoff`, `NoClose` och, om appen körs som admin, `HideFastUserSwitching`). Då finns bara "Avbryt" kvar i menyn. De ursprungliga värdena sparas i `%LOCALAPPDATA%\BlueLock\policy-backup.json` och återställs när appen avslutas, och vid nästa start om den kraschat.
- **Ström:** skärmsläckare och viloläge stängs av medan appen kör.
- Bara en instans kan köra åt gången.

Kommandoradsflaggor:

```powershell
BlueLock.exe --install-autostart   # startar med användaren (HKCU\...\Run)
BlueLock.exe --remove-autostart
BlueLock.exe --restore-policies    # återställ policys manuellt om något gått fel
```

Fel loggas till `%LOCALAPPDATA%\BlueLock\kiosk.log`.

## Styra kiosken via SSH

Program som startas från Windows OpenSSH körs i SSH-inloggningens egen, osynliga session och kan inte visa fönster. BlueLock startar därför kiosken via en schemalagd aktivitet, `BlueLock`, som körs i aktivitetens användares session. Det gäller både en session vid den fysiska skärmen och en session via fjärrskrivbord (RDP).

**Engångsinställning** (på kioskdatorn, eller via SSH som kioskanvändaren):

```powershell
BlueLock.exe --register-task                             # för aktuell användare
BlueLock.exe --register-task KIOSK\visning               # för en annan användare (kräver admin)
BlueLock.exe --register-task KIOSK\visning --autostart   # starta även automatiskt, se nedan
```

**Från en annan dator:**

```powershell
ssh kiosk@kioskdator "C:\BlueLock\BlueLock.exe"           # startar kiosken på skärmen
ssh kiosk@kioskdator "C:\BlueLock\BlueLock.exe --status"  # körs den? vem är inloggad?
ssh kiosk@kioskdator "C:\BlueLock\BlueLock.exe --stop"    # stänger kiosken
```

| Kommando | Beskrivning | Slutkod |
|---|---|---|
| `--start` (eller ingen flagga över SSH) | Kör aktiviteten och väntar tills kiosken syns | 0 = startad eller körde redan, 2 = användaren är inte inloggad, 3 = startade inte, 5 = aktiviteten saknas |
| `--stop` | Ber kiosken stänga sig, i alla sessioner. Policys återställs precis som med tangentsekvensen | 0 = stoppad, 4 = inget svar |
| `--status` | Visar om kiosken körs, alla inloggade sessioner (skärmen eller fjärrskrivbord) och aktivitetens användare | 0 = körs, 1 = körs inte |
| `--unregister-task` | Tar bort aktiviteten | |

Bra att veta:

- Aktivitetens användare måste vara **inloggad**, antingen vid skärmen (till exempel med automatisk inloggning) eller via fjärrskrivbord. Om sessionen är frånkopplad startar kiosken ändå och syns när någon ansluter igen.
- SSH-sessioner känns igen på miljövariabeln `SSH_CONNECTION`. Startar du `BlueLock.exe` utan flaggor över SSH används alltså automatiskt `--start`.
- Stoppsignalen är filen `%ProgramData%\BlueLock\stop.signal`. Kiosken tittar efter den fyra gånger per sekund. Mappen får skrivrättighet för alla autentiserade användare, så att SSH-kontot och kioskkontot kan vara olika användare.
- Med `--autostart` startar kiosken automatiskt när användaren loggar in och när någon ansluter till sessionen, vid skärmen eller via fjärrskrivbord. Om kiosken redan körs händer ingenting. För att underhålla datorn via fjärrskrivbord stänger du först kiosken med tangentsekvensen eller `--stop`. Utlösarna kan kräva att `--register-task` körs som administratör.
- Den som har SSH-åtkomst kan stoppa kiosken. Begränsa därför vilka konton som får logga in via SSH (`AllowUsers` i `C:\ProgramData\ssh\sshd_config`).

## Fjärrskrivbord (RDP)

Kiosken fungerar på samma sätt i en fjärrskrivbordssession som vid skärmen:

- Tangentbordet låses och Ctrl+Alt+End, som är fjärrskrivbordets Ctrl+Alt+Del, visar samma nedskurna meny.
- Fönstren byggs om när klienten ändrar upplösning eller när sessionen flyttas mellan skärmen och en fjärrskrivbordsklient.
- Användaren kan alltid stänga fjärrskrivbordsklienten på sin egen dator. Då kopplas sessionen bara från, och kiosken fortsätter köra och finns kvar vid nästa anslutning.
- Windows 11 Pro tillåter en session åt gången. Ansluter kioskanvändaren via fjärrskrivbord flyttas sessionen, och kiosken med den, från skärmen till klienten. Loggar en annan användare in kopplas kioskanvändaren från.
- Ska tangentsekvensen fungera via fjärrskrivbord måste klienten skicka F-tangenterna och Esc vidare. Det gör `mstsc` som standard.

## Begränsningar och rekommendationer för skarp drift

En vanlig app i en vanlig användarsession kan inte bli helt låst. För en riktig kiosk:

1. Använd ett **eget standardkonto** utan adminrättigheter.
2. Kör appen som skal med **Shell Launcher** eller **Assigned Access** (Windows Enterprise/Education/IoT). Då startar Explorer aldrig.
3. Pekskärmar: stäng av kantsvep med `HKLM\SOFTWARE\Policies\Microsoft\Windows\EdgeUI` → `AllowEdgeSwipe = 0` (DWORD).
4. Blockera **Sticky Keys** och andra hjälpmedelsgenvägar under Inställningar → Hjälpmedel.
5. Om processen dödas eller datorn startas om hamnar man i Windows. Kombinera därför med autostart, automatisk inloggning och gärna Shell Launchers inställning "starta om vid avslut".
