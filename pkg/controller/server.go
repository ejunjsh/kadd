package controller

import (
	"context"
	"encoding/json"
	"fmt"
	remoteapi "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
	"log"
	"net/http"
	"net/url"
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
	mux.HandleFunc("/api/v1/create/", serve)
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
	fmt.Println(req.URL.Path)
	sa := strings.Split(req.URL.Path, "/")

	image, _ := url.QueryUnescape(sa[4])

	fmt.Println(image)

	containerUri, _ := url.QueryUnescape(sa[5])

	fmt.Println(containerUri)

	containerUriParts := strings.SplitN(containerUri, "://", 2)
	if len(containerUriParts) != 2 {
		http.Error(w, "target container id must have form scheme:id but was "+containerUri, 400)
	}

	targetId := containerUriParts[1]

	cmd, _ := url.QueryUnescape(sa[6])

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
