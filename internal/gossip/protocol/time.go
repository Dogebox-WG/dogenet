package protocol

import "time"

const (
	// Midnight, December 6, 2013 in UNIX Epoch Time
	// Subtract this from UNIX Timestamp to get Doge Epoch Time
	DogeEpoch = 1386288000
)

func DogeNow() uint32 {
	return uint32(time.Now().Unix() - DogeEpoch)
}

func DogeToTime(ts uint32) time.Time {
	return time.Unix(int64(ts)+DogeEpoch, 0)
}

func TimeToDoge(time time.Time) uint32 {
	return uint32(time.Unix() - DogeEpoch)
}
