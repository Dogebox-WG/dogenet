package web

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"code.dogecoin.org/dogenet/pkg/spec"
	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/governor"
)

func New(bind spec.Address, store spec.Store, netSvc spec.NetSvc) governor.Service {
	mux := http.NewServeMux()
	a := &WebAPI{
		_store: store,
		srv: http.Server{
			Addr:    bind.String(),
			Handler: mux,
		},
		netSvc: netSvc,
	}

	mux.HandleFunc("/nodes", a.getNodes)
	mux.HandleFunc("/addpeer", a.addpeer)

	return a
}

type WebAPI struct {
	governor.ServiceCtx
	_store spec.Store
	store  spec.Store
	srv    http.Server
	netSvc spec.NetSvc
}

// called on any
func (a *WebAPI) Stop() {
	// new goroutine because Shutdown() blocks
	go func() {
		// cannot use ServiceCtx here because it's already cancelled
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		a.srv.Shutdown(ctx) // blocking call
		cancel()
	}()
}

// goroutine
func (a *WebAPI) Run() {
	a.store = a._store.WithCtx(a.Context) // Service Context is first available here
	log.Printf("HTTP server listening on: %v\n", a.srv.Addr)
	if err := a.srv.ListenAndServe(); err != http.ErrServerClosed { // blocking call
		log.Printf("HTTP server: %v\n", err)
	}
}

func (a *WebAPI) getNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		nodes, err := a.store.NodeList()
		if err != nil {
			http.Error(w, fmt.Sprintf("error in query: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		if nodes == nil {
			// Go incorrectly encodes this as `null`
			nodes = make([]spec.NetNode, 0)
		}
		bytes, err := json.Marshal(nodes)
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

type AddPeer struct {
	Key  string `json:"key"`
	Addr string `json:"addr"`
}

func (a *WebAPI) addpeer(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// request
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
			return
		}
		var to AddPeer
		err = json.Unmarshal(body, &to)
		if err != nil {
			http.Error(w, fmt.Sprintf("error decoding JSON: %s", err.Error()), http.StatusBadRequest)
			return
		}

		// add peer
		pub, err := hex.DecodeString(to.Key)
		if err != nil || len(pub) != 32 {
			http.Error(w, fmt.Sprintf("invalid key: %v", err), http.StatusBadRequest)
			return
		}
		addr, err := dnet.ParseAddress(to.Addr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid peer address: %v", err), http.StatusBadRequest)
			return
		}
		// attempt to connect to the peer (soonish)
		a.netSvc.AddPeer(spec.NodeInfo{PubKey: ([32]byte)(pub), Addr: addr})
		log.Printf("added peer: %v %v %v", hex.EncodeToString(pub), addr.Host, addr.Port)

		// response
		res, err := json.Marshal("OK")
		if err != nil {
			http.Error(w, fmt.Sprintf("error encoding JSON: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
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
