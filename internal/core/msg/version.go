package msg

import "code.dogecoin.org/gossip/codec"

// Services bit flags:
const (
	NodeNetwork        = 1    // This node can be asked for full blocks instead of just headers.
	NodeGetUTXO        = 2    // See BIP 0064
	NodeBloom          = 4    // See BIP 0111
	NodeWitness        = 8    // See BIP 0144
	NodeCompactFilters = 64   // See BIP 0157
	NodeNetworkLimited = 1024 // See BIP 0159
)

// VersionMsg represents the structure of the version message
type VersionMsg struct {
	Version    int32   // PROTOCOL_VERSION
	Services   uint64  // Services bit flags
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
	d := codec.Decode(payload)
	v.Version = int32(d.UInt32le())
	if v.Version == 10300 {
		// a fixup found in dogecoin-seeder
		v.Version = 300
	}
	v.Services = d.UInt64le()
	v.Timestamp = int64(d.UInt64le())
	v.RemoteAddr = DecodeNetAddr(d, 0)
	if v.Version >= 106 {
		v.LocalAddr = DecodeNetAddr(d, 0)
		v.Nonce = d.UInt64le()
		v.Agent = d.VarString()
		if v.Version >= 209 {
			v.Height = int32(d.UInt32le())
			// some peers send version >= 70001 but don't send Relay.
			if v.Version >= 70001 && d.Has(1) {
				v.Relay = d.Bool()
			}
		}
	}
	return
}

func EncodeVersion(version VersionMsg) []byte {
	e := codec.Encode(86)
	e.UInt32le(uint32(version.Version))
	e.UInt64le(uint64(version.Services))
	e.UInt64le(uint64(version.Timestamp))
	EncodeNetAddr(version.RemoteAddr, e, 0)
	if version.Version >= 106 {
		EncodeNetAddr(version.LocalAddr, e, 0)
		e.UInt64le(version.Nonce)
		e.VarString(version.Agent)
		e.UInt32le(uint32(version.Height))
		if version.Version >= 70001 {
			e.Bool(version.Relay)
		}
	}
	return e.Result()
}
