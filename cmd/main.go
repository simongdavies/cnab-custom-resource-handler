package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/simongdavies/cnab-custom-resource-handler/pkg/handlers"
	log "github.com/sirupsen/logrus"
)

func main() {
	port, exists := os.LookupEnv("LISTENER_PORT")
	if !exists {
		port = "8080"
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Timeout(60 * time.Second))
	router.Use(middleware.Recoverer)
	router.Handle("/", handlers.NewCustomResourceHandler())
	log.Infof("Starting to listen on port  %s", port)
	err := http.ListenAndServe(fmt.Sprintf(":%s", port), router)
	if err != nil {
		log.Fatalf("Error running HTTP Server %v", err)
	}
}
