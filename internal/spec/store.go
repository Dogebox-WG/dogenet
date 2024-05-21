package spec

import (
	"net"
	"time"
)

// Keep nodes in the map for 5 days before expiry
const ExpiryTime = time.Duration(5 * 24 * time.Hour)

type Store interface {
	ChooseNode() string
	UpdateTime(address string)
	AddNode(address net.IP, port uint16, time uint32, services uint64)
	Stats() (mapSize int, newNodes int)
	Trim()
	Payload() (res []Payload)
}

type Payload struct {
	Address  string `json:"address"`
	Time     uint32 `json:"time"`
	Services uint64 `json:"services"`
}
