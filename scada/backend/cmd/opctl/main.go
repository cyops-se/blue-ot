// Command opctl operates a breaker or disconnector by tag name. The tag and
// the desired state are given as arguments; everything else (host, port,
// Modbus address or IEC 104 control IOA) comes from the SCADA configuration
// file. Both Modbus TCP and IEC 60870-5-104 are supported.
//
//	opctl [flags] <tag> <open|close>
//
// Examples:
//
//	opctl G1.CB close                       # Modbus TCP, from data/config.json
//	opctl -protocol iec104 Q0.CB open       # IEC 104
//	opctl -data ../other/data QS1_POS open
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/simonvetter/modbus"

	"blueot/scada/internal/iec104client"
	"blueot/scada/internal/model"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fel:", err)
		os.Exit(1)
	}
}

func run() error {
	data := flag.String("data", "data", "sökväg till datakatalogen eller direkt till config.json")
	protocol := flag.String("protocol", "modbus", "protokoll: modbus eller iec104")
	timeout := flag.Duration("timeout", 5*time.Second, "tidsgräns per anrop")
	openVal := flag.Int("open-value", -1, "Modbus-värde för öppna/frånslag (standard: coil 0, holding 1)")
	closeVal := flag.Int("close-value", -1, "Modbus-värde för slut/tillslag (standard: coil 1, holding 2)")
	host := flag.String("host", "", "överskrid IP-adressen/värdnamnet från konfigurationen")
	port := flag.Int("port", 0, "överskrid porten från konfigurationen (Modbus eller IEC 104 beroende på protokoll)")
	unit := flag.Int("unit", -1, "överskrid unit id från konfigurationen")
	verify := flag.Bool("verify", true, "läs tillbaka lägestaggen efter manövern (Modbus)")
	dryRun := flag.Bool("dry-run", false, "visa vad som skulle göras utan att ansluta")
	list := flag.Bool("list", false, "lista kända brytare, frånskiljare och pumpar (valfritt filter som argument)")
	proxy := flag.String("proxy", "", "adress till en blue-proxy, t.ex. http://127.0.0.1:8083, för frys/block-åtgärder")
	category := flag.String("category", "", "tillämpa åtgärden på en hel typ (brytare, frånskiljare, pump …); gäller både manöver och -proxy")
	flag.Usage = usage
	flag.Parse()

	if *proxy != "" {
		return runProxy(*proxy, *category, flag.Args(), *timeout)
	}

	if *list {
		cfg, err := loadConfig(*data)
		if err != nil {
			return err
		}
		return listTags(cfg, flag.Arg(0))
	}

	// Bulk manoeuvre of a whole type, mirroring the proxy's -category / type:
	// targets: operate every operable point of the type over one protocol.
	if want, state, bulk, err := operateTarget(*category, flag.Args()); err != nil {
		return err
	} else if bulk {
		closeOp, err := parseState(state)
		if err != nil {
			return err
		}
		cfg, err := loadConfig(*data)
		if err != nil {
			return err
		}
		return doBulk(cfg, want, closeOp, *protocol, *host, *port, *unit, *openVal, *closeVal, *timeout, *verify, *dryRun)
	}

	if flag.NArg() != 2 {
		usage()
		return errors.New("ange en tagg och ett tillstånd")
	}
	tagID := flag.Arg(0)
	closeOp, err := parseState(flag.Arg(1))
	if err != nil {
		return err
	}

	cfg, err := loadConfig(*data)
	if err != nil {
		return err
	}
	r, err := resolve(cfg, tagID)
	if err != nil {
		return err
	}
	dev := applyOverrides(r.device, *host, *port)
	action := "öppna/frånslag"
	if closeOp {
		action = "slut/tillslag"
	}

	switch strings.ToLower(*protocol) {
	case "modbus":
		return doModbus(r, dev, closeOp, action, *openVal, *closeVal, *unit, *timeout, *verify, *dryRun)
	case "iec104", "iec", "104":
		return doIEC(r, dev, closeOp, action, *timeout, *dryRun)
	default:
		return fmt.Errorf("okänt protokoll %q (välj modbus eller iec104)", *protocol)
	}
}

// ---- Modbus TCP ----

