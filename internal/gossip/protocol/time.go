package protocol

import "time"

// Doge Epoch Time is a 32-bit unsigned timestamp
// which is UNIX Epoch Time starting from Dec 6, 2013.
// Range: 2013 - 2149
type DogeTime uint32

func (ts DogeTime) ToUnix() time.Time {
	return time.Unix(int64(ts)+DogeEpoch, 0)
}

const (
	// Subtract this from a UNIX Timestamp to get Doge Epoch Time.
	// Midnight, December 6, 2013 in UNIX Epoch Time.
	DogeEpoch = 1386288000

	// One Day in UNIX Epoch Time (and Doge Epoch)
	OneDay = 86400
)

func DogeNow() DogeTime {
	return DogeTime(time.Now().Unix() - DogeEpoch)
}

func UnixToDoge(time time.Time) DogeTime {
	return DogeTime(time.Unix() - DogeEpoch)
}
