package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"voltafinbook/backend/internal/database"
	"voltafinbook/backend/internal/httpapi"
)

func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	config, configErr := database.LoadConfig()
	if configErr != nil {
		log.Print("MySQL database configuration unavailable; interface preview remains available")
	}
	server := &http.Server{
		Addr: addr, Handler: httpapi.NewHandler(database.NewChecker(config)),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-stop.Done()
		ctx, release := context.WithTimeout(context.Background(), 5*time.Second)
		defer release()
		if err := server.Shutdown(ctx); err != nil {
			log.Print("HTTP shutdown did not complete")
		}
	}()
	log.Printf("VOLTA FinBook foundation API listening at http://%s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
