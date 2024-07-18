package spec

type NodeListRes struct {
	Core []CoreNode `json:"core"`
	Net  []NetNode  `json:"net"`
}

type CoreNode struct {
	Address  string `json:"address"`
	Time     int64  `json:"time"`
	Services uint64 `json:"services"`
}

type NetNode struct {
	PubKey   string `json:"pubkey"`
	Address  string `json:"address"`
	Time     int64  `json:"time"`
	Services uint64 `json:"services"`
}
