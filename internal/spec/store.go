package spec

import (
	"net"
	"time"
)

// Keep nodes in the map for 5 days before expiry
const ExpiryTime = time.Duration(5 * 24 * time.Hour)

type Store interface {
	ChooseCoreNode() string
	UpdateCoreTime(address string)
	AddCoreNode(address net.IP, port uint16, time uint32, services uint64)
	AddNetNode(address string)
	CoreStats() (mapSize int, newNodes int)
	NetStats() (mapSize int, newNodes int)
	TrimNodes()
	NodeList() (res []Payload)
}

type Payload struct {
	Address  string `json:"address"`
	Time     uint32 `json:"time"`
	Services uint64 `json:"services"`
}
