package spec

import (
	"context"
	"net"

	"code.dogecoin.org/gossip/dnet"
)

const SecondsPerDay = 24 * 60 * 60

// Keep core nodes for 3 midnights before expiry.
// Just before midnight -> 2 days.
// Just after midnight -> 3 days.
const MaxCoreNodeDays = 3

// Store is the top-level interface (e.g. SQLiteStore)
// It is bound to a cancellable Context.
type Store interface {
	WithCtx(ctx context.Context) Store
	// common
	NetStats() (mapSize int, err error)
	NodeList() (net []NetNode, err error)
	TrimNodes() (advanced bool, remNode int64, err error)
	// dogenet nodes
	GetAnnounce() (payload []byte, sig []byte, time int64, owner []byte, err error)
	SetAnnounce(payload []byte, sig []byte, time int64) error
	SetAnnounceOwner(owner []byte) error
	AddNetNode(key []byte, address Address, time int64, owner []byte, channels []dnet.Tag4CC, payload []byte, sig []byte) (changed bool, err error)
	UpdateNetTime(key []byte) error
	ChooseNetNode() (NodeInfo, error)
	ChooseNetNodeMsg() (NodeRecord, error)
	SampleNodesByChannel(channels []dnet.Tag4CC, exclude [][]byte) ([]NodeInfo, error)
	SampleNodesByIP(ipaddr net.IP, exclude [][]byte) ([]NodeInfo, error)
	// registered channels
	GetChannels() (channels []dnet.Tag4CC, err error)
	AddChannel(channel dnet.Tag4CC) error
}
