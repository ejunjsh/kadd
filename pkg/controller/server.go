package controller

import (
	"context"
	"encoding/json"
	remoteapi "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
)

const PORT = "8787"

const RuntimeTimeout = 30 * time.Second
const StreamIdleTimeout = 10 * time.Minute
const StreamCreationTimeout = 15 * time.Second

func Start() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/enter", serve)
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("I'm fine!"))
	})
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

func serve(w http.ResponseWriter, req *http.Request) {
	streamOpts := &remotecommand.Options{
		Stdin:  true,
		Stdout: true,
		Stderr: false,
		TTY:    true,
	}

	image := req.FormValue("image")
	if len(image) < 1 {
		http.Error(w, "image must be provided", 400)
		return
	}

	containerUri := req.FormValue("containerUri")
	if len(containerUri) < 1 {
		http.Error(w, "containerUri must be provided", 400)
		return
	}

	containerUriParts := strings.SplitN(containerUri, "://", 2)
	if len(containerUriParts) != 2 {
		http.Error(w, "target container id must have form scheme:id but was "+containerUri, 400)
	}

	targetId := containerUriParts[1]

	cmd := req.FormValue("cmd")
	var commandSlice []string
	err := json.Unmarshal([]byte(cmd), &commandSlice)
	if err != nil || len(commandSlice) < 1 {
		http.Error(w, "cannot parse command", 400)
		return
	}

	context, cancel := context.WithCancel(req.Context())
	defer cancel()

	remotecommand.ServeAttach(
		w,
		req,
		&attacher{context: context, targetId: targetId, cmd: commandSlice, image: image},
		"",
		"",
		"",
		streamOpts,
		StreamIdleTimeout,
		StreamCreationTimeout,
		remoteapi.SupportedStreamingProtocols)
}
