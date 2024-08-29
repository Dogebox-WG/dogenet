module code.dogecoin.org/dogenet

require code.dogecoin.org/governor v1.0.2

require code.dogecoin.org/gossip v0.0.7

require github.com/mattn/go-sqlite3 v1.14.22

// until radicle supports canonical tags
replace code.dogecoin.org/governor => github.com/dogeorg/governor v1.0.2

replace code.dogecoin.org/gossip => github.com/dogeorg/gossip v0.0.7

go 1.20
