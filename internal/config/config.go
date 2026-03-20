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
)
