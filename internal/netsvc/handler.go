package netsvc

import (
	"bufio"
	"log"
	"net"

	"rad/gossip/dnet"

	"github.com/dogeorg/dogenet/internal/spec"
)

type handlerConn struct {
	ns      *netService
	conn    net.Conn
	channel dnet.Tag4CC
	store   spec.Store
	receive map[dnet.Tag4CC]chan dnet.Message
	send    chan dnet.Message
	name    string
}

func newHandler(conn net.Conn, ns *netService) *handlerConn {
	hand := &handlerConn{
		ns:      ns,
		conn:    conn,
		store:   ns.store,
		receive: make(map[dnet.Tag4CC]chan dnet.Message),
		send:    make(chan dnet.Message),
		name:    "protocol-handler",
	}
	return hand
}

func (hand *handlerConn) start() {
	go hand.receiveFromHandler()
	go hand.sendToHandler()
}

// goroutine
func (hand *handlerConn) receiveFromHandler() {
	reader := bufio.NewReader(hand.conn)
	for !hand.ns.Stopping() {
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] cannot receive from handler: %v", hand.name, err)
			hand.ns.closeHandler(hand)
			return
		}
		hand.ns.forwardToPeers(msg)
	}
}

// goroutine
func (hand *handlerConn) sendToHandler() {
	conn := hand.conn
	send := hand.send
	for !hand.ns.Stopping() {
		select {
		case msg := <-send:
			// forward the message to the peer
			err := dnet.ForwardMessage(conn, msg)
			if err != nil {
				log.Printf("[%s] cannot send to handler: %v", hand.name, err)
				hand.ns.closeHandler(hand)
				return
			}
		case <-hand.ns.Context.Done():
			// shutting down
			hand.ns.closeHandler(hand)
			return
		}
	}
}
