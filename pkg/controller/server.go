package controller

import (
	"context"
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
	mux.HandleFunc("/api/v1/create", serve)
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

	image := "nginx"

	containerUri := "docker://734bd2abacc2c4303e28346760383baf18176cb9c65a2c849a68584b4a670ece"

	containerUriParts := strings.SplitN(containerUri, "://", 2)
	if len(containerUriParts) != 2 {
		http.Error(w, "target container id must have form scheme:id but was "+containerUri, 400)
	}

	targetId := containerUriParts[1]

	var cccc [1]string
	cccc[0] = "bash"

	context, cancel := context.WithCancel(req.Context())
	defer cancel()

	remotecommand.ServeAttach(
		w,
		req,
		&attacher{context: context, targetId: targetId, cmd: cccc[:], image: image},
		"",
		"",
		"",
		streamOpts,
		StreamIdleTimeout,
		StreamCreationTimeout,
		remoteapi.SupportedStreamingProtocols)
}
