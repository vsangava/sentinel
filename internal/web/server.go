package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/vsangava/distractions-free/internal/config"
)

//go:embed static/*
var webFiles embed.FS

func StartWebServer() {
	fsys, err := fs.Sub(webFiles, "static")
	if err != nil {
		log.Fatalf("Failed to load embedded web files: %v", err)
	}

	http.Handle("/", http.FileServer(http.FS(fsys)))

	http.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cfg := config.GetConfig()
		json.NewEncoder(w).Encode(cfg)
	})

	log.Println("Web server starting on http://localhost:8040")
	if err := http.ListenAndServe("127.0.0.1:8040", nil); err != nil {
		log.Fatalf("Web server failed: %v", err)
	}
}
