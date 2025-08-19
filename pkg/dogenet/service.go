package dogenet

import (
	"context"
	"log"
	"path"
	"time"

	"code.dogecoin.org/dogenet/internal/announce"
	"code.dogecoin.org/dogenet/internal/netsvc"
	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/dogenet/internal/store"
	"code.dogecoin.org/dogenet/internal/web"
	"code.dogecoin.org/gossip/dnet"
	"code.dogecoin.org/governor"
)

type DogeNetService struct {
	Dir          string
	DBFile       string
	Binds        []dnet.Address
	BindWeb      []dnet.Address
	HandlerBind  spec.BindTo
	NodeKey      dnet.KeyPair
	AllowLocal   bool
	Public       dnet.Address
	UseReflector bool
	gov          governor.Governor
}

func (s *DogeNetService) Start() error {
	// open the database.
	dbpath := path.Join(s.Dir, s.DBFile)
	db, err := store.NewSQLiteStore(dbpath, context.Background())
	if err != nil {
		log.Printf("Error opening database: %v [%s]\n", err, dbpath)
		return err
	}

	gov := governor.New().CatchSignals().Restart(1 * time.Second)
	s.gov = gov

	// start the gossip server
	changes := make(chan any, 10)
	netSvc := netsvc.New(s.Binds, s.HandlerBind, s.NodeKey, db, s.AllowLocal, changes)
	gov.Add("gossip", netSvc)

	// start the announcement service
	gov.Add("announce", announce.New(s.Public, s.NodeKey, db, netSvc, changes, s.UseReflector))

	// start the web server.
	for _, bind := range s.BindWeb {
		gov.Add("web-api", web.New(bind, db, netSvc))
	}

	// start the store trimmer
	gov.Add("store", store.NewStoreTrimmer(db))

	// run services until interrupted.
	gov.Start()
	gov.WaitForShutdown()

	return nil
}

func (s *DogeNetService) Stop() error {
	s.gov.Shutdown()
	return nil
}
