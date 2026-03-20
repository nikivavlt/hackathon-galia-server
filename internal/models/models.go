package models

// BonusKind maps to the grid values used by the client renderer.
type BonusKind int

const (
	BonusSpeed    BonusKind = 2
	BonusImmunity BonusKind = 3
)

type Bonus struct {
	ID   int       `json:"id"`
	Kind BonusKind `json:"kind"`
	Pos  Pos       `json:"pos"`
}

type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Player struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Pos            Pos    `json:"pos"`
	Connected      bool   `json:"connected"`
	Direction      string `json:"direction"`
	Frags          int    `json:"frags"`
	SpeedUntil     int64  `json:"-"` // internal: UnixMilli when speed expires
	ImmortalUntil  int64  `json:"-"` // internal: UnixMilli when immunity expires
	SpeedActive    bool   `json:"speedActive"`
	ImmortalActive bool   `json:"immortalActive"`
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
	VictimID   int    `json:"-"`
	KillerID   int    `json:"killerId"`
	KillerName string `json:"killerName"`
	VictimName string `json:"victimName"`
}

type StatEntry struct {
	PlayerID int    `json:"playerId"`
	Name     string `json:"name"`
	Frags    int    `json:"frags"`
}

type StatsPayload = []StatEntry

type MapPayload struct {
	Grid [][]int `json:"grid"`
}

type BonusEvent struct {
	PlayerID   int       `json:"playerId"`
	PlayerName string    `json:"playerName"`
	Kind       BonusKind `json:"kind"` // 2=speed, 3=immortality
}

type StatePayload struct {
	Players      []Player      `json:"players"`
	Bullets      []Bullet      `json:"bullets"`
	Bonuses      []Bonus       `json:"bonuses"`
	KillEvents   []KillEvent   `json:"-"`
	BonusEvents  []BonusEvent  `json:"bonusEvents"`
}
