package netsvc

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"

	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/gossip/dnet"
)

type handlerConn struct {
	ns      *NetService
	conn    net.Conn
	channel uint32 // for atomic.Load
	receive map[dnet.Tag4CC]chan dnet.Message
	send    chan dnet.RawMessage
	name    string
}

func newHandler(conn net.Conn, ns *NetService) *handlerConn {
	hand := &handlerConn{
		ns:      ns,
		conn:    conn,
		receive: make(map[dnet.Tag4CC]chan dnet.Message),
		send:    make(chan dnet.RawMessage),
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
	// set our channel so forwardToHandlers will start sending us messages.
	atomic.StoreUint32(&hand.channel, uint32(bind.Chan))
	log.Printf("[%s] handler bound to channel: [%v]", hand.name, bind.Chan)
	// add the channel in the database (or update time)
	store := hand.ns.store
	err = store.AddChannel(bind.Chan)
	if err != nil {
		log.Println(err.Error())
		hand.ns.closeHandler(hand)
		return
	}
	// forward the owner pubkey to the announce service
	// only when received from the [Iden] pup
	if bind.Chan == dnet.ChannelIdentity {
		hand.ns.announceChanges <- spec.ChangeOwnerKey{Key: &bind.PubKey}
		hand.ns.announceChanges <- spec.ChangeChannel{Chan: bind.Chan}
	}
	// send bind message in reply, with this node's pubkey
	reply := dnet.BindMessage{Version: 1, Chan: bind.Chan, PubKey: *hand.ns.nodeKey.Pub}
	_, err = hand.conn.Write(reply.Encode())
	if err != nil {
		log.Printf("[%s] cannot send BindMessage: %v", hand.name, err)
		hand.ns.closeHandler(hand)
		return
	}
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
		log.Printf("[%s] received from handler: [%v][%v]", hand.name, msg.Chan, msg.Tag)
		hand.ns.forwardToPeers(dnet.RawMessage{Header: msg.RawHdr, Payload: msg.Payload})
	}
}

func (hand *handlerConn) readBindMessage(reader io.Reader) (bind dnet.BindMessage, err error) {
	buf := [dnet.BindMessageSize]byte{}
	_, err = io.ReadAtLeast(reader, buf[:], len(buf))
	if err != nil {
		return bind, fmt.Errorf("[%s] cannot read bind message from handler: %v", hand.name, err)
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
			log.Printf("[%s] sending to handler: [%v][%v]", hand.name, cha, tag)
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
