package config

const (
	GridSize        = 1024
	TickMs          = 17
	Speed           = 2
	PlayerSize      = 32
	TileSize        = PlayerSize
	TileCount       = GridSize / TileSize
	BulletSpeed     = Speed * 3
	BulletSize      = 8
	ShootCooldownMs = 300
	MaxBullets      = 5

	BonusSpeedMs    = 10000 // speed boost duration ms
	BonusImmunityMs = 10000 // immunity duration ms
	BonusRespawnMs  = 12000 // ms before a collected bonus respawns
	BonusCount      = 4     // total bonuses active on map at once
)
