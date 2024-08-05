package spec

import (
	"context"
	"net"
	"time"

	"code.dogecoin.org/gossip/dnet"
)

// Keep nodes in the map for 5 days before expiry
const ExpiryTime = time.Duration(5 * 24 * time.Hour)

// Store is the top-level interface (e.g. SQLiteStore)
type Store interface {
	WithCtx(ctx context.Context) StoreCtx
}

// StoreCtx is a Store bound to a cancellable Context
type StoreCtx interface {
	CoreStats() (mapSize int, newNodes int)
	NetStats() (mapSize int, newNodes int)
	NodeList() (res NodeListRes)
	TrimNodes()
	// core nodes
	AddCoreNode(address Address, time int64, services uint64)
	UpdateCoreTime(address Address)
	ChooseCoreNode() Address
	SampleCoreNodes() []Address
	// dogenet nodes
	GetAnnounce() (payload []byte, sig []byte, time int64, err error)
	SetAnnounce(payload []byte, sig []byte, time int64) error
	AddNetNode(key PubKey, address Address, time int64, owner PubKey, channels []dnet.Tag4CC, payload []byte, sig []byte) (changed bool, err error)
	UpdateNetTime(key PubKey)
	ChooseNetNode() NodeInfo
	SampleNetNodes() []NodeInfo
	SampleNodesByChannel(channels []dnet.Tag4CC, exclude []PubKey) []NodeInfo
	SampleNodesByIP(ipaddr net.IP, exclude []PubKey) []NodeInfo
}
