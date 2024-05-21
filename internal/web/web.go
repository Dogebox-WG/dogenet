package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/dogeorg/dogenet/internal/governor"
	"github.com/dogeorg/dogenet/internal/spec"
)

func New(store spec.Store, bind string, port int) governor.Service {
	mux := http.NewServeMux()
	a := &WebAPI{
		store: store,
		srv: http.Server{
			Addr:    net.JoinHostPort(bind, strconv.Itoa(port)),
			Handler: mux,
		},
	}
	mux.HandleFunc("/nodes", a.getNodes)
	return a
}

type WebAPI struct {
	governor.ServiceCtx
	store spec.Store
	srv   http.Server
}

func (a *WebAPI) Stop() {
	// new goroutine because Shutdown() blocks
	go func() {
		// cannot use ServiceCtx here because it's already cancelled
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		a.srv.Shutdown(ctx) // blocking call
		cancel()
	}()
}

func (a *WebAPI) Run() {
	log.Printf("HTTP server listening on: %v\n", a.srv.Addr)
	if err := a.srv.ListenAndServe(); err != http.ErrServerClosed { // blocking call
		log.Printf("HTTP server: %v\n", err)
	}
}

func (a *WebAPI) getNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		bytes, err := json.Marshal(a.store.Payload())
		if err != nil {
			http.Error(w, fmt.Sprintf("error encoding JSON: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
		w.Header().Set("Allow", "GET, OPTIONS")
		w.Write(bytes)
	} else {
		optionsGET(w, r)
	}
}

func optionsGET(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("Allow", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", "GET, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
