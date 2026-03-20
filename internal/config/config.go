package config

const (
	GridSize        = 1024
	TickMs          = 30
	Speed           = 3
	PlayerSize      = 32
	TileSize        = PlayerSize
	TileCount       = GridSize / TileSize
	BulletSpeed     = Speed * 2
	BulletSize      = 8
	ShootCooldownMs = 300
	MaxBullets      = 5
)
