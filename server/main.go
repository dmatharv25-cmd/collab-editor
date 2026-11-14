package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/dmatharv25-cmd/collab-editor/server/hub"
	"github.com/dmatharv25-cmd/collab-editor/server/store"
)

func main() {
	dbPath := flag.String("db", "collab.db", "SQLite file for persistence (empty = memory only)")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	var h *hub.Hub
	if *dbPath == "" {
		h = hub.New()
		log.Println("persistence: off (memory only)")
	} else {
		s, err := store.Open(*dbPath)
		if err != nil {
			log.Fatal("open db: ", err)
		}
		defer s.Close()
		h = hub.NewWithStore(s)
		log.Println("persistence: on, database file", *dbPath)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, h.Text(r.URL.Query().Get("room")))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	log.Println("listening on", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
