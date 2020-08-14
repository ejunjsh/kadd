package controller

import (
	"context"
	remoteapi "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type Controller struct {
}

const PORT = "8787"
const steamTimeOut = 1000

func (c *Controller) Start() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/enter", c.serve)
	server := &http.Server{Addr: ":" + PORT, Handler: mux}

	go func() {
		log.Printf("Listening on %s \n", PORT)

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

func (c *Controller) serve(w http.ResponseWriter, req *http.Request) {
	streamOpts := &remotecommand.Options{
		Stdin:  true,
		Stdout: true,
		Stderr: false,
		TTY:    true,
	}
	remotecommand.ServeAttach(
		w,
		req,
		&attacher{},
		"",
		"",
		"",
		streamOpts,
		steamTimeOut,
		steamTimeOut,
		remoteapi.SupportedStreamingProtocols)
}
