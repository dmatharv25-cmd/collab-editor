package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/dmatharv25-cmd/collab-editor/server/hub"
	"github.com/dmatharv25-cmd/collab-editor/server/store"
)

func main() {
	dbPath := flag.String("db", "collab.db", "SQLite file for persistence (empty = memory only)")
	addr := flag.String("addr", ":8080", "listen address (the PORT environment variable overrides the port)")
	static := flag.String("static", "", "directory of built client files to serve (empty = serve none)")
	debug := flag.Bool("debug", false, "expose /text?room= (used by the load test; leave off when public)")
	flag.Parse()

	if p := os.Getenv("PORT"); p != "" {
		*addr = ":" + p
	}

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
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	if *debug {
		mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, h.Text(r.URL.Query().Get("room")))
		})
		log.Println("debug: /text enabled")
	}
	if *static != "" {
		mux.Handle("/", http.FileServer(http.Dir(*static)))
		log.Println("serving client files from", *static)
	}
	log.Println("listening on", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
