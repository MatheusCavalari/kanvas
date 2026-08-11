package main

import (
	"log"
	"net/http"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
)

func main() {
	router := httpserver.NewRouter()
	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
