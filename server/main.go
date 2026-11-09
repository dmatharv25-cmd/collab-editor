package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/dmatharv25-cmd/collab-editor/server/hub"
)

func main() {
	h := hub.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, h.Text(r.URL.Query().Get("room")))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	addr := ":8080"
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
