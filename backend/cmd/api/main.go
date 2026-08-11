package main

import (
	"log"
	"net/http"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/config"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	router := httpserver.NewRouter()
	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
