// Command blue-proxy is a transparent Modbus TCP / IEC 60870-5-104 proxy that
// sits between a SCADA master and an upstream device. It remembers collected
// values and the last commands, and per point or per type the last reported
// value can be held so it keeps being reported regardless of the source. A web
// interface shows collected vs reported values and toggles the hold mode.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"blue-proxy/internal/config"
	"blue-proxy/internal/iecproxy"
	"blue-proxy/internal/mbproxy"
	"blue-proxy/internal/state"
	"blue-proxy/internal/web"
)

// displayURL turns a listen address into a clickable URL, mapping a wildcard
// or empty host to localhost.
func displayURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return host + ":" + port
}

func startIEC(ctx context.Context, listen, upstream string, timeout time.Duration, cfg *config.Loaded, store *state.Store, errc chan<- error) {
	ip := iecproxy.New(listen, upstream, timeout, cfg, store)
	ip.Logf = log.Printf
	go func() { errc <- ip.ListenAndServe(ctx) }()
}

func main() {
	cfgPath := flag.String("config", "../scada/backend/data/config.json", "sökväg till SCADA:s config.json (eller katalogen)")
	device := flag.String("device", "blue-station", "enhet i konfigurationen att proxa")
	mbListen := flag.String("modbus", ":1510", "lyssnaradress för Modbus TCP-proxyn (tom = av)")
	iecListen := flag.String("iec104", ":2414", "lyssnaradress för IEC 104-proxyn (tom = av)")
	httpAddr := flag.String("http", "0.0.0.0:8083", "lyssnaradress för webbgränssnittet, t.ex. 0.0.0.0:8083 eller 127.0.0.1:8083 (tom = av)")
	upstreamMB := flag.String("upstream-modbus", "", "överskrid uppströms Modbus host:port")
	upstreamIEC := flag.String("upstream-iec104", "", "överskrid uppströms IEC 104 host:port")
	timeout := flag.Duration("timeout", 5*time.Second, "uppströms tidsgräns")
	flag.Parse()

	cfg, err := config.Load(*cfgPath, *device)
	if err != nil {
		log.Fatalf("konfiguration: %v", err)
	}
	store := state.New()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 3)

	if *mbListen != "" {
		up := *upstreamMB
		if up == "" {
			up = fmt.Sprintf("%s:%d", cfg.Device.Host, cfg.Device.Port)
		}
		mp := mbproxy.New(*mbListen, up, *timeout, cfg, store)
		mp.Logf = log.Printf
		go func() { errc <- mp.ListenAndServe(ctx) }()
	}

	if *iecListen != "" {
		if cfg.Device.IEC == nil {
			log.Printf("IEC 104-proxy avstängd: enheten %q saknar iec104 i konfigurationen", *device)
		} else {
			up := *upstreamIEC
			if up == "" {
				up = fmt.Sprintf("%s:%d", cfg.Device.Host, cfg.Device.IEC.Port)
			}
			startIEC(ctx, *iecListen, up, *timeout, cfg, store, errc)
		}
	}

	if *httpAddr != "" {
		title := fmt.Sprintf("%s (%s) — proxy", cfg.Device.Name, cfg.Device.ID)
		hs := &http.Server{Addr: *httpAddr, Handler: web.Handler(store, title)}
		go func() { <-ctx.Done(); hs.Close() }()
		go func() {
			if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
		log.Printf("Webbgränssnitt på http://%s", displayURL(*httpAddr))
	}

	select {
	case <-ctx.Done():
		log.Print("Avslutar")
	case err := <-errc:
		if err != nil {
			log.Fatal(err)
		}
	}
}
