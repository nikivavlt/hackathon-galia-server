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

	g.generateWalls()
	g.ensureConnectivity()
}

func (g *Game) generateWalls() {
	s := config.TileCount
	half := s / 2
	cornerClear := 4

	set := func(x, y, w, h int) {
		for dy := 0; dy < h; dy++ {
			for dx := 0; dx < w; dx++ {
				tx, ty := x+dx, y+dy
				if tx <= 0 || ty <= 0 || tx >= s-1 || ty >= s-1 {
					continue
				}
				if tx < 1+cornerClear && ty < 1+cornerClear {
					continue
				}
				if tx > s-2-cornerClear && ty < 1+cornerClear {
					continue
				}
				if tx < 1+cornerClear && ty > s-2-cornerClear {
					continue
				}
				if tx > s-2-cornerClear && ty > s-2-cornerClear {
					continue
				}
				g.grid[ty][tx] = 1
			}
		}
	}

	mirror4 := func(x, y, w, h int) {
		set(x, y, w, h)
		set(s-1-x-w, y, w, h)
		set(x, s-1-y-h, w, h)
		set(s-1-x-w, s-1-y-h, w, h)
	}

	mirrorCenter := func(x, y, w, h int) {
		set(x, y, w, h)
		set(s-1-x-w, s-1-y-h, w, h)
	}

	segmentCount := 8 + rand.Intn(7)

	for i := 0; i < segmentCount; i++ {
		shape := rand.Intn(4)
		qx := 1 + cornerClear + rand.Intn(half-2-cornerClear)
		qy := 1 + cornerClear + rand.Intn(half-2-cornerClear)

		switch shape {
		case 0:
			l := 2 + rand.Intn(4)
			mirror4(qx, qy, l, 1)
		case 1:
			l := 2 + rand.Intn(4)
			mirror4(qx, qy, 1, l)
		case 2:
			l := 2 + rand.Intn(3)
			mirror4(qx, qy, l, 1)
			mirror4(qx, qy, 1, l)
		case 3:
			l := 2 + rand.Intn(3)
			mirror4(qx, qy, l, 1)
			mirror4(qx+l-1, qy, 1, l)
		}
	}

	centerCount := 1 + rand.Intn(3)
	for i := 0; i < centerCount; i++ {
		shape := rand.Intn(3)
		cx := half - 2 + rand.Intn(4)
		cy := half - 2 + rand.Intn(4)
		switch shape {
		case 0:
			l := 2 + rand.Intn(4)
			mirrorCenter(cx, cy, l, 1)
		case 1:
			l := 2 + rand.Intn(4)
			mirrorCenter(cx, cy, 1, l)
		case 2:
			mirrorCenter(cx, cy, 3, 1)
			mirrorCenter(cx, cy, 1, 3)
		}
	}
}

func (g *Game) ensureConnectivity() {
	s := config.TileCount

	findStart := func() (int, int) {
		for y := 1; y < s-1; y++ {
			for x := 1; x < s-1; x++ {
				if g.grid[y][x] == 0 {
					return x, y
				}
			}
		}
		return 1, 1
	}

	floodFill := func(sx, sy int) map[[2]int]bool {
		visited := make(map[[2]int]bool)
		queue := [][2]int{{sx, sy}}
		visited[[2]int{sx, sy}] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, d := range [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}} {
				nx, ny := cur[0]+d[0], cur[1]+d[1]
				if nx <= 0 || ny <= 0 || nx >= s-1 || ny >= s-1 {
					continue
				}
				if g.grid[ny][nx] == 1 {
					continue
				}
				if visited[[2]int{nx, ny}] {
					continue
				}
				visited[[2]int{nx, ny}] = true
				queue = append(queue, [2]int{nx, ny})
			}
		}
		return visited
	}

	for pass := 0; pass < 3; pass++ {
		sx, sy := findStart()
		reachable := floodFill(sx, sy)

		for y := 1; y < s-1; y++ {
			for x := 1; x < s-1; x++ {
				if g.grid[y][x] == 0 && !reachable[[2]int{x, y}] {
					g.grid[y][x] = 1
				}
			}
		}
	}
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
	maxTX := config.TileCount - 2
	maxTY := config.TileCount - 2

	for attempt := 0; attempt < 300; attempt++ {
		tx := 1 + rand.Intn(maxTX-1)
		ty := 1 + rand.Intn(maxTY-1)
		p := models.Pos{
			X: tx * config.TileSize,
			Y: ty * config.TileSize,
		}

		if !g.spawnWalkable(p) {
			continue
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
	return g.fallbackSpawnPos()
}

func (g *Game) spawnWalkable(p models.Pos) bool {
	for dy := 0; dy < config.PlayerSize; dy += config.TileSize {
		for dx := 0; dx < config.PlayerSize; dx += config.TileSize {
			tx := (p.X + dx) / config.TileSize
			ty := (p.Y + dy) / config.TileSize
			if tx < 0 || ty < 0 || tx >= config.TileCount || ty >= config.TileCount {
				return false
			}
			if g.grid[ty][tx] == 1 {
				return false
			}
		}
	}
	return true
}

func (g *Game) fallbackSpawnPos() models.Pos {
	margin := config.TileSize * 2
	for ty := 1; ty < config.TileCount-1; ty++ {
		for tx := 1; tx < config.TileCount-1; tx++ {
			if g.grid[ty][tx] == 0 {
				return models.Pos{
					X: tx*config.TileSize + margin,
					Y: ty*config.TileSize + margin,
				}
			}
		}
	}
	return models.Pos{X: margin, Y: margin}
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
						killer := g.players[b.PlayerID]
						killer.Frags++
						g.players[b.PlayerID] = killer
						kills = append(kills, models.KillEvent{
							VictimID:   j,
							KillerID:   b.PlayerID,
							KillerName: killer.Name,
							VictimName: pl.Name,
						})
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

func (g *Game) GetStats() models.StatsPayload {
	g.mu.RLock()
	defer g.mu.RUnlock()

	leaderboard := make([]models.StatEntry, 0, len(g.players))
	for _, pl := range g.players {
		leaderboard = append(leaderboard, models.StatEntry{
			PlayerID: pl.ID,
			Name:     pl.Name,
			Frags:    pl.Frags,
		})
	}
	for i := 0; i < len(leaderboard)-1; i++ {
		for j := i + 1; j < len(leaderboard); j++ {
			if leaderboard[j].Frags > leaderboard[i].Frags {
				leaderboard[i], leaderboard[j] = leaderboard[j], leaderboard[i]
			}
		}
	}
	return leaderboard
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
	corners := [4][2]int{
		{p.X, p.Y},
		{p.X + config.PlayerSize - 1, p.Y},
		{p.X, p.Y + config.PlayerSize - 1},
		{p.X + config.PlayerSize - 1, p.Y + config.PlayerSize - 1},
	}
	for _, c := range corners {
		if g.grid[c[1]/config.TileSize][c[0]/config.TileSize] == 1 {
			return false
		}
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
