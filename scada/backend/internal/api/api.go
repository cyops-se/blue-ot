// Package api exposes the REST and WebSocket interface used by the HMI.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"blueot/scada/internal/comm"
	"blueot/scada/internal/model"
	"blueot/scada/internal/realtime"
	"blueot/scada/internal/store"
	"blueot/scada/internal/web"
)

// Server bundles the dependencies of the HTTP handlers.
type Server struct {
	Store *store.Store
	Hub   *realtime.Hub
	Comm  *comm.Manager
	Web   fs.FS // built HMI served for all non-API paths (optional)
}

// Handler builds the HTTP router.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.Logger)
		r.Get("/config", s.getConfig)

		r.Get("/devices", s.listDevices)
		r.Post("/devices", s.saveDevice)
		r.Put("/devices/{id}", s.saveDevice)
		r.Delete("/devices/{id}", s.deleteDevice)

		r.Get("/tags", s.listTags)
		r.Post("/tags", s.saveTag)
		r.Put("/tags/{id}", s.saveTag)
		r.Delete("/tags/{id}", s.deleteTag)

		r.Get("/values", s.getValues)
		r.Get("/status", s.getStatus)
		r.Get("/history/{tag}", s.getHistory)
		r.Get("/events", s.getEvents)
		r.Post("/commands", s.postCommand)

		r.Get("/views", s.listViews)
		r.Post("/views", s.createView)
		r.Get("/views/{id}", s.getView)
		r.Put("/views/{id}", s.updateView)
		r.Delete("/views/{id}", s.deleteView)

		r.Get("/images", s.listImages)
		r.Post("/images", s.uploadImage)
	})
	r.Get("/ws", s.serveWS)
	r.Handle("/images/*", http.StripPrefix("/images/", http.FileServer(http.Dir(s.Store.ImageDir()))))
	if s.Web != nil {
		r.NotFound(web.Handler(s.Web))
	}
	return r
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if errors.Is(err, store.ErrNotFound) {
		code = http.StatusNotFound
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 10<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("ogiltig JSON: %w", err)
	}
	return nil
}

// ---- configuration ----

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Config())
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Config().Devices)
}

func (s *Server) saveDevice(w http.ResponseWriter, r *http.Request) {
	var d model.Device
	if err := readJSON(r, &d); err != nil {
		writeErr(w, err)
		return
	}
	d, err := s.Store.UpsertDevice(chi.URLParam(r, "id"), d)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Comm.Reload()
	s.Hub.AddEvent("system", "info", "Konfiguration", "Enhet sparad: "+d.ID, "")
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Store.DeleteDevice(id); err != nil {
		writeErr(w, err)
		return
	}
	s.Comm.Reload()
	s.Hub.AddEvent("system", "info", "Konfiguration", "Enhet borttagen: "+id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Store.Config().Tags)
}

func (s *Server) saveTag(w http.ResponseWriter, r *http.Request) {
	t := model.Tag{Bit: -1, Scale: 1}
	if err := readJSON(r, &t); err != nil {
		writeErr(w, err)
		return
	}
	t, err := s.Store.UpsertTag(chi.URLParam(r, "id"), t)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Comm.Reload()
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteTag(chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	s.Comm.Reload()
	w.WriteHeader(http.StatusNoContent)
}

// ---- realtime ----

func (s *Server) getValues(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Hub.Snapshot())
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Hub.Devices())
}

func (s *Server) getHistory(w http.ResponseWriter, r *http.Request) {
	minutes, _ := strconv.Atoi(r.URL.Query().Get("minutes"))
	if minutes <= 0 {
		minutes = 10
	}
	since := time.Now().Add(-time.Duration(minutes) * time.Minute)
	writeJSON(w, http.StatusOK, s.Hub.History(chi.URLParam(r, "tag"), since))
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Hub.Events())
}

func (s *Server) postCommand(w http.ResponseWriter, r *http.Request) {
	var req model.CommandRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	src := req.Source
	if src == "" {
		src = req.Tag
	}
	text := req.Text
	if text == "" {
		text = "Skriv " + strconv.FormatFloat(req.Value, 'f', -1, 64)
	}
	s.Hub.AddEvent("command", "info", src, "Manöver begärd: "+text, req.User)
	if err := s.Comm.Write(req); err != nil {
		s.Hub.AddEvent("command", "alarm", src, "Manöver misslyckades: "+text+" ("+err.Error()+")", req.User)
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Hub.AddEvent("command", "info", src, "Manöver skickad: "+text, req.User)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- views ----

func (s *Server) listViews(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.Views()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) getView(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.View(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) createView(w http.ResponseWriter, r *http.Request) {
	var v model.View
	if err := readJSON(r, &v); err != nil {
		writeErr(w, err)
		return
	}
	v.ID = ""
	v, err := s.Store.SaveView(v)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateView(w http.ResponseWriter, r *http.Request) {
	var v model.View
	if err := readJSON(r, &v); err != nil {
		writeErr(w, err)
		return
	}
	v.ID = chi.URLParam(r, "id")
	v, err := s.Store.SaveView(v)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteView(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteView(chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- images (static graphics) ----

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true}

func (s *Server) listImages(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.Store.ImageDir())
	if err != nil {
		writeErr(w, err)
		return
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() && imageExt[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, "/images/"+e.Name())
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) uploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeErr(w, err)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, err)
		return
	}
	defer f.Close()
	name := filepath.Base(hdr.Filename)
	if !imageExt[strings.ToLower(filepath.Ext(name))] {
		writeErr(w, errors.New("filtypen stöds inte (png, jpg, gif, svg, webp)"))
		return
	}
	name = strings.Map(func(r rune) rune {
		if r == ' ' || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	out, err := os.Create(filepath.Join(s.Store.ImageDir(), name))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, f); err != nil {
		writeErr(w, err)
		return
	}
	log.Printf("bild uppladdad: %s", name)
	writeJSON(w, http.StatusCreated, map[string]string{"url": "/images/" + name})
}
