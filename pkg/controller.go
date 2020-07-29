package pkg

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type Controller struct {

}

func (c *Controller) Start() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/", ...)
	server := &http.Server{Addr: ..., Handler: mux}

	go func() {
		log.Printf("Listening on %s \n", ...)

		if err := server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	<-stop

	log.Println("shutting done server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)

	return nil
}
