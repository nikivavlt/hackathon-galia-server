package game

import (
	"math/rand"
	"sort"
	"sync"
	"time"

	"game-server/internal/config"
	"game-server/internal/models"
)

type pendingBonus struct {
	kind    models.BonusKind
	spawnAt int64
}

type Game struct {
	mu               sync.RWMutex
	players          map[int]models.Player
	grid             [config.TileCount][config.TileCount]int
	activeDirections map[int]string
	lastDirections   map[int]string
	bullets          map[int][]*models.Bullet
	shootCooldown    map[int]int64
	nextBulletID     int
	bonuses          map[int]*models.Bonus
	nextBonusID      int
	pendingBonuses   []pendingBonus
	gridDirty        bool
}

func NewGame() *Game {
	g := &Game{
		players:          make(map[int]models.Player),
		activeDirections: make(map[int]string),
		lastDirections:   make(map[int]string),
		bullets:          make(map[int][]*models.Bullet),
		shootCooldown:    make(map[int]int64),
		bonuses:          make(map[int]*models.Bonus),
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
	g.spawnInitialBonuses()
}

func (g *Game) generateWalls() {
	s := config.TileCount

	bl := func(x, y, w, h int) {
		for ty := y; ty < y+h; ty++ {
			for tx := x; tx < x+w; tx++ {
				if tx > 0 && tx < s-1 && ty > 0 && ty < s-1 {
					g.grid[ty][tx] = 1
				}
			}
		}
	}

	switch rand.Intn(5) {

	case 0:
		bl(1, 6, 14, 2)
		bl(17, 12, 14, 2)
		bl(1, 18, 14, 2)
		bl(17, 24, 14, 2)
		bl(17, 8, 2, 2)
		bl(11, 15, 2, 2)
		bl(17, 21, 2, 2)
		bl(4, 2, 2, 2)
		bl(26, 2, 2, 2)

	case 1:
		bl(2, 2, 8, 2); bl(2, 2, 2, 8)
		bl(22, 2, 8, 2); bl(28, 2, 2, 8)
		bl(2, 28, 8, 2); bl(2, 22, 2, 8)
		bl(22, 28, 8, 2); bl(28, 22, 2, 8)
		bl(8, 8, 2, 2); bl(22, 8, 2, 2)
		bl(8, 22, 2, 2); bl(22, 22, 2, 2)
		bl(14, 14, 4, 4)

	case 2:
		bl(11, 5, 2, 22)
		bl(19, 5, 2, 22)
		bl(13, 14, 6, 3)
		bl(5, 8, 2, 6); bl(25, 8, 2, 6)
		bl(5, 19, 2, 6); bl(25, 19, 2, 6)

	case 3:
		bl(3, 6, 11, 2); bl(18, 6, 11, 2)
		bl(3, 24, 11, 2); bl(18, 24, 11, 2)
		bl(6, 3, 2, 11); bl(6, 18, 2, 11)
		bl(24, 3, 2, 11); bl(24, 18, 2, 11)
		bl(12, 12, 2, 2); bl(18, 12, 2, 2)
		bl(12, 18, 2, 2); bl(18, 18, 2, 2)

	case 4:
		bl(1, 5, 10, 2); bl(21, 9, 10, 2)
		bl(1, 13, 10, 2); bl(21, 17, 10, 2)
		bl(1, 21, 10, 2); bl(21, 25, 10, 2)
		bl(15, 8, 2, 2); bl(15, 16, 2, 2); bl(15, 24, 2, 2)
	}
}

func (g *Game) spawnInitialBonuses() {
	kinds := []models.BonusKind{
		models.BonusSpeed, models.BonusSpeed,
		models.BonusImmunity, models.BonusImmunity,
	}
	for i := 0; i < config.BonusCount; i++ {
		g.spawnBonus(kinds[i%len(kinds)])
	}
}

func (g *Game) spawnBonus(kind models.BonusKind) {
	for attempt := 0; attempt < 300; attempt++ {
		tx := 2 + rand.Intn(config.TileCount-4)
		ty := 2 + rand.Intn(config.TileCount-4)
		if g.grid[ty][tx] != 0 {
			continue
		}
		p := models.Pos{X: tx * config.TileSize, Y: ty * config.TileSize}
		tooClose := false
		for _, b := range g.bonuses {
			dx := b.Pos.X - p.X
			dy := b.Pos.Y - p.Y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			if dx < config.TileSize*4 && dy < config.TileSize*4 {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		id := g.nextBonusID
		g.nextBonusID++
		g.bonuses[id] = &models.Bonus{ID: id, Kind: kind, Pos: p}
		g.grid[ty][tx] = int(kind)
		g.gridDirty = true
		return
	}
}

func (g *Game) checkBonusPickup(pl models.Player, now int64) (models.Player, *models.BonusEvent) {
	pcx := pl.Pos.X + config.PlayerSize/2
	pcy := pl.Pos.Y + config.PlayerSize/2
	for id, b := range g.bonuses {
		bcx := b.Pos.X + config.TileSize/2
		bcy := b.Pos.Y + config.TileSize/2
		dx := pcx - bcx
		dy := pcy - bcy
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if dx < config.TileSize && dy < config.TileSize {
			tx := b.Pos.X / config.TileSize
			ty := b.Pos.Y / config.TileSize
			g.grid[ty][tx] = 0
			g.gridDirty = true
			delete(g.bonuses, id)
			g.pendingBonuses = append(g.pendingBonuses, pendingBonus{
				kind:    b.Kind,
				spawnAt: now + config.BonusRespawnMs,
			})
			switch b.Kind {
			case models.BonusSpeed:
				pl.SpeedUntil = now + config.BonusSpeedMs
				pl.SpeedActive = true
			case models.BonusImmunity:
				pl.ImmortalUntil = now + config.BonusImmunityMs
				pl.ImmortalActive = true
			}
			return pl, &models.BonusEvent{
				PlayerID:   pl.ID,
				PlayerName: pl.Name,
				Kind:       b.Kind,
			}
		}
	}
	return pl, nil
}

func (g *Game) tickBonuses(now int64) {
	remaining := g.pendingBonuses[:0]
	for _, pb := range g.pendingBonuses {
		if now >= pb.spawnAt {
			g.spawnBonus(pb.kind)
		} else {
			remaining = append(remaining, pb)
		}
	}
	g.pendingBonuses = remaining
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

	for {
		sx, sy := findStart()
		reachable := floodFill(sx, sy)

		changed := false
		for y := 1; y < s-1; y++ {
			for x := 1; x < s-1; x++ {
				if g.grid[y][x] == 0 && !reachable[[2]int{x, y}] {
					g.grid[y][x] = 1
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
}

func (g *Game) ConsumeGridDirty() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gridDirty {
		g.gridDirty = false
		return true
	}
	return false
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
	for ty := 1; ty < config.TileCount-1; ty++ {
		for tx := 1; tx < config.TileCount-1; tx++ {
			if g.grid[ty][tx] == 0 {
				return models.Pos{
					X: tx * config.TileSize,
					Y: ty * config.TileSize,
				}
			}
		}
	}
	return models.Pos{X: config.TileSize, Y: config.TileSize}
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
	defer g.mu.Unlock()
	if dir == "stop" {
		g.activeDirections[playerID] = ""
	} else {
		g.activeDirections[playerID] = dir
		g.lastDirections[playerID] = dir
	}
}

func (g *Game) Shoot(playerID int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	pl, ok := g.players[playerID]
	if !ok {
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

	now := time.Now().UnixMilli()
	var kills []models.KillEvent

	g.tickBonuses(now)

	var bonusEvents []models.BonusEvent
	for id, pl := range g.players {
		if g.activeDirections[id] == "" {
			continue
		}
		pl.SpeedActive = pl.SpeedUntil > now
		pl.ImmortalActive = pl.ImmortalUntil > now
		speed := config.Speed
		if pl.SpeedActive {
			speed = config.Speed * 2
		}
		dir := g.activeDirections[id]
		for step := 0; step < speed; step++ {
			next := move(pl.Pos, dir)
			if g.walkable(next, id) {
				pl.Pos = next
				continue
			}
			if slid, ok := g.slideInto(pl.Pos, dir, id); ok {
				pl.Pos = slid
			}
			break
		}
		var ev *models.BonusEvent
		pl, ev = g.checkBonusPickup(pl, now)
		if ev != nil {
			bonusEvents = append(bonusEvents, *ev)
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
					if j == b.PlayerID {
						continue
					}
					if b.Pos.X < pl.Pos.X+config.PlayerSize && b.Pos.X+config.BulletSize > pl.Pos.X &&
						b.Pos.Y < pl.Pos.Y+config.PlayerSize && b.Pos.Y+config.BulletSize > pl.Pos.Y {
						if pl.ImmortalActive {
						continue
						}
						respawned := pl
						respawned.Pos = g.findSpawnPos()
						respawned.SpeedActive = false
						respawned.SpeedUntil = 0
						respawned.ImmortalActive = false
						respawned.ImmortalUntil = 0
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
	payload.BonusEvents = bonusEvents
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
	sort.Slice(leaderboard, func(i, j int) bool {
		return leaderboard[i].Frags > leaderboard[j].Frags
	})
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

	bonuses := make([]models.Bonus, 0, len(g.bonuses))
	for _, b := range g.bonuses {
		bonuses = append(bonuses, *b)
	}

	return models.StatePayload{Players: players, Bullets: bullets, Bonuses: bonuses}
}

func (g *Game) slideInto(pos models.Pos, dir string, id int) (models.Pos, bool) {
	nudgeX := dir == "up" || dir == "down"
	var val int
	if nudgeX {
		val = pos.X
	} else {
		val = pos.Y
	}
	mod := val % config.TileSize
	if mod == 0 {
		return pos, false
	}
	var delta int
	if mod <= config.TileSize/2 {
		delta = -1
	} else {
		delta = 1
	}
	candidate := pos
	if nudgeX {
		candidate.X += delta
	} else {
		candidate.Y += delta
	}
	if g.walkable(candidate, id) {
		return candidate, true
	}
	return pos, false
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
		if i == forPlayer {
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
