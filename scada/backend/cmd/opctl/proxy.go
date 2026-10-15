package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// runProxy drives a blue-proxy instance over its HTTP API: freeze/block points
// just like the proxy's own web interface, and show its state.
//
//	opctl -proxy <url> freeze|unfreeze|block|unblock <tag>
//	opctl -proxy <url> freeze|unfreeze|block|unblock -category <typ|all>
//	opctl -proxy <url> status
func runProxy(base, category string, args []string, timeout time.Duration) error {
	base = strings.TrimRight(base, "/")
	if len(args) == 0 {
		return fmt.Errorf("ange en åtgärd: freeze, unfreeze, block, unblock eller status")
	}
	action := strings.ToLower(args[0])
	client := &http.Client{Timeout: timeout}

	if action == "status" {
		return proxyStatus(client, base)
	}

	var endpoint string
	var on bool
	switch action {
	case "freeze", "frys":
		endpoint, on = "hold", true
	case "unfreeze", "release", "släpp", "slapp":
		endpoint, on = "hold", false
	case "block", "blockera":
		endpoint, on = "block", true
	case "unblock", "allow", "tillåt", "tillat":
		endpoint, on = "block", false
	default:
		return fmt.Errorf("okänd åtgärd %q (freeze, unfreeze, block, unblock, status)", action)
	}

	arg := ""
	if len(args) >= 2 {
		arg = args[1]
	}
	// The target is a tag key, or a category when given as "all" / "type:<cat>"
	// (or via -category, which must precede the action because of flag parsing).
	body := map[string]any{"on": on}
	var label string
	switch {
	case category != "":
		body["category"], label = category, "typ "+category
	case arg == "all":
		body["category"], label = "all", "alla punkter"
	case strings.HasPrefix(arg, "type:"):
		cat := strings.TrimPrefix(arg, "type:")
		body["category"], label = cat, "typ "+cat
	case arg != "":
		body["key"], label = arg, arg
	default:
		return fmt.Errorf("ange en tagg, all, type:<typ>, eller -category <typ>")
	}

	var res struct {
		Affected int    `json:"affected"`
		Error    string `json:"error"`
	}
	if err := proxyPost(client, base+"/api/"+endpoint, body, &res); err != nil {
		return err
	}
	if res.Error != "" {
		return fmt.Errorf("%s", res.Error)
	}
	fmt.Printf("%s: %s (%d punkt(er) påverkade)\n", action, label, res.Affected)
	return nil
}

func proxyPost(c *http.Client, url string, body, out any) error {
	b, _ := json.Marshal(body)
	resp, err := c.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("når inte blue-proxy: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("blue-proxy svarade %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

type proxyPoint struct {
	Key          string `json:"key"`
	Category     string `json:"category"`
	Proto        string `json:"proto"`
	CollectedTxt string `json:"collectedTxt"`
	ReportedTxt  string `json:"reportedTxt"`
	Held         bool   `json:"held"`
	Blocked      bool   `json:"blocked"`
	LastCmd      string `json:"lastCmd"`
}

func proxyStatus(c *http.Client, base string) error {
	resp, err := c.Get(base + "/api/state")
	if err != nil {
		return fmt.Errorf("når inte blue-proxy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("blue-proxy svarade %s", resp.Status)
	}
	var state struct {
		Title  string       `json:"title"`
		Points []proxyPoint `json:"points"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return err
	}
	if state.Title != "" {
		fmt.Println(state.Title)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TAGG\tTYP\tPROT\tINSAMLAT\tRAPPORTERAT\tLÄGE\tSENASTE MANÖVER")
	for _, p := range state.Points {
		mode := "transp."
		if p.Held {
			mode = "FRYST"
		}
		if p.Blocked {
			mode += " BLOCKERAD"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			p.Key, p.Category, p.Proto, dash(p.CollectedTxt), dash(p.ReportedTxt), mode, p.LastCmd)
	}
	w.Flush()
	return nil
}

func dash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
