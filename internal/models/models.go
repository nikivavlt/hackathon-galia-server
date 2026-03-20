package models

import "game-server/internal/config"

var StartPositions = [config.MaxPlayers]Pos{
	{0, 0},
	{config.GridSize - config.PlayerSize, 0},
	{0, config.GridSize - config.PlayerSize},
	{config.GridSize - config.PlayerSize, config.GridSize - config.PlayerSize},
}

var PlayerColors = [config.MaxPlayers]string{"blue", "green", "orange", "purple"}

type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Player struct {
	ID        int    `json:"id"`
	Pos       Pos    `json:"pos"`
	Connected bool   `json:"connected"`
	Color     string `json:"color"`
	Direction string `json:"direction"`
}

type Bullet struct {
	ID       int    `json:"id"`
	PlayerID int    `json:"playerId"`
	Pos      Pos    `json:"pos"`
	Dir      string `json:"direction"`
}

type ClientMsg struct {
	Direction string `json:"direction"`
	Action    string `json:"action"`
}

type ServerMsg struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type JoinedPayload struct {
	PlayerID int `json:"playerId"`
}

type KillEvent struct {
	VictimID int `json:"-"`
	KillerID int `json:"killerId"`
}

type StatePayload struct {
	Players    [config.MaxPlayers]Player `json:"players"`
	Bullets    []Bullet                  `json:"bullets"`
	KillEvents []KillEvent               `json:"-"`
}
