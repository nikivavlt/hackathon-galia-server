package game

import (
	"math/rand"
	"sync"
	"time"

	"game-server/internal/config"
	"game-server/internal/models"
)

type pendingBonus struct {
	kind    models.BonusKind
	spawnAt int64 // UnixMilli when this bonus should reappear
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
	gridDirty        bool // true when bonus tiles changed; hub should re-broadcast map
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

	// Outer border walls
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

// generateWalls builds an open-arena map:
// start with a fully open interior, then scatter a small number of
// 4-fold-symmetric cover structures across a regular grid.
// Every path between any two structures is guaranteed ≥4 tiles wide.
func (g *Game) generateWalls() {
	s := config.TileCount // 32 tiles

	// wall writes a wall tile, clamped inside the border.
	wall := func(tx, ty int) {
		if tx > 0 && tx < s-1 && ty > 0 && ty < s-1 {
			g.grid[ty][tx] = 1
		}
	}

	// block fills a rectangle with walls.
	block := func(x, y, w, h int) {
		for dy := 0; dy < h; dy++ {
			for dx := 0; dx < w; dx++ {
				wall(x+dx, y+dy)
			}
		}
	}

	// place4 stamps a shape at (cx,cy) and its 3 mirror copies.
	// cx,cy are in the top-left quadrant; mirrors fill the other three.
	place4 := func(cx, cy int, draw func(ox, oy int)) {
		for _, sx := range []int{1, -1} {
			for _, sy := range []int{1, -1} {
				draw(cx*sx + (s/2)*(1-sx)/2*2, cy*sy + (s/2)*(1-sy)/2*2)
			}
		}
	}
	_ = place4

	// mirror4 stamps shape centred at (cx,cy) and its 4-fold mirror.
	// cx,cy are offsets from map centre (s/2).
	mirror4 := func(cx, cy int, draw func(ax, ay int)) {
		half := s / 2
		draw(half+cx, half+cy)
		draw(half-cx, half+cy)
		draw(half+cx, half-cy)
		draw(half-cx, half-cy)
	}

	// ── Cover shape library ─────────────────────────────────────────────────
	// Each shape is a small cluster of wall tiles relative to an anchor (ax,ay).
	shapes := []func(ax, ay int){
		// 2×2 solid pillar
		func(ax, ay int) { block(ax, ay, 2, 2) },
		// 3×1 horizontal bar
		func(ax, ay int) { block(ax-1, ay, 3, 1) },
		// 1×3 vertical bar
		func(ax, ay int) { block(ax, ay-1, 1, 3) },
		// L-shape (2 tiles right, 2 tiles down)
		func(ax, ay int) {
			block(ax, ay, 3, 1) // horizontal arm
			block(ax, ay, 1, 3) // vertical arm
		},
		// T-shape
		func(ax, ay int) {
			block(ax-1, ay, 3, 1) // top bar
			block(ax, ay+1, 1, 2) // stem
		},
		// Z-shape
		func(ax, ay int) {
			block(ax, ay, 2, 1)
			block(ax+1, ay+1, 2, 1)
		},
		// Plus / cross
		func(ax, ay int) {
			block(ax-1, ay, 3, 1)
			block(ax, ay-1, 1, 3)
		},
	}

	// ── Grid placement ───────────────────────────────────────────────────
	// Divide the top-left quadrant into cells of size `cell`.
	// Each cell gets a shape with probability `density`.
	// cell=7 → paths between objects always ≥4 tiles wide (7-3=4).
	const cell = 7
	const density = 55 // percent chance a cell gets a shape

	// Keep 3 tiles clear of border so corner spawn areas stay open.
	half := s / 2
	for cy := 3; cy < half-2; cy += cell {
		for cx := 3; cx < half-2; cx += cell {
			if rand.Intn(100) >= density {
				continue
			}
			// Jitter anchor slightly within the cell for organic feel.
			jx := cx + rand.Intn(3)
			jy := cy + rand.Intn(3)
			shape := shapes[rand.Intn(len(shapes))]
			// Stamp 4-fold symmetric: top-left, top-right, bottom-left, bottom-right.
			mirror4(jx, jy, shape)
		}
	}

	// ── Centre piece ───────────────────────────────────────────────────────
	// Always place one centrepiece for map identity.
	mid := s / 2
	switch rand.Intn(4) {
	case 0: // open centre — just a ring of pillars
		for _, d := range []int{-3, 3} {
			block(mid+d, mid-1, 1, 2)
			block(mid-1, mid+d, 2, 1)
		}
	case 1: // solid 3×3 centre block
		block(mid-1, mid-1, 3, 3)
	case 2: // hollow 5×5 centre frame (open inside)
		block(mid-2, mid-2, 5, 1)
		block(mid-2, mid+2, 5, 1)
		block(mid-2, mid-1, 1, 3)
		block(mid+2, mid-1, 1, 3)
	case 3: // diagonal pillars
		for _, d := range []int{-2, 2} {
			block(mid+d, mid+d, 2, 2)
			block(mid-d-1, mid+d, 2, 2)
		}
	}
}

// spawnInitialBonuses places BonusCount bonuses evenly split between kinds.
func (g *Game) spawnInitialBonuses() {
	kinds := []models.BonusKind{
		models.BonusSpeed, models.BonusSpeed,
		models.BonusImmunity, models.BonusImmunity,
	}
	for i := 0; i < config.BonusCount; i++ {
		g.spawnBonus(kinds[i%len(kinds)])
	}
}

// spawnBonus places one bonus of the given kind on a random open tile,
// keeping at least 4 tiles away from every other bonus.
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
		g.grid[ty][tx] = int(kind) // stamp 2 or 3 into the grid
		g.gridDirty = true
		return
	}
}

// checkBonusPickup checks whether the player stepped on any bonus this tick,
// applies the effect, and returns any BonusEvent that occurred.
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
			g.grid[ty][tx] = 0 // clear bonus tile from grid
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

// tickBonuses respawns any bonus whose cooldown has elapsed.
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

// ConsumeGridDirty returns true (and resets the flag) if the grid changed
// since the last call — hub uses this to re-broadcast the map message.
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

	now := time.Now().UnixMilli()
	var kills []models.KillEvent

	// ── 1. Respawn collected bonuses whose timer elapsed ────────────────────
	g.tickBonuses(now)

	// ── 2. Move players, apply speed buff, pick up bonuses ────────────────
	var bonusEvents []models.BonusEvent
	for id, pl := range g.players {
		if !pl.Connected || g.activeDirections[id] == "" {
			continue
		}
		// Derive active flags from timestamps each tick
		pl.SpeedActive = pl.SpeedUntil > now
		pl.ImmortalActive = pl.ImmortalUntil > now
		speed := config.Speed
		if pl.SpeedActive {
			speed = config.Speed * 2
		}
		for step := 0; step < speed; step++ {
			next := move(pl.Pos, g.activeDirections[id])
			if !g.walkable(next, id) {
				g.activeDirections[id] = ""
				break
			}
			pl.Pos = next
		}
		var ev *models.BonusEvent
		pl, ev = g.checkBonusPickup(pl, now)
		if ev != nil {
			bonusEvents = append(bonusEvents, *ev)
		}
		g.players[id] = pl
	}

	// ── 3. Advance bullets + hit detection ──────────────────────────────
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
						if pl.ImmortalActive {
							continue // bullet passes through immortal player
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

	bonuses := make([]models.Bonus, 0, len(g.bonuses))
	for _, b := range g.bonuses {
		bonuses = append(bonuses, *b)
	}

	return models.StatePayload{Players: players, Bullets: bullets, Bonuses: bonuses}
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
