package dogenet

import (
	"context"
	"log"
	"path"

	"code.dogecoin.org/dogenet/internal/announce"
	"code.dogecoin.org/dogenet/internal/netsvc"
	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/dogenet/internal/store"
	"code.dogecoin.org/dogenet/internal/web"
	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/governor"
)

type DogeNetConfig struct {
	Dir          string
	DBFile       string
	Binds        []dnet.Address
	BindWeb      []dnet.Address
	HandlerBind  spec.BindTo
	NodeKey      dnet.KeyPair
	AllowLocal   bool
	Public       dnet.Address
	UseReflector bool
	Govenor      governor.Governor
}

func StartDogeNetService(s DogeNetConfig) error {
	// open the database.
	dbpath := path.Join(s.Dir, s.DBFile)
	db, err := store.NewSQLiteStore(dbpath, context.Background())
	if err != nil {
		log.Printf("Error opening database: %v [%s]\n", err, dbpath)
		return err
	}

	// start the gossip server
	changes := make(chan any, 10)
	netSvc := netsvc.New(s.Binds, s.HandlerBind, s.NodeKey, db, s.AllowLocal, changes)
	s.Govenor.Add("gossip", netSvc)

	// start the announcement service
	s.Govenor.Add("announce", announce.New(s.Public, s.NodeKey, db, netSvc, changes, s.UseReflector))

	// start the web server.
	for _, bind := range s.BindWeb {
		s.Govenor.Add("web-api", web.New(bind, db, netSvc))
	}

	// start the store trimmer
	s.Govenor.Add("store", store.NewStoreTrimmer(db))

	// run services until interrupted.
	s.Govenor.Start()

	return nil
}
