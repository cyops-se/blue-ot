// Command blue-station runs a simulated 220/20 kV transformer substation with
// Modbus TCP and IEC 60870-5-104 interfaces and a web dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"blue-station/internal/chplink"
	"blue-station/internal/iec104"
	"blue-station/internal/modbus"
	"blue-station/internal/station"
	"blue-station/internal/web"
)

func main() {
	mbAddr := flag.String("modbus", ":502", "lyssnaradress för Modbus TCP (tom = av)")
	iecAddr := flag.String("iec104", ":2404", "lyssnaradress för IEC 60870-5-104 (tom = av)")
	httpAddr := flag.String("http", ":8080", "lyssnaradress för webbgränssnitt (tom = av)")
	ca := flag.Uint("ca", 1, "IEC 104 gemensam adress (common address of ASDU)")
	utc := flag.Bool("iec-utc", false, "tidsstämplar i UTC istället för lokal tid")
	tick := flag.Duration("tick", time.Second, "simuleringens tidssteg")
	timeScale := flag.Float64("timescale", 1, "hastighet på dygnsklockan som styr lastprofilerna (t.ex. 60 = en timme per minut)")
	points := flag.Bool("points", false, "skriv ut signallistan som Markdown och avsluta")
	chp := flag.String("chp", "http://127.0.0.1:8082", "adress till blue-chp för fack G1 (tom = ingen kraftvärme)")
	flag.Parse()

	st := station.New(station.DefaultConfig(), *timeScale)
	if *points {
		station.WritePointTable(os.Stdout, st.Points())
		return
	}
	st.SetEventHook(func(e station.Event) { log.Printf("HÄNDELSE: %s", e.Text) })

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go st.Run(ctx, *tick)
	if *chp != "" {
		go chplink.Run(ctx, st, *chp, *tick, log.Printf)
		log.Printf("Fack G1 kopplat mot kraftvärmeverket på %s", *chp)
	}

	errc := make(chan error, 3)
	if *mbAddr != "" {
		srv := modbus.New(st)
		srv.Logf = log.Printf
		go func() { errc <- srv.ListenAndServe(ctx, *mbAddr) }()
		log.Printf("Modbus TCP lyssnar på %s", *mbAddr)
	}
	if *iecAddr != "" {
		srv := iec104.New(st, uint16(*ca))
		srv.UTC = *utc
		srv.Logf = log.Printf
		go func() { errc <- srv.ListenAndServe(ctx, *iecAddr) }()
		log.Printf("IEC 104 lyssnar på %s (CA %d)", *iecAddr, *ca)
	}
	if *httpAddr != "" {
		hs := &http.Server{Addr: *httpAddr, Handler: web.Handler(st)}
		go func() {
			<-ctx.Done()
			hs.Close()
		}()
		go func() {
			if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
		log.Printf("Webbgränssnitt på http://localhost%s", *httpAddr)
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
