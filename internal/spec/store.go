package spec

import (
	"time"
)

// Keep nodes in the map for 5 days before expiry
const ExpiryTime = time.Duration(5 * 24 * time.Hour)

type Store interface {
	ChooseCoreNode() Address
	ChooseNetNode() Address
	UpdateCoreTime(address Address)
	UpdateNetTime(key PubKey)
	AddCoreNode(address Address, time int64, services uint64, hasNet bool)
	AddNetNode(pubkey PubKey, address Address, time int64, services uint64)
	NewNetNode(address Address, time int64)
	CoreStats() (mapSize int, newNodes int)
	NetStats() (mapSize int, newNodes int)
	NodeKey() (pub PubKey, priv PrivKey)
	TrimNodes()
	NodeList() (res NodeListRes)
	LoadFrom(filename string) error
	Persist(filename string) error
}