func doModbus(r resolved, dev model.Device, closeOp bool, action string, openVal, closeVal, unit int, timeout time.Duration, verify, dry bool) error {
	cmd := r.cmd
	if cmd == nil {
		return fmt.Errorf("hittar ingen skrivbar manövertagg för %q", r.status.ID)
	}
	ov, cv := defaultValues(cmd.Area)
	if openVal >= 0 {
		ov = openVal
	}
	if closeVal >= 0 {
		cv = closeVal
	}
	value := ov
	if closeOp {
		value = cv
	}
	unitID := dev.UnitID
	if unit >= 0 {
		unitID = unit
	}
	fmt.Printf("Modbus TCP %s:%d unit %d — %s: skriver %s %d till %s (%s)\n",
		dev.Host, dev.Port, unitID, action, cmd.Area, value, cmd.ID, addrText(cmd))
	if dry {
		return nil
	}

	mc, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:     fmt.Sprintf("tcp://%s:%d", dev.Host, dev.Port),
		Timeout: timeout,
	})
	if err != nil {
		return err
	}
	if err := mc.SetUnitId(uint8(unitID)); err != nil {
		return err
	}
	if err := mc.Open(); err != nil {
		return &connError{fmt.Errorf("ansluter till %s:%d: %w", dev.Host, dev.Port, err)}
	}
	defer mc.Close()

	switch cmd.Area {
	case model.AreaCoil:
		err = mc.WriteCoil(uint16(cmd.Address), value != 0)
	case model.AreaHolding:
		if value < 0 || value > 0xFFFF {
			return fmt.Errorf("värdet %d ryms inte i ett register", value)
		}
		err = mc.WriteRegister(uint16(cmd.Address), uint16(value))
	default:
		return fmt.Errorf("manövertaggen %q ligger i det skrivskyddade området %q", cmd.ID, cmd.Area)
	}
	if err != nil {
		return fmt.Errorf("skrivning misslyckades: %w", err)
	}
	fmt.Println("Manöver skickad.")

	if verify && r.status != nil {
		if err := verifyModbus(mc, r.status, float64(value)); err != nil {
			fmt.Fprintln(os.Stderr, "varning: kunde inte läsa tillbaka läget:", err)
		}
	}
	return nil
}

// verifyModbus reads the status tag until it reaches the expected end position
// (breakers and disconnectors are given time to travel there) or a short
// timeout elapses, then prints the result. In all bundled simulators the
// end-position code equals the command value written, so `expect` is that value.
func verifyModbus(mc *modbus.ModbusClient, status *model.Tag, expect float64) error {
	deadline := time.Now().Add(5 * time.Second)
	var raw float64
	reached := false
	for {
		time.Sleep(300 * time.Millisecond)
		v, err := readStatus(mc, status)
		if err != nil {
			return err
		}
		raw = v
		if v == expect {
			reached = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
	}
	suffix := ""
	if !reached {
		suffix = " (nådde inte förväntat läge inom 5 s)"
	}
	fmt.Printf("Läge (%s): %s%s\n", status.ID, stateTextFor(status, raw), suffix)
	return nil
}

func readStatus(mc *modbus.ModbusClient, status *model.Tag) (float64, error) {
	switch status.Area {
	case model.AreaDiscrete:
		v, err := mc.ReadDiscreteInput(uint16(status.Address))
		if err != nil || !v {
			return 0, err
		}
		return 1, nil
	case model.AreaCoil:
		v, err := mc.ReadCoil(uint16(status.Address))
		if err != nil || !v {
			return 0, err
		}
		return 1, nil
	default:
		rt := modbus.HOLDING_REGISTER
		if status.Area == model.AreaInput {
			rt = modbus.INPUT_REGISTER
		}
		v, err := mc.ReadRegister(uint16(status.Address), rt)
		return float64(v), err
	}
}

// ---- IEC 60870-5-104 ----

func doIEC(r resolved, dev model.Device, closeOp bool, action string, timeout time.Duration, dry bool) error {
	if dev.IEC == nil {
		return fmt.Errorf("enheten %q saknar iec104-uppgifter i konfigurationen", dev.ID)
	}
	st := r.status
	if st == nil || st.CtrlIOA == 0 {
		return fmt.Errorf("taggen %q saknar ctrlIoa i konfigurationen (IEC 104 går inte)", r.statusName())
	}
	cmdType := st.Cmd
	if cmdType == "" {
		cmdType = "C_DC"
	}
	addr := fmt.Sprintf("%s:%d", dev.Host, dev.IEC.Port)
	fmt.Printf("IEC 104 %s CA %d — %s: %s IOA %d\n", addr, dev.IEC.CA, action, cmdType, st.CtrlIOA)
	if dry {
		return nil
	}

	cl, err := iec104client.Dial(addr, timeout)
	if err != nil {
		return &connError{err}
	}
	defer cl.Close()

	switch cmdType {
	case "C_DC":
		err = cl.Breaker(uint16(dev.IEC.CA), st.CtrlIOA, closeOp)
	case "C_SC":
		err = cl.Switch(uint16(dev.IEC.CA), st.CtrlIOA, closeOp)
	default:
		return fmt.Errorf("okänd kommandotyp %q (stöds: C_DC, C_SC)", cmdType)
	}
	if err != nil {
		return err
	}
	fmt.Println("Manöver bekräftad (ACTTERM).")
	return nil
}

// ---- configuration and tag resolution ----

func loadConfig(path string) (*model.Config, error) {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		path = filepath.Join(path, "config.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("läser konfiguration: %w", err)
	}
	var cfg model.Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("tolkar %s: %w", path, err)
	}
	return &cfg, nil
}

