# Galia Game Server

Galia Game Server is a small, in-memory multiplayer game server written in Go.
Players connect over WebSocket to move, shoot, collect bonuses, and receive live
game updates.

## Run

```sh
go run .
```

The server listens on `http://localhost:8080` and accepts WebSocket connections
at `ws://localhost:8080/ws`.

## Usage

Connect with a WebSocket client, for example:

```sh
npx wscat -c 'ws://localhost:8080/ws?name=Alice'
```

Move or stop:

```json
{"direction":"up"}
{"direction":"stop"}
```

Valid directions are `up`, `down`, `left`, `right`, and `stop`.

Shoot:

```json
{ "action": "shoot" }
```

The server sends live `joined`, `map`, `state`, `stats`, and `kill` messages.
Game state is kept only in memory and resets when the server restarts.

## Configuration

Game settings such as map size, speed, tick rate, bonuses, and shooting limits
are defined in `internal/config/config.go`. The server port is currently fixed
to `8080` in `main.go`.
