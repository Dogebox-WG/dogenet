module code.dogecoin.org/dogenet

require code.dogecoin.org/governor v1.0.2

require code.dogecoin.org/gossip v0.0.18

require github.com/mattn/go-sqlite3 v1.14.22

require (
	github.com/btcsuite/golangcrypto v0.0.0-20150304025918-53f62d9b43e8 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.3.0 // indirect
	github.com/dogeorg/doge v0.0.12 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/philpearl/intern v0.0.1 // indirect
	github.com/philpearl/stringbank v1.2.0 // indirect
)

// until radicle supports canonical tags
replace code.dogecoin.org/governor => github.com/dogeorg/governor v1.0.2

replace code.dogecoin.org/gossip => github.com/dogeorg/gossip v0.0.18

go 1.20