func addrText(t *model.Tag) string { return fmt.Sprintf("%s %d", t.Area, t.Address) }

func stateTextFor(t *model.Tag, raw float64) string {
	key := fmt.Sprintf("%g", raw)
	if t.StateText != nil {
		if s, ok := t.StateText[key]; ok {
			return fmt.Sprintf("%s (%s)", s, key)
		}
	}
	return key
}

type resolved struct {
	status *model.Tag
	cmd    *model.Tag
	device model.Device
}

func (r resolved) statusName() string {
	if r.status != nil {
		return r.status.ID
	}
	return "(okänd)"
}

// resolve finds the status tag and the writable command tag for the given id,
// which may be either of them, and the device they belong to.
func resolve(cfg *model.Config, id string) (resolved, error) {
	byID := indexByID(cfg)
	t := byID[id]
	if t == nil {
		return resolved{}, fmt.Errorf("okänd tagg %q i konfigurationen", id)
	}

	var status, cmd *model.Tag
	if t.Writable {
		cmd = t
		status = firstOf(byID, statusCandidates(id))
	} else {
		status = t
		cmd = firstOf(byID, cmdCandidates(id))
	}

	devID := t.Device
	if status != nil {
		devID = status.Device
	}
	dev, ok := deviceByID(cfg, devID)
	if !ok {
		return resolved{}, fmt.Errorf("okänd enhet %q för tagg %q", devID, id)
	}
	return resolved{status: status, cmd: cmd, device: dev}, nil
}

// cmdCandidates lists possible command-tag ids for a status tag id.
func cmdCandidates(id string) []string {
	out := []string{id + ".Cmd", id + "_CMD"}
	if s, ok := strings.CutSuffix(id, "_POS"); ok {
		out = append(out, s+"_CMD")
	}
	if s, ok := strings.CutSuffix(id, ".Pos"); ok {
		out = append(out, s+".Cmd")
	}
	return out
}

// statusCandidates lists possible status-tag ids for a command tag id.
func statusCandidates(id string) []string {
	var out []string
	if s, ok := strings.CutSuffix(id, ".Cmd"); ok {
		out = append(out, s, s+"_POS", s+".Pos")
	}
	if s, ok := strings.CutSuffix(id, "_CMD"); ok {
		out = append(out, s+"_POS", s)
	}
	return out
}

func firstOf(byID map[string]*model.Tag, ids []string) *model.Tag {
	for _, id := range ids {
		if t := byID[id]; t != nil {
			return t
		}
	}
	return nil
}

func deviceByID(cfg *model.Config, id string) (model.Device, bool) {
	for _, d := range cfg.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return model.Device{}, false
}

// ---- listing ----

var (
	reBreaker = regexp.MustCompile(`(^|[._])Q[0-9]`)  // Q0, Q1 … (not QS)
	reDisc    = regexp.MustCompile(`(^|[._])QS[0-9]`) // QS1 …
)

