package protocol

import "time"

const (
	// DogeNet timestamps are 32-bit unsigned "Doge Epoch Time"
	// which is UNIX Epoch Time starting from Dec 6, 2013.
	// Range: 2013 - 2149

	// Subtract this from a UNIX Timestamp to get Doge Epoch Time.
	// Midnight, December 6, 2013 in UNIX Epoch Time.
	DogeEpoch = 1386288000

	// One Day in UNIX Epoch Time (and Doge Epoch)
	OneDay = 86400
)

func DogeNow() uint32 {
	return uint32(time.Now().Unix() - DogeEpoch)
}

func DogeToUnix(ts uint32) time.Time {
	return time.Unix(int64(ts)+DogeEpoch, 0)
}

func UnixToDoge(time time.Time) uint32 {
	return uint32(time.Unix() - DogeEpoch)
}
