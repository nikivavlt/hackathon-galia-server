package game

import (
	"math/rand"
	"sync"
	"time"

	"game-server/internal/config"
	"game-server/internal/models"
)

type Game struct {
	mu               sync.RWMutex
	players          map[int]models.Player
	grid             [config.TileCount][config.TileCount]int
	activeDirections map[int]string
	lastDirections   map[int]string
	bullets          map[int][]*models.Bullet
	shootCooldown    map[int]int64
	nextBulletID     int
}

func NewGame() *Game {
	g := &Game{
		players:          make(map[int]models.Player),
		activeDirections: make(map[int]string),
		lastDirections:   make(map[int]string),
		bullets:          make(map[int][]*models.Bullet),
		shootCooldown:    make(map[int]int64),
	}
	g.buildGrid()
	return g
}

func (g *Game) buildGrid() {
	s := config.TileCount

	for i := 0; i < s; i++ {
		g.grid[0][i] = 1
		g.grid[s-1][i] = 1
		g.grid[i][0] = 1
		g.grid[i][s-1] = 1
	}

	set := func(x, y, w, h int) {
		for dy := 0; dy < h; dy++ {
			for dx := 0; dx < w; dx++ {
				if ty, tx := y+dy, x+dx; ty >= 0 && ty < s && tx >= 0 && tx < s {
					g.grid[ty][tx] = 1
				}
			}
		}
	}

	half := s / 2
	q := s / 4

	set(half-3, q-1, 6, 1)
	set(half-3, half+q-1, 6, 1)
	set(q-1, half-3, 1, 6)
	set(half+q-1, half-3, 1, 6)
	set(half-1, half-5, 1, 10)
	set(half-5, half-1, 10, 1)
	set(q-2, q-1, 4, 1)
	set(q-1, q-2, 1, 4)
	set(half+q-2, q-1, 4, 1)
	set(half+q-1, q-2, 1, 4)
	set(q-2, half+q-1, 4, 1)
	set(q-1, half+q-2, 1, 4)
	set(half+q-2, half+q-1, 4, 1)
	set(half+q-1, half+q-2, 1, 4)
}

func (g *Game) GetMap() models.MapPayload {
	g.mu.RLock()
	defer g.mu.RUnlock()

	grid := make([][]int, config.TileCount)
	for y := 0; y < config.TileCount; y++ {
		row := make([]int, config.TileCount)
		copy(row, g.grid[y][:])
		grid[y] = row
	}
	return models.MapPayload{Grid: grid}
}

func (g *Game) ConnectPlayer(id int, name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.players[id] = models.Player{
		ID:        id,
		Name:      name,
		Pos:       g.findSpawnPos(),
		Connected: true,
	}
	g.activeDirections[id] = ""
	g.bullets[id] = nil
	g.shootCooldown[id] = 0
}

func (g *Game) findSpawnPos() models.Pos {
	safeRadius := config.PlayerSize * 3
	maxX := config.GridSize - config.PlayerSize
	maxY := config.GridSize - config.PlayerSize

	for attempt := 0; attempt < 100; attempt++ {
		p := models.Pos{
			X: rand.Intn(maxX),
			Y: rand.Intn(maxY),
		}
		overlap := false
		for _, pl := range g.players {
			if !pl.Connected {
				continue
			}
			dx := p.X - pl.Pos.X
			dy := p.Y - pl.Pos.Y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			if dx < safeRadius && dy < safeRadius {
				overlap = true
				break
			}
		}
		if !overlap {
			return p
		}
	}
	return models.Pos{X: rand.Intn(maxX), Y: rand.Intn(maxY)}
}

func (g *Game) DisconnectPlayer(id int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.players, id)
	delete(g.activeDirections, id)
	delete(g.lastDirections, id)
	delete(g.bullets, id)
	delete(g.shootCooldown, id)
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

	pl, ok := g.players[playerID]
	if !ok || !pl.Connected {
		return
	}
	dir := g.lastDirections[playerID]
	if dir == "" {
		return
	}
	now := time.Now().UnixMilli()
	if now-g.shootCooldown[playerID] < config.ShootCooldownMs {
		return
	}
	if len(g.bullets[playerID]) >= config.MaxBullets {
		return
	}

	bx := pl.Pos.X + config.PlayerSize/2 - config.BulletSize/2
	by := pl.Pos.Y + config.PlayerSize/2 - config.BulletSize/2

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

	for id, pl := range g.players {
		if !pl.Connected || g.activeDirections[id] == "" {
			continue
		}
		for step := 0; step < config.Speed; step++ {
			next := move(pl.Pos, g.activeDirections[id])
			if !g.walkable(next, id) {
				g.activeDirections[id] = ""
				break
			}
			pl.Pos = next
		}
		g.players[id] = pl
	}

	for id, playerBullets := range g.bullets {
		active := playerBullets[:0]
		for _, b := range playerBullets {
			keep := true
			for step := 0; step < config.BulletSpeed; step++ {
				b.Pos = move(b.Pos, b.Dir)

				tx, ty := b.Pos.X/config.TileSize, b.Pos.Y/config.TileSize
				if b.Pos.X < 0 || b.Pos.Y < 0 || b.Pos.X+config.BulletSize > config.GridSize || b.Pos.Y+config.BulletSize > config.GridSize || g.grid[ty][tx] == 1 {
					keep = false
					break
				}

				hit := false
				for j, pl := range g.players {
					if j == b.PlayerID || !pl.Connected {
						continue
					}
					if b.Pos.X < pl.Pos.X+config.PlayerSize && b.Pos.X+config.BulletSize > pl.Pos.X &&
						b.Pos.Y < pl.Pos.Y+config.PlayerSize && b.Pos.Y+config.BulletSize > pl.Pos.Y {
						respawned := pl
						respawned.Pos = g.findSpawnPos()
						g.players[j] = respawned
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
		g.bullets[id] = active
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
	players := make([]models.Player, 0, len(g.players))
	for id, pl := range g.players {
		pl.Direction = g.lastDirections[id]
		players = append(players, pl)
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
	tx, ty := p.X/config.TileSize, p.Y/config.TileSize
	if g.grid[ty][tx] == 1 {
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