// category classifies a tag as a breaker, disconnector or pump from its id and
// name. The SCADA config has no explicit type, so this is heuristic. It
// returns "" for anything else. Order matters: pump, then disconnector, then
// breaker (so QS is not taken for a Q breaker).
func category(t *model.Tag) string {
	low := strings.ToLower(t.ID + " " + t.Name)
	switch {
	case strings.Contains(low, "pump"):
		return "pump"
	case reDisc.MatchString(t.ID) || strings.Contains(low, "frånskilj") || strings.Contains(low, "disconnect"):
		return "frånskiljare"
	case strings.Contains(t.ID, ".CB") || strings.Contains(t.ID, "_CB") ||
		reBreaker.MatchString(t.ID) || strings.Contains(low, "brytare") || strings.Contains(low, "breaker"):
		return "brytare"
	}
	return ""
}

var catOrder = map[string]int{"brytare": 0, "frånskiljare": 1, "pump": 2}

// matchFilter maps a user filter to a canonical category, or "" for "all".
func matchFilter(f string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "":
		return "", true
	case "brytare", "breaker", "cb":
		return "brytare", true
	case "frånskiljare", "franskiljare", "disconnector", "qs":
		return "frånskiljare", true
	case "pump", "pumpar", "pumps":
		return "pump", true
	}
	return "", false
}

// listTags prints the known breakers, disconnectors and pumps and how each one
// can be operated. The command tag and IEC 104 availability come from the
// configuration, exactly as the operate path resolves them.
func listTags(cfg *model.Config, filter string) error {
	want, ok := matchFilter(filter)
	if !ok {
		return fmt.Errorf("okänt filter %q (använd brytare, frånskiljare eller pump)", filter)
	}

	byID := indexByID(cfg)

	type row struct{ typ, tag, dev, proto, cmd string }
	var rows []row
	for i := range cfg.Tags {
		t := &cfg.Tags[i]
		if t.Writable { // the command tag, not the position tag
			continue
		}
		cat := category(t)
		if cat == "" || (want != "" && cat != want) {
			continue
		}
		protos, cmdName := operability(cfg, byID, t)
		if len(protos) == 0 { // not operable: skip
			continue
		}
		rows = append(rows, row{cat, t.ID, t.Device, strings.Join(protos, ", "), cmdName})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].typ != rows[j].typ {
			return catOrder[rows[i].typ] < catOrder[rows[j].typ]
		}
		if rows[i].dev != rows[j].dev {
			return rows[i].dev < rows[j].dev
		}
		return rows[i].tag < rows[j].tag
	})

	if len(rows) == 0 {
		fmt.Println("Inga brytare, frånskiljare eller pumpar hittades i konfigurationen.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TYP\tTAGG\tENHET\tPROTOKOLL\tMANÖVERTAGG")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.typ, r.tag, r.dev, r.proto, r.cmd)
	}
	w.Flush()
	fmt.Printf("\n%d st.\n", len(rows))
	return nil
}

// indexByID maps every tag id to its tag in the config.
func indexByID(cfg *model.Config) map[string]*model.Tag {
	m := make(map[string]*model.Tag, len(cfg.Tags))
	for i := range cfg.Tags {
		m[cfg.Tags[i].ID] = &cfg.Tags[i]
	}
	return m
}

// operability reports which protocols can operate the status tag t, and the
// name of its Modbus command tag (or "–"). It is the single source of truth
// for "is this point operable" used by both -list and bulk manoeuvres.
func operability(cfg *model.Config, byID map[string]*model.Tag, t *model.Tag) (protos []string, cmdName string) {
	cmdName = "–"
	if cmd := firstOf(byID, cmdCandidates(t.ID)); cmd != nil && (cmd.Area == model.AreaCoil || cmd.Area == model.AreaHolding) {
		protos = append(protos, "modbus")
		cmdName = cmd.ID
	}
	if dev, ok := deviceByID(cfg, t.Device); ok && dev.IEC != nil && t.CtrlIOA != 0 {
		protos = append(protos, "iec104")
	}
	return protos, cmdName
}

// applyOverrides returns a copy of dev with the -host/-port overrides applied.
// The IEC endpoint is copied so the shared config struct is left untouched.
func applyOverrides(dev model.Device, host string, port int) model.Device {
	if host != "" {
		dev.Host = host
	}
	if port > 0 {
		dev.Port = port
		if dev.IEC != nil {
			iec := *dev.IEC
			iec.Port = port
			dev.IEC = &iec
		}
	}
	return dev
}

