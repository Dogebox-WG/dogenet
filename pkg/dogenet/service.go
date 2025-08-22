package dogenet

import (
	"log"

	"code.dogecoin.org/dogenet/internal/announce"
	"code.dogecoin.org/dogenet/internal/netsvc"
	"code.dogecoin.org/dogenet/internal/store"
	"code.dogecoin.org/dogenet/internal/web"
	"code.dogecoin.org/dogenet/pkg/spec"
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
}

func DogeNet(gov governor.Governor, cfg DogeNetConfig) error {
	// open the database.
	db, err := store.NewSQLiteStore(cfg.DBFile, gov.GlobalContext())
	if err != nil {
		log.Printf("Error opening database: %v [%s]\n", err, cfg.DBFile)
		return err
	}

	// start the gossip server
	changes := make(chan any, 10)
	netSvc := netsvc.New(cfg.Binds, cfg.HandlerBind, cfg.NodeKey, db, cfg.AllowLocal, changes)
	gov.Add("gossip", netSvc)

	// start the announcement service
	gov.Add("announce", announce.New(cfg.Public, cfg.NodeKey, db, netSvc, changes, cfg.UseReflector))

	// start the web server.
	for _, bind := range cfg.BindWeb {
		gov.Add("web-api", web.New(bind, db, netSvc))
	}

	// start the store trimmer
	gov.Add("store", store.NewStoreTrimmer(db))

	return nil
}
