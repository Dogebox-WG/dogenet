module code.dogecoin.org/dogenet

require code.dogecoin.org/governor v1.0.1

require code.dogecoin.org/gossip v0.0.3

require github.com/mattn/go-sqlite3 v1.14.22

// until radicle supports canonical tags
replace code.dogecoin.org/governor => github.com/dogeorg/governor v1.0.1

replace code.dogecoin.org/gossip => github.com/dogeorg/gossip v0.0.3

go 1.18
