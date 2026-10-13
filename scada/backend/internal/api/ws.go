package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"blueot/scada/internal/model"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type snapshotMsg struct {
	Type    string               `json:"type"`
	Values  []model.Value        `json:"values"`
	Devices []model.DeviceStatus `json:"devices"`
}

// serveWS streams realtime changes to the HMI. The first message is a full
// snapshot; afterwards only changes ("values", "device", "event") are sent.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: %v", err)
		return
	}
	defer conn.Close()

	sub := s.Hub.Subscribe()
	defer s.Hub.Unsubscribe(sub)

	snap, _ := json.Marshal(snapshotMsg{Type: "snapshot", Values: s.Hub.Snapshot(), Devices: s.Hub.Devices()})
	if err := conn.WriteMessage(websocket.TextMessage, snap); err != nil {
		return
	}

	// Reader: handles pong/close frames.
	closed := make(chan struct{})
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-closed:
			return
		case msg, ok := <-sub:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