// ---- bulk manoeuvre over a whole type ----

// connError marks a failure to reach a device endpoint (the connect step), as
// opposed to a per-point protocol error such as an interlock rejection. In a
// bulk manoeuvre the remaining points on the same endpoint are skipped once a
// connError is seen for it.
type connError struct{ err error }

func (e *connError) Error() string { return e.err.Error() }
func (e *connError) Unwrap() error { return e.err }

func isConnError(err error) bool {
	var c *connError
	return errors.As(err, &c)
}

// connEndpoint is the host:port a manoeuvre connects to, used as the key for
// endpoints already known to be unreachable.
func connEndpoint(dev model.Device, proto string) string {
	port := dev.Port
	if proto == "iec104" && dev.IEC != nil {
		port = dev.IEC.Port
	}
	return fmt.Sprintf("%s:%d", dev.Host, port)
}

// canonProtocol maps a -protocol value (with aliases) to "modbus" or "iec104".
func canonProtocol(p string) (string, error) {
	switch strings.ToLower(p) {
	case "modbus":
		return "modbus", nil
	case "iec104", "iec", "104":
		return "iec104", nil
	}
	return "", fmt.Errorf("okänt protokoll %q (välj modbus eller iec104)", p)
}

// operateTarget detects a bulk manoeuvre: the -category flag, or an "all" or
// "type:<cat>" argument in place of a tag. It returns the canonical category to
// operate ("" means every operable point), the state argument, and whether this
// is a bulk operation. A non-bulk result (bulk == false) falls through to the
// single-tag path.
func operateTarget(category string, args []string) (want, state string, bulk bool, err error) {
	parseType := func(s string) (string, error) {
		c, ok := matchFilter(s)
		if !ok {
			return "", fmt.Errorf("okänd typ %q (brytare, frånskiljare eller pump)", s)
		}
		return c, nil
	}
	if category != "" {
		c, err := parseType(category)
		if err != nil {
			return "", "", true, err
		}
		if len(args) != 1 {
			return "", "", true, errors.New("ange ett tillstånd (open/close) efter -category")
		}
		return c, args[0], true, nil
	}
	if len(args) == 2 {
		switch {
		case strings.EqualFold(args[0], "all"):
			return "", args[1], true, nil
		case strings.HasPrefix(args[0], "type:"):
			c, err := parseType(strings.TrimPrefix(args[0], "type:"))
			if err != nil {
				return "", "", true, err
			}
			return c, args[1], true, nil
		}
	}
	return "", "", false, nil
}

