package client

import (
	"encoding/json"
	"log"
	"time"

	"game-server/internal/models"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 512
)

type GameController interface {
	SetMove(playerID int, dir string)
	Shoot(playerID int)
}

type Hub interface {
	Unregister(c *Client)
}

type Client struct {
	Hub      Hub
	Game     GameController
	Conn     *websocket.Conn
	Send     chan []byte
	PlayerID int
	Name     string
}

func (c *Client) SendJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case c.Send <- data:
	default:
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMsgSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
				log.Printf("[client %d] read error: %v", c.PlayerID, err)
			}
			return
		}
		var cmd models.ClientMsg
		if err := json.Unmarshal(raw, &cmd); err != nil {
			continue
		}
		if validDir(cmd.Direction) {
			c.Game.SetMove(c.PlayerID, cmd.Direction)
		}
		if cmd.Action == "shoot" {
			c.Game.Shoot(c.PlayerID)
		}
	}
}

func (c *Client) WritePump() {
	ping := time.NewTicker(pingPeriod)
	defer func() {
		ping.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case data, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ping.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func validDir(d string) bool {
	switch d {
	case "up", "down", "left", "right", "stop":
		return true
	}
	return false
}
