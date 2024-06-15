package gossip

import (
	"log"
	"net"
	"sync"

	"github.com/dogeorg/dogenet/internal/governor"
)

type netService struct {
	governor.ServiceCtx
	address     string
	mutex       sync.Mutex
	listner     net.Listener
	connections map[net.Conn]struct{}
}

func New(address string) governor.Service {
	return &netService{
		address:     address,
		connections: make(map[net.Conn]struct{}),
	}
}

func (ns *netService) Run() {
	var lc net.ListenConfig
	listner, err := lc.Listen(ns.Context, "tcp", ns.address)
	if err != nil {
		log.Printf("[%s] cannot listen on `%v`: %v", ns.ServiceName, ns.address, err)
		return
	}
	log.Printf("[%s] listening on %v", ns.ServiceName, ns.address)
	ns.mutex.Lock()
	ns.listner = listner
	ns.mutex.Unlock()
	defer listner.Close()
	for {
		conn, err := listner.Accept()
		if err != nil {
			log.Printf("[%s] accept failed on `%v`: %v", ns.ServiceName, ns.address, err)
			return // typically due to Stop()
		}
		if ns.trackConn(conn, true) {
			go func(conn net.Conn) {
				defer func(conn net.Conn) { conn.Close(); ns.trackConn(conn, false) }(conn)
				ns.inboundConnection(conn)
			}(conn)
		} else { // Stop was called
			conn.Close()
			return
		}
	}
}

func (ns *netService) Stop() {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	// stop accepting connections
	if ns.listner != nil {
		ns.listner.Close()
	}
	// close all active connections
	for c := range ns.connections {
		c.Close()
	}
}

func (ns *netService) trackConn(c net.Conn, add bool) bool {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	if add {
		if ns.Stopping() {
			return false
		}
		ns.connections[c] = struct{}{}
	} else {
		delete(ns.connections, c)
	}
	return true
}

func (ns *netService) inboundConnection(conn net.Conn) {
	conn.Write([]byte("hello"))
}
