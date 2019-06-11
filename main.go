package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twooster/glock/app"
)

var tableName = "Glock"

func main() {
	db := app.BuildDynamodbClient()
	backend := app.DynamoBackend{
		Db:    db,
		Table: tableName,
	}
	server := app.NewServer(&backend)

	srv := &http.Server{
		Addr:    ":12345",
		Handler: server,
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		fmt.Println("Starting HTTP server")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("Error starting server: %v\n", err)
		}
		cancel()
	}()

	go func() {
		stopChannel := make(chan os.Signal, 1)
		signal.Notify(stopChannel, syscall.SIGTERM, syscall.SIGINT)
		s := <-stopChannel
		fmt.Printf("Received signal '%v', shutting down...\n", s)
		cancel()
	}()

	<-ctx.Done()
	// We received an interrupt signal, shut down.
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("Error shutting down server: %v\n", err)
	}
}
