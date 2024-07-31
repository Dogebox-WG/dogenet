module code.dogecoin.org/dogenet

require code.dogecoin.org/governor v0.0.1

require code.dogecoin.org/gossip v0.0.2

require github.com/mattn/go-sqlite3 v1.14.22 // indirect

// until radicle supports canonical tags
replace code.dogecoin.org/governor => ../governor

replace code.dogecoin.org/gossip => ../gossip

go 1.18
