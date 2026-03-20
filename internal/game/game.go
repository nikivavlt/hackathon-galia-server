package game

import (
	"sync"
	"time"

	"game-server/internal/config"
	"game-server/internal/models"
)

type Game struct {
	mu               sync.RWMutex
	players          [config.MaxPlayers]models.Player
	grid             [config.GridSize][config.GridSize]int
	activeDirections [config.MaxPlayers]string
	lastDirections   [config.MaxPlayers]string
	bullets          [config.MaxPlayers][]*models.Bullet
	shootCooldown    [config.MaxPlayers]int64
	nextBulletID     int
}

func NewGame() *Game {
	g := &Game{}
	g.resetPlayers()
	return g
}

func (g *Game) resetPlayers() {
	for i := 0; i < config.MaxPlayers; i++ {
		g.players[i] = models.Player{
			ID:    i,
			Pos:   models.StartPositions[i],
			Color: models.PlayerColors[i],
		}
	}
}

func (g *Game) ConnectPlayer(id int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.players[id].Pos = models.StartPositions[id]
	g.players[id].Connected = true
	g.activeDirections[id] = ""
	g.bullets[id] = nil
	g.shootCooldown[id] = 0
}

func (g *Game) DisconnectPlayer(id int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.players[id].Connected = false
	g.activeDirections[id] = ""
	g.bullets[id] = nil
}

func (g *Game) SetMove(playerID int, dir string) {
	g.mu.Lock()
	if dir == "stop" {
		g.activeDirections[playerID] = ""
	} else {
		g.activeDirections[playerID] = dir
		g.lastDirections[playerID] = dir
	}
	g.mu.Unlock()
}

func (g *Game) Shoot(playerID int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	dir := g.lastDirections[playerID]
	if dir == "" || !g.players[playerID].Connected {
		return
	}

	now := time.Now().UnixMilli()
	if now-g.shootCooldown[playerID] < config.ShootCooldownMs {
		return
	}
	if len(g.bullets[playerID]) >= config.MaxBullets {
		return
	}
	p := g.players[playerID].Pos
	bx := p.X + config.PlayerSize/2 - config.BulletSize/2
	by := p.Y + config.PlayerSize/2 - config.BulletSize/2

	g.bullets[playerID] = append(g.bullets[playerID], &models.Bullet{
		ID:       g.nextBulletID,
		PlayerID: playerID,
		Pos:      models.Pos{X: bx, Y: by},
		Dir:      dir,
	})
	g.nextBulletID++
	g.shootCooldown[playerID] = now
}

func (g *Game) Tick() models.StatePayload {
	g.mu.Lock()
	defer g.mu.Unlock()

	var kills []models.KillEvent

	for i := 0; i < config.MaxPlayers; i++ {
		if !g.players[i].Connected || g.activeDirections[i] == "" {
			continue
		}
		for step := 0; step < config.Speed; step++ {
			next := move(g.players[i].Pos, g.activeDirections[i])
			if !g.walkable(next, i) {
				g.activeDirections[i] = ""
				break
			}
			g.players[i].Pos = next
		}
	}

	for i := 0; i < config.MaxPlayers; i++ {
		active := g.bullets[i][:0]
		for _, b := range g.bullets[i] {
			keep := true
			for step := 0; step < config.BulletSpeed; step++ {
				b.Pos = move(b.Pos, b.Dir)

				if b.Pos.X < 0 || b.Pos.Y < 0 || b.Pos.X+config.BulletSize > config.GridSize || b.Pos.Y+config.BulletSize > config.GridSize {
					keep = false
					break
				}

				hit := false
				for j := 0; j < config.MaxPlayers; j++ {
					if j == b.PlayerID || !g.players[j].Connected {
						continue
					}
					pl := g.players[j]
					if b.Pos.X < pl.Pos.X+config.PlayerSize && b.Pos.X+config.BulletSize > pl.Pos.X &&
						b.Pos.Y < pl.Pos.Y+config.PlayerSize && b.Pos.Y+config.BulletSize > pl.Pos.Y {
						g.players[j].Pos = models.StartPositions[j]
						g.activeDirections[j] = ""
						kills = append(kills, models.KillEvent{VictimID: j, KillerID: b.PlayerID})
						keep = false
						hit = true
						break
					}
				}
				if hit {
					break
				}
			}
			if keep {
				active = append(active, b)
			}
		}
		g.bullets[i] = active
	}

	payload := g.buildPayload()
	payload.KillEvents = kills
	return payload
}

func (g *Game) Snapshot() models.StatePayload {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.buildPayload()
}

func (g *Game) buildPayload() models.StatePayload {
	players := g.players
	for i := range players {
		players[i].Direction = g.lastDirections[i]
	}

	bullets := []models.Bullet{}
	for _, playerBullets := range g.bullets {
		for _, b := range playerBullets {
			bullets = append(bullets, *b)
		}
	}

	return models.StatePayload{Players: players, Bullets: bullets}
}

func (g *Game) walkable(p models.Pos, forPlayer int) bool {
	if p.X < 0 || p.Y < 0 || p.X+config.PlayerSize > config.GridSize || p.Y+config.PlayerSize > config.GridSize {
		return false
	}
	if g.grid[p.Y][p.X] == 1 {
		return false
	}
	for i, pl := range g.players {
		if i == forPlayer || !pl.Connected {
			continue
		}
		if p.X < pl.Pos.X+config.PlayerSize && p.X+config.PlayerSize > pl.Pos.X &&
			p.Y < pl.Pos.Y+config.PlayerSize && p.Y+config.PlayerSize > pl.Pos.Y {
			return false
		}
	}
	return true
}

func move(p models.Pos, dir string) models.Pos {
	switch dir {
	case "up":
		p.Y--
	case "down":
		p.Y++
	case "left":
		p.X--
	case "right":
		p.X++
	}
	return p
}
