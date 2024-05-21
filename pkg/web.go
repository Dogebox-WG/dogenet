package dogenet

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
)

type WebAPI struct {
	srv http.Server
}

func (w *WebAPI) Stop() {
	w.srv.Shutdown(context.Background())
}

func NewWebAPI(wg *sync.WaitGroup, bind string, port int) Service {
	mux := http.NewServeMux()
	mux.HandleFunc("/nodes", getNodes)
	w := &WebAPI{
		srv: http.Server{
			Addr:    net.JoinHostPort(bind, strconv.Itoa(port)),
			Handler: mux,
		},
	}
	wg.Add(1)
	go w.Run(wg)
	return w
}

func (w *WebAPI) Run(wg *sync.WaitGroup) {
	log.Printf("HTTP server listening on: %v\n", w.srv.Addr)
	if err := w.srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("HTTP server: %v\n", err)
	}
	log.Printf("HTTP server stopped.\n")
	wg.Done()
}

func getNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		bytes, err := json.Marshal(Map.Payload())
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