// doBulk operates every operable point of the wanted category (or all operable
// points when want is "") over one protocol. Each point is resolved and operated
// exactly as a single manoeuvre, so the overrides and -verify/-dry-run behave
// identically; the overrides apply to every point, so use them only against a
// single device.
func doBulk(cfg *model.Config, want string, closeOp bool, protocol, host string, port, unit, openVal, closeVal int, timeout time.Duration, verify, dry bool) error {
	proto, err := canonProtocol(protocol)
	if err != nil {
		return err
	}
	byID := indexByID(cfg)
	var targets []*model.Tag
	for i := range cfg.Tags {
		t := &cfg.Tags[i]
		if t.Writable {
			continue
		}
		cat := category(t)
		if cat == "" || (want != "" && cat != want) {
			continue
		}
		if protos, _ := operability(cfg, byID, t); slices.Contains(protos, proto) {
			targets = append(targets, t)
		}
	}
	slices.SortFunc(targets, func(a, b *model.Tag) int {
		if ca, cb := category(a), category(b); ca != cb {
			return catOrder[ca] - catOrder[cb]
		}
		if a.Device != b.Device {
			return strings.Compare(a.Device, b.Device)
		}
		return strings.Compare(a.ID, b.ID)
	})

	label := "alla typer"
	if want != "" {
		label = "typ " + want
	}
	if len(targets) == 0 {
		return fmt.Errorf("inga manövrerbara punkter för %s via %s", label, proto)
	}
	action := "öppna/frånslag"
	if closeOp {
		action = "slut/tillslag"
	}
	fmt.Printf("%s: %s för %s (%d punkt(er))\n\n", proto, action, label, len(targets))

	unreachable := map[string]bool{} // endpoints that already refused a connection
	failed, skipped := 0, 0
	for _, st := range targets {
		r, err := resolve(cfg, st.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n\n", st.ID, err)
			failed++
			continue
		}
		dev := applyOverrides(r.device, host, port)
		endpoint := connEndpoint(dev, proto)
		// Skip points on an endpoint that already refused a connection, instead
		// of waiting for the same timeout again.
		if unreachable[endpoint] {
			fmt.Fprintf(os.Stderr, "  %s: hoppar över, ingen förbindelse med %s\n\n", st.ID, endpoint)
			skipped++
			continue
		}
		if proto == "modbus" {
			err = doModbus(r, dev, closeOp, action, openVal, closeVal, unit, timeout, verify, dry)
		} else {
			err = doIEC(r, dev, closeOp, action, timeout, dry)
		}
		if err != nil {
			failed++
			if isConnError(err) {
				unreachable[endpoint] = true
				fmt.Fprintf(os.Stderr, "  %s: %v – hoppar över återstående punkter mot %s\n", st.ID, err, endpoint)
			} else {
				fmt.Fprintf(os.Stderr, "  %s: %v\n", st.ID, err)
			}
		}
		fmt.Println()
	}

	done := len(targets) - failed - skipped
	fmt.Printf("Klart: %d av %d lyckades", done, len(targets))
	if failed > 0 || skipped > 0 {
		fmt.Printf(" (%d misslyckade, %d överhoppade)", failed, skipped)
	}
	fmt.Println(".")
	if n := failed + skipped; n > 0 {
		return fmt.Errorf("%d punkt(er) kunde inte manövreras", n)
	}
	return nil
}

func defaultValues(area string) (open, close int) {
	if area == model.AreaHolding {
		return 1, 2 // double-point command convention (modsim): 1 = frånslag, 2 = tillslag
	}
	return 0, 1 // coil: 0 = öppna/från, 1 = slut/till
}

// parseState maps a state argument to "close" (true) or "open" (false).
func parseState(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "close", "closed", "on", "1", "slut", "sluten", "till", "tillslag", "stäng":
		return true, nil
	case "open", "off", "0", "öppna", "öppen", "från", "frånslag", "oppna":
		return false, nil
	}
	return false, fmt.Errorf("okänt tillstånd %q (använd open/close, från/till, 0/1)", s)
}

func usage() {
	fmt.Fprint(os.Stderr, `opctl – manövrera brytare och frånskiljare via SCADA-konfigurationen

Användning:
  opctl [flaggor] <tagg> <open|close>
  opctl [flaggor] <all|type:TYP> <open|close>
  opctl [flaggor] -category <TYP> <open|close>
  opctl -list [brytare|frånskiljare|pump]
  opctl -proxy <url> <freeze|unfreeze|block|unblock> <tagg>
  opctl -proxy <url> <freeze|unfreeze|block|unblock> <all|type:TYP>
  opctl -proxy <url> status

Tagg kan vara lägestaggen (t.ex. G1.CB, Q1_POS) eller manövertaggen
(G1.CB.Cmd, Q1_CMD). Tillstånd: open/close, från/till, öppna/slut, 0/1.

Med all, type:TYP eller -category manövreras alla manövrerbara punkter av
typen (brytare, frånskiljare, pump) över det valda protokollet, på samma sätt
som -proxy fryser och blockerar per typ. Eventuella -host/-port/-unit gäller
då varje punkt, så använd dem bara mot en enskild enhet.

Flaggor:
`)
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, `
Exempel:
  opctl -list
  opctl -list pump
  opctl G1.CB close
  opctl -protocol iec104 Q0.CB open
  opctl -data ../blue/data QS1_POS open
  opctl type:brytare open
  opctl -category frånskiljare close
  opctl all open
  opctl -proxy http://127.0.0.1:8083 freeze Grid.U
  opctl -proxy http://127.0.0.1:8083 block type:brytare
  opctl -proxy http://127.0.0.1:8083 unfreeze all
  opctl -proxy http://127.0.0.1:8083 status
`)
}

// *model.Tag, so expose the helpers as functions on model.Tag via a view.
