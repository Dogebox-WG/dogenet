package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/dogeorg/dogenet/internal/dogeicon"
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
	mux.HandleFunc("/compress", a.compress)

	fs := http.FileServer(http.Dir("./web"))
	mux.Handle("/web/", http.StripPrefix("/web/", fs))

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
		bytes, err := json.Marshal(a.store.NodeList())
		if err != nil {
			http.Error(w, fmt.Sprintf("error encoding JSON: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
		w.Header().Set("Allow", "GET, OPTIONS")
		w.Write(bytes)
	} else {
		options(w, r, "GET, OPTIONS")
	}
}

// func (a *WebAPI) uploadPage(w http.ResponseWriter, r *http.Request) {
// 	if r.Method == http.MethodGet {
// 		r, err := os.Open("web/upload.html")
// 		if err != nil {
// 			http.Error(w, "not found", http.StatusNotFound)
// 			return
// 		}
// 		defer r.Close()
// 		bytes, err := io.ReadAll(r)
// 		if err != nil {
// 			http.Error(w, "cannot read", http.StatusNotFound)
// 			return
// 		}
// 		w.Header().Set("Content-Type", "text/html")
// 		w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
// 		w.Header().Set("Allow", "GET, OPTIONS")
// 		w.Write(bytes)
// 	} else {
// 		options(w, r, "GET, OPTIONS")
// 	}
// }

const imgSizeRGB = 48 * 48 * 3
const imgSizeRGBA = 48 * 48 * 4

func (a *WebAPI) compress(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
			return
		}

		stride := 3
		if len(body) == imgSizeRGBA {
			stride = 4
		} else if len(body) != imgSizeRGB {
			http.Error(w, fmt.Sprintf("bad request: size is %d, expecting %d or %d", len(body), imgSizeRGB, imgSizeRGBA), http.StatusBadRequest)
			return
		}

		_, res := dogeicon.Compress(body, stride)

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(res)))
		w.Header().Set("Allow", "POST, OPTIONS")
		w.Write(res[:])
	} else {
		options(w, r, "POST, OPTIONS")
	}
}

func options(w http.ResponseWriter, r *http.Request, options string) {
	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("Allow", options)
		w.WriteHeader(http.StatusNoContent)

	default:
		w.Header().Set("Allow", options)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
