package announce

import (
	"bytes"
	"encoding/hex"
	"log"
	"slices"
	"time"

	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/gossip/node"
	"code.dogecoin.org/governor"
)

// const AnnounceLongevity = 24 * time.Hour
const AnnounceLongevity = 5 * time.Minute
const QueueAnnouncement = 1 * time.Minute

type Announce struct {
	governor.ServiceCtx
	store        spec.Store
	cstore       spec.StoreCtx
	nodeKey      dnet.KeyPair          // node keypair for signing address messages
	changes      chan any              // input: changes to public address, owner pubkey, channels we have handlers for.
	receiver     spec.AnnounceReceiver // output: AnnounceReceiver receives new announcement RawMessages
	nextAnnounce node.AddressMsg       // current: public address, owner pubkey, channels, services
}

func New(public spec.Address, idenPub dnet.PubKey, store spec.Store, nodeKey dnet.KeyPair, receiver spec.AnnounceReceiver) *Announce {
	return &Announce{
		store:    store,
		nodeKey:  nodeKey,
		changes:  make(chan any, 10),
		receiver: receiver,
		nextAnnounce: node.AddressMsg{
			// Time: is dynamically updated
			Address: public.Host.To16(),
			Port:    public.Port,
			Owner:   idenPub,
			// Channels: are dynamically updated
			Services: []node.Service{
				// XXX this needs to be a config option (public Core Node address)
				{Tag: dnet.ServiceCore, Port: 22556},
			},
		},
	}
}

// goroutine
func (ns *Announce) Run() {
	ns.cstore = ns.store.WithCtx(ns.Context) // Service Context is first available here
	ns.updateAnnounce()
}

// goroutine
func (ns *Announce) updateAnnounce() {
	msg, remain := ns.loadOrGenerateAnnounce()
	ns.receiver.ReceiveAnnounce(msg)
	timer := time.NewTimer(remain)
	for !ns.Stopping() {
		select {
		case change := <-ns.changes:
			// whenever the node's address or channels change, gossip a new announcement.
			switch msg := change.(type) {
			case spec.ChangePublicAddress:
				ns.nextAnnounce.Address = msg.Addr.Host.To16()
				ns.nextAnnounce.Port = msg.Addr.Port
				log.Printf("[announce] received new public address: %v", msg.Addr)
			case spec.ChangeOwnerKey:
				ns.nextAnnounce.Owner = msg.Key
				log.Printf("[announce] received new owner key: %v", hex.EncodeToString(msg.Key))
			case spec.ChangeChannel:
				if msg.Remove {
					ns.nextAnnounce.Channels = slices.DeleteFunc(ns.nextAnnounce.Channels, func(e dnet.Tag4CC) bool {
						return e == msg.Tag
					})
					log.Printf("[announce] received remove channel: %v", msg.Tag)
				} else {
					if !slices.Contains(ns.nextAnnounce.Channels, msg.Tag) {
						ns.nextAnnounce.Channels = append(ns.nextAnnounce.Channels, msg.Tag)
					}
					log.Printf("[announce] received add channel: %v", msg.Tag)
				}
			default:
				log.Printf("[announce] invalid change message received")
				continue
			}
			// whenever a change is received, reset timer to 60 seconds.
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(QueueAnnouncement)

		case <-timer.C:
			// every 24 hours, re-sign and gossip the announcement.
			msg, remain := ns.generateAnnounce(ns.nextAnnounce)
			ns.receiver.ReceiveAnnounce(msg)
			log.Printf("[announce] sending announcement to all peers")
			// restart the timer
			timer.Reset(remain)

		case <-ns.Context.Done():
			timer.Stop()
			return
		}
	}
}

func (ns *Announce) loadOrGenerateAnnounce() (dnet.RawMessage, time.Duration) {
	// load the stored announcement from the database
	oldPayload, sig, expires, err := ns.cstore.GetAnnounce()
	now := time.Now().Unix()
	if err != nil {
		log.Printf("[announce] cannot load announcement: %v", err)
	} else if len(oldPayload) >= node.AddrMsgMinSize && len(sig) == 64 && now < expires {
		// determine if the announcement we stored is the same as the announcement
		// we would produce now; if so, avoid gossiping a new announcement
		oldMsg := node.DecodeAddrMsg(oldPayload) // for Time
		newMsg := ns.nextAnnounce                // copy
		newMsg.Time = oldMsg.Time                // ignore Time for Equals()
		if bytes.Equal(newMsg.Encode(), oldPayload) {
			// re-encode the stored announcement
			log.Printf("[announce] re-using stored announcement for %v seconds", expires-now)
			msg := dnet.ReEncodeMessage(node.ChannelNode, node.TagAddress, ns.nodeKey.Pub, sig, oldPayload)
			return msg, time.Duration(expires-now) * time.Second
		}
	}
	// create a new announcement and store it
	return ns.generateAnnounce(ns.nextAnnounce)
}

func (ns *Announce) generateAnnounce(newMsg node.AddressMsg) (dnet.RawMessage, time.Duration) {
	log.Printf("[announce] signing a new announcement")
	now := time.Now()
	newMsg.Time = dnet.UnixToDoge(now)
	payload := newMsg.Encode()
	msg := dnet.EncodeMessage(node.ChannelNode, node.TagAddress, ns.nodeKey, payload)
	view := dnet.MsgView(msg)
	err := ns.cstore.SetAnnounce(payload, view.Signature(), now.Add(AnnounceLongevity).Unix())
	if err != nil {
		log.Printf("[announce] cannot store announcement: %v", err)
	}
	return dnet.RawMessage{Header: view.Header(), Payload: payload}, AnnounceLongevity
}
