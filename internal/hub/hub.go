package hub

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"game-server/internal/client"
	"game-server/internal/config"
	"game-server/internal/game"
	"game-server/internal/models"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type Hub struct {
	mu    sync.Mutex
	clients map[int]*client.Client
	game  *game.Game
	reg   chan *client.Client
	unreg chan *client.Client
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[int]*client.Client, config.MaxPlayers),
		game:    game.NewGame(),
		reg:     make(chan *client.Client),
		unreg:   make(chan *client.Client),
	}
}

func (h *Hub) Unregister(c *client.Client) {
	h.unreg <- c
}

func (h *Hub) Run() {
	ticker := time.NewTicker(config.TickMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case c := <-h.reg:
			h.mu.Lock()
			h.clients[c.PlayerID] = c
			h.mu.Unlock()

			h.game.ConnectPlayer(c.PlayerID)
			log.Printf("[hub] player %d connected (%d/%d)", c.PlayerID, len(h.clients), config.MaxPlayers)

			c.SendJSON(models.ServerMsg{Type: "joined", Payload: models.JoinedPayload{
				PlayerID: c.PlayerID,
			}})
			c.SendJSON(models.ServerMsg{Type: "state", Payload: h.game.Snapshot()})

		case c := <-h.unreg:
			h.mu.Lock()
			delete(h.clients, c.PlayerID)
			h.mu.Unlock()

			h.game.DisconnectPlayer(c.PlayerID)
			log.Printf("[hub] player %d disconnected (%d/%d)", c.PlayerID, len(h.clients), config.MaxPlayers)

		case <-ticker.C:
			state := h.game.Tick()
			data, err := json.Marshal(models.ServerMsg{Type: "state", Payload: state})
			if err != nil {
				continue
			}

			h.mu.Lock()
			for _, c := range h.clients {
				select {
				case c.Send <- data:
				default:
				}
			}
			for _, kill := range state.KillEvents {
				if victim, ok := h.clients[kill.VictimID]; ok {
					victim.SendJSON(models.ServerMsg{Type: "killed", Payload: kill})
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	slot, ok := h.nextSlot()
	if !ok {
		http.Error(w, "game full (max 4 players)", http.StatusServiceUnavailable)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("[ws] upgrade error:", err)
		return
	}
	c := &client.Client{
		Hub:      h,
		Game:     h.game,
		Conn:     conn,
		Send:     make(chan []byte, 64),
		PlayerID: slot,
	}
	h.reg <- c
	go c.WritePump()
	go c.ReadPump()
}

func (h *Hub) nextSlot() (int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := 0; i < config.MaxPlayers; i++ {
		if _, used := h.clients[i]; !used {
			return i, true
		}
	}
	return -1, false
}
