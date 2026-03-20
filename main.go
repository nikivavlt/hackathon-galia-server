package main

import (
	"log"
	"net/http"

	"game-server/internal/hub"
)

func main() {
	h := hub.NewHub()
	go h.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Println("Game server listening on :8080")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", mux))
}
