package msg

// VersionMessage represents the structure of the version message
type VersionMessage struct {
	Version    int32
	Services   uint64
	Timestamp  int64
	RemoteAddr NetAddr // network address of the node receiving this message
	// version ≥ 106
	LocalAddr NetAddr // network address of the node emitting this message (now ignored)
	Agent     string
	Nonce     uint64 // randomly generated every time a version packet is sent
	Height    int32  // last block received by the emitting node
	// version ≥ 70001
	Relay bool
}

func DecodeVersion(payload []byte) (v VersionMessage) {
	// https://en.bitcoin.it/wiki/Protocol_documentation#version
	d := Decode(payload)
	v.Version = int32(d.uint32le())
	v.Services = d.uint64le()
	v.Timestamp = int64(d.uint64le())
	v.RemoteAddr = DecodeNetAddr(d, 0)
	if v.Version >= 106 {
		v.LocalAddr = DecodeNetAddr(d, 0)
		v.Agent = d.var_string()
		v.Nonce = d.uint64le()
		v.Height = int32(d.uint32le())
		if v.Version >= 70001 {
			v.Relay = d.bool()
		}
	}
	return
}

func EncodeVersion(version VersionMessage) []byte {
	e := Encode(86)
	e.uint32le(uint32(version.Version))
	e.uint64le(version.Services)
	e.uint64le(uint64(version.Timestamp))
	EncodeNetAddr(version.RemoteAddr, e, 0)
	if version.Version >= 106 {
		EncodeNetAddr(version.LocalAddr, e, 0)
		e.var_string(version.Agent)
		e.uint64le(version.Nonce)
		e.uint32le(uint32(version.Height))
		if version.Version >= 70001 {
			e.bool(version.Relay)
		}
	}
	return e.buf
}
