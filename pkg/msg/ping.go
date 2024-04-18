package msg

type PingMsg struct {
	Nonce uint64 // random nonce
}

func DecodePing(payload []byte) (msg PingMsg) {
	d := Decode(payload)
	msg.Nonce = d.uint64le()
	return
}

func EncodePing(msg PingMsg) []byte {
	e := Encode(86)
	e.uint64le(msg.Nonce)
	return e.buf
}
