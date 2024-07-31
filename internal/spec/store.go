package spec

import (
	"net"
	"time"

	"code.dogecoin.org/gossip/dnet"
)

// Keep nodes in the map for 5 days before expiry
const ExpiryTime = time.Duration(5 * 24 * time.Hour)

// type Store interface {
// 	ChooseCoreNode() Address
// 	ChooseNetNode() Address
// 	UpdateCoreTime(address Address)
// 	UpdateNetTime(key PubKey)
// 	AddCoreNode(address Address, time int64, services uint64, hasNet bool)
// 	AddNetNode(pubkey PubKey, address Address, time int64, services uint64)
// 	CoreStats() (mapSize int, newNodes int)
// 	NetStats() (mapSize int, newNodes int)
// 	NodeKey() (pub PubKey, priv PrivKey)
// 	TrimNodes()
// 	NodeList() (res NodeListRes)
// 	LoadFrom(filename string) error
// 	Persist(filename string) error
// }

type Store interface {
	NodeKey() (pub PubKey, priv PrivKey)
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
	AddNetNode(pubkey PubKey, address Address, time int64, channels []dnet.Tag4CC, msg []byte)
	UpdateNetTime(key PubKey)
	ChooseNetNode() NodeInfo
	SampleNetNodes() []NodeInfo
	SampleNodesByChannel(channels []dnet.Tag4CC, exclude []PubKey) []NodeInfo
	SampleNodesByIP(ipaddr net.IP, exclude []PubKey) []NodeInfo
}

type Service struct {
	Tag  dnet.Tag4CC // [4] Service Tag (Big-Endian)
	Port uint16      // [2] TCP Port number (Big-Endian)
	Data string      // [1+] Service Data (optional)
}

type NodeInfo struct {
	PubKey []byte // 32 bytes
	Addr   Address
}
