// Command scada runs the SCADA backend: Modbus TCP / IEC 104 polling, REST API,
// WebSocket push and the HMI, which is embedded in the executable.
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"blueot/scada/internal/api"
	"blueot/scada/internal/comm"
	"blueot/scada/internal/realtime"
	"blueot/scada/internal/store"
	"blueot/scada/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dataDir := flag.String("data", "data", "data directory (config.json, views/, images/)")
	webDir := flag.String("web", "", "serve the HMI from this directory instead of the embedded build (development)")
	flag.Parse()

	st, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	hub := realtime.New()
	mgr := comm.NewManager(st, hub)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	mgr.Start(ctx)
	hub.AddEvent("system", "info", "SCADA", "Server startad", "")

	var hmi fs.FS
	if *webDir != "" {
		hmi = os.DirFS(*webDir)
		log.Printf("HMI serveras från %s", *webDir)
	} else if files, ok := web.Embedded(); ok {
		hmi = files
		log.Printf("HMI serveras från inbäddad build")
	} else {
		hmi = files
		log.Printf("varning: ingen HMI inbäddad – kör 'npm run build' i scada/frontend och bygg om")
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&api.Server{Store: st, Hub: hub, Comm: mgr, Web: hmi}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("HTTP lyssnar på %s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
