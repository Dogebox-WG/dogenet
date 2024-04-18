package msg

type LocalNodeServices uint64

// LocalNodeServices bit flags:
const (
	NodeNetwork        LocalNodeServices = 1    // This node can be asked for full blocks instead of just headers.
	NodeGetUTXO        LocalNodeServices = 2    // See BIP 0064
	NodeBloom          LocalNodeServices = 4    // See BIP 0111
	NodeWitness        LocalNodeServices = 8    // See BIP 0144
	NodeCompactFilters LocalNodeServices = 64   // See BIP 0157
	NodeNetworkLimited LocalNodeServices = 1024 // See BIP 0159
)

// VersionMsg represents the structure of the version message
type VersionMsg struct {
	Version    int32 // PROTOCOL_VERSION
	Services   LocalNodeServices
	Timestamp  int64   // nTime: UNIX time in seconds
	RemoteAddr NetAddr // addrYou: network address of the node receiving this message
	// version ≥ 106
	LocalAddr NetAddr // addrMe: network address of the node emitting this message (now ignored)
	Nonce     uint64  // nonce: randomly generated every time a version packet is sent
	Agent     string  // strSubVersion:
	Height    int32   // 32 nNodeStartingHeight
	// version ≥ 70001
	Relay bool // fRelayTxs
}

func DecodeVersion(payload []byte) (v VersionMsg) {
	d := Decode(payload)
	v.Version = int32(d.uint32le())
	v.Services = LocalNodeServices(d.uint64le())
	v.Timestamp = int64(d.uint64le())
	v.RemoteAddr = DecodeNetAddr(d, 0)
	if v.Version >= 106 {
		v.LocalAddr = DecodeNetAddr(d, 0)
		v.Nonce = d.uint64le()
		v.Agent = d.var_string()
		v.Height = int32(d.uint32le())
		if v.Version >= 70001 {
			v.Relay = d.bool()
		}
	}
	return
}

func EncodeVersion(version VersionMsg) []byte {
	e := Encode(86)
	e.uint32le(uint32(version.Version))
	e.uint64le(uint64(version.Services))
	e.uint64le(uint64(version.Timestamp))
	EncodeNetAddr(version.RemoteAddr, e, 0)
	if version.Version >= 106 {
		EncodeNetAddr(version.LocalAddr, e, 0)
		e.uint64le(version.Nonce)
		e.var_string(version.Agent)
		e.uint32le(uint32(version.Height))
		if version.Version >= 70001 {
			e.bool(version.Relay)
		}
	}
	return e.buf
}
