package gossip

import (
	"bufio"
	"log"
	"net"

	"github.com/dogeorg/dogenet/internal/gossip/protocol"
	"github.com/dogeorg/dogenet/internal/spec"
)

// Peer connections exchange messages with a remote peer;
// forward received messages to channel owners,
// and periodically send gossip from channel owners.

type peerConn struct {
	ns       *netService
	address  spec.Address
	store    spec.Store
	receive  map[protocol.Tag4CC]chan protocol.Message
	send     chan []byte
	signKey  protocol.PrivKey
	identPub protocol.PubKey
}

func newPeer(conn net.Conn, address spec.Address, ns *netService, priv protocol.PrivKey, pub protocol.PubKey) *peerConn {
	peer := &peerConn{
		ns:       ns,
		address:  address,
		store:    ns.store,
		receive:  make(map[protocol.Tag4CC]chan protocol.Message),
		send:     make(chan []byte),
		signKey:  priv,
		identPub: pub,
	}
	go peer.receiveFromPeer(conn)
	go peer.sendToPeer(conn)
	return peer
}

// goroutine
func (peer *peerConn) receiveFromPeer(conn net.Conn) {
	who := peer.address.String()
	reader := bufio.NewReader(conn)
	peer.sendMyAddress(conn)
	for {
		msg, err := protocol.ReadMessage(reader)
		if err != nil {
			log.Printf("[%s] bad message: %v", who, err)
			peer.ns.closeConn(conn)
			return
		}
		// forward received message to the channel owner
		if channel, ok := peer.receive[msg.Chan]; ok {
			channel <- msg
		} else {
			// send a reject on the channel, with message tag as data
			_, err := conn.Write(
				protocol.EncodeMessage(msg.Chan, protocol.TagReject, peer.signKey,
					protocol.EncodeReject(protocol.REJECT_CHAN, "", msg.Tag.Bytes())))
			if err != nil {
				log.Printf("[%s] failed to send reject: %s", who, msg.Tag)
				peer.ns.closeConn(conn)
				return
			}
		}
	}
}

// goroutine
func (peer *peerConn) sendToPeer(conn net.Conn) {
	who := peer.address.String()
	for {
		select {
		case msg := <-peer.send:
			// forward the message to the peer
			_, err := conn.Write(msg)
			if err != nil {
				log.Printf("[%s] cannot send: %v", who, err)
				peer.ns.closeConn(conn)
				return
			}
		case <-peer.ns.Context.Done():
			// shutting down
			peer.ns.closeConn(conn)
			return
		}
	}
}

func (peer *peerConn) sendMyAddress(conn net.Conn) {
	addr := protocol.AddressMsg{
		Time:    protocol.DogeNow(),
		Address: peer.address.Host.To16(), // can return nil
		Port:    peer.address.Port,
		Channels: []protocol.Tag4CC{
			protocol.ChannelIdentity,
			protocol.ChannelB0rk,
		},
		Services: []protocol.Service{
			{Tag: protocol.ServiceCore, Port: 22556, Data: ""},
		},
	}
	addr.Owner = peer.identPub // shared!
	conn.Write(protocol.EncodeMessage(protocol.ChannelDoge, protocol.TagAddress, peer.signKey, addr.Encode()))
}
