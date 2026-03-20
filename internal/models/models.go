package models

type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Player struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Pos       Pos    `json:"pos"`
	Connected bool   `json:"connected"`
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

type MapPayload struct {
	Grid [][]int `json:"grid"`
}

type StatePayload struct {
	Players    []Player    `json:"players"`
	Bullets    []Bullet    `json:"bullets"`
	KillEvents []KillEvent `json:"-"`
}
