// Command blue-water runs a simulated drinking water treatment plant (vattenverk) with Modbus TCP and
// IEC 60870-5-104 interfaces and a web dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"blue-water/internal/iec104"
	"blue-water/internal/modbus"
	"blue-water/internal/plant"
	"blue-water/internal/web"
)

func main() {
	mbAddr := flag.String("modbus", ":1503", "lyssnaradress för Modbus TCP (tom = av)")
	iecAddr := flag.String("iec104", ":2406", "lyssnaradress för IEC 60870-5-104 (tom = av)")
	httpAddr := flag.String("http", ":8085", "lyssnaradress för webbgränssnitt (tom = av)")
	ca := flag.Uint("ca", 3, "IEC 104 gemensam adress (common address of ASDU)")
	utc := flag.Bool("iec-utc", false, "tidsstämplar i UTC istället för lokal tid")
	tick := flag.Duration("tick", time.Second, "simuleringens tidssteg")
	timeScale := flag.Float64("timescale", 1, "hastighet på dygnsklockan som styr dygnsprofilerna")
	points := flag.Bool("points", false, "skriv ut signallistan som Markdown och avsluta")
	pointsJSON := flag.Bool("points-json", false, "skriv ut signallistan som JSON och avsluta")
	flag.Parse()

	pl := plant.New(plant.DefaultConfig(), *timeScale)
	if *pointsJSON {
		if err := plant.WritePointJSON(os.Stdout, pl.Points()); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *points {
		plant.WritePointTable(os.Stdout, pl.Points())
		return
	}
	pl.SetEventHook(func(e plant.Event) { log.Printf("HÄNDELSE: %s", e.Text) })

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go pl.Run(ctx, *tick)

	errc := make(chan error, 3)
	if *mbAddr != "" {
		srv := modbus.New(pl)
		srv.Logf = log.Printf
		go func() { errc <- srv.ListenAndServe(ctx, *mbAddr) }()
		log.Printf("Modbus TCP lyssnar på %s", *mbAddr)
	}
	if *iecAddr != "" {
		srv := iec104.New(pl, uint16(*ca))
		srv.UTC = *utc
		srv.Logf = log.Printf
		go func() { errc <- srv.ListenAndServe(ctx, *iecAddr) }()
		log.Printf("IEC 104 lyssnar på %s (CA %d)", *iecAddr, *ca)
	}
	if *httpAddr != "" {
		hs := &http.Server{Addr: *httpAddr, Handler: web.Handler(pl)}
		go func() {
			<-ctx.Done()
			hs.Close()
		}()
		go func() {
			if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
		url := *httpAddr
		if strings.HasPrefix(url, ":") {
			url = "localhost" + url
		}
		log.Printf("Webbgränssnitt på http://%s", url)
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
