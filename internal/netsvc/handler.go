package netsvc

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"

	"code.dogecoin.org/gossip/dnet"
)

type handlerConn struct {
	ns      *NetService
	conn    net.Conn
	channel uint32 // for atomic.Load
	receive map[dnet.Tag4CC]chan dnet.Message
	send    chan RawMessage
	name    string
}

func newHandler(conn net.Conn, ns *NetService) *handlerConn {
	hand := &handlerConn{
		ns:      ns,
		conn:    conn,
		receive: make(map[dnet.Tag4CC]chan dnet.Message),
		send:    make(chan RawMessage),
		name:    "protocol-handler",
	}
	return hand
}

func (hand *handlerConn) start() {
	go hand.receiveFromHandler()
}

// goroutine
func (hand *handlerConn) receiveFromHandler() {
	// expect a BindMessage from the handler
	reader := bufio.NewReader(hand.conn)
	bind, err := hand.readBindMessage(reader)
	if err != nil {
		log.Println(err.Error())
		hand.ns.closeHandler(hand)
		return
	}
	atomic.StoreUint32(&hand.channel, uint32(bind.Chan))
	log.Printf("[%s] handler bound to channel: [%v]", hand.name, bind.Chan)
	// start forwarding messages to the handler
	go hand.sendToHandler()
	// start receiving messages from the handler
	for !hand.ns.Stopping() {
		msg, err := dnet.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] cannot receive from handler: %v", hand.name, err)
			hand.ns.closeHandler(hand)
			return
		}
		// forward the message to all peers (ignore channel here)
		log.Printf("[%s] received from handler: %v %v", hand.name, msg.Chan, msg.Tag)
		hand.ns.forwardToPeers(RawMessage{Header: msg.RawHdr, Payload: msg.Payload})
	}
}

func (hand *handlerConn) readBindMessage(reader io.Reader) (bind dnet.BindMessage, err error) {
	buf := [8]byte{}
	n, err := io.ReadAtLeast(reader, buf[:], 8)
	if err != nil {
		return bind, fmt.Errorf("[%s] cannot read bind message from handler: %v", hand.name, err)
	}
	if n != 8 {
		return bind, fmt.Errorf("[%s] short bind message: %v vs %v", hand.name, n, 8)
	}
	bind, ok := dnet.DecodeBindMessage(buf[:])
	if !ok {
		return bind, fmt.Errorf("[%s] invalid bind message (wrong size)", hand.name)
	}
	return bind, nil
}

// goroutine
func (hand *handlerConn) sendToHandler() {
	conn := hand.conn
	send := hand.send
	for !hand.ns.Stopping() {
		select {
		case raw := <-send:
			// forward the raw message to the handler
			cha, tag := dnet.MsgView(raw.Header).ChanTag()
			log.Printf("[%s] sending to handler: %v %v", hand.name, cha, tag)
			_, err := conn.Write(raw.Header)
			if err != nil {
				log.Printf("[%s] cannot send to handler: %v", hand.name, err)
				hand.ns.closeHandler(hand)
				return
			}
			_, err = conn.Write(raw.Payload)
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
