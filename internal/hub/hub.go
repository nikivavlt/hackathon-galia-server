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
	mu      sync.Mutex
	clients map[int]*client.Client
	game    *game.Game
	reg     chan *client.Client
	unreg   chan *client.Client
	nextID  int
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[int]*client.Client),
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

			h.game.ConnectPlayer(c.PlayerID, c.Name)
			log.Printf("[hub] player %d connected (%d total)", c.PlayerID, len(h.clients))

			c.SendJSON(models.ServerMsg{Type: "joined", Payload: models.JoinedPayload{
				PlayerID: c.PlayerID,
			}})
			c.SendJSON(models.ServerMsg{Type: "map", Payload: h.game.GetMap()})
			c.SendJSON(models.ServerMsg{Type: "stats", Payload: h.game.GetStats()})
			c.SendJSON(models.ServerMsg{Type: "state", Payload: h.game.Snapshot()})

		case c := <-h.unreg:
			h.mu.Lock()
			delete(h.clients, c.PlayerID)
			h.mu.Unlock()

			h.game.DisconnectPlayer(c.PlayerID)
			log.Printf("[hub] player %d disconnected (%d total)", c.PlayerID, len(h.clients))

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
			if len(state.KillEvents) > 0 {
				statsMsg := models.ServerMsg{Type: "stats", Payload: h.game.GetStats()}
				for _, kill := range state.KillEvents {
					killMsg := models.ServerMsg{Type: "kill", Payload: kill}
					for _, c := range h.clients {
						c.SendJSON(killMsg)
						c.SendJSON(statsMsg)
					}
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Player"
	}
	if len(name) > 20 {
		name = name[:20]
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("[ws] upgrade error:", err)
		return
	}

	h.mu.Lock()
	id := h.nextID
	h.nextID++
	h.mu.Unlock()

	c := &client.Client{
		Hub:      h,
		Game:     h.game,
		Conn:     conn,
		Send:     make(chan []byte, 64),
		PlayerID: id,
		Name:     name,
	}
	h.reg <- c
	go c.WritePump()
	go c.ReadPump()
}
