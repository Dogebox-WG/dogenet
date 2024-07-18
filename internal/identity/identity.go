package identity

// Identity Cache and Protocol-Handler

// Identities are broadcast on the "Iden" channel.
// An identity stays active for 30 days after signing.
// Tracks all currently active identities on the network.
// Allows Identities to be pinned.
// Prepares a set of identities to gossip to peers.

// type IdentityService struct {
// 	governor.ServiceCtx
// 	receive     chan protocol.Message
// 	address     spec.Address
// 	mutex       sync.Mutex
// 	listner     net.Listener
// 	channels    map[protocol.Tag4CC]chan protocol.Message
// 	store       spec.Store
// 	remotePort  uint16
// 	connections map[net.Conn]spec.Address
// }

// func New(address spec.Address, remotePort uint16, store spec.Store) governor.Service {
// 	return &IdentityService{
// 		address:     address,
// 		channels:    make(map[protocol.Tag4CC]chan protocol.Message),
// 		store:       store,
// 		remotePort:  remotePort,
// 		connections: make(map[net.Conn]spec.Address),
// 	}
// }

// func (s *IdentityService) Run() {
// 	for !s.Stopping() {

// 	}
// 	var lc net.ListenConfig
// 	who := s.address.String()
// 	listner, err := lc.Listen(s.Context, "tcp", s.address.String())
// 	if err != nil {
// 		log.Printf("[%s] cannot listen on `%v`: %v", s.ServiceName, who, err)
// 		return
// 	}
// 	log.Printf("[%s] listening on %v", s.ServiceName, who)
// 	s.mutex.Lock()
// 	s.listner = listner
// 	s.mutex.Unlock()
// 	defer listner.Close()
// 	for {
// 		conn, err := listner.Accept()
// 		if err != nil {
// 			log.Printf("[%s] accept failed on `%v`: %v", s.ServiceName, who, err)
// 			return // typically due to Stop()
// 		}
// 		remote, err := spec.ParseAddress(conn.RemoteAddr().String())
// 		if err != nil {
// 			log.Printf("[%s] no remote address for inbound peer: %v", who, err)
// 		}
// 		if s.trackConn(conn, remote) {
// 			newPeer(conn, remote, s, s.privKey, s.identPub)
// 		} else { // Stop was called
// 			conn.Close()
// 			return
// 		}
// 	}
// }

// func (ns *IdentityService) Stop() {
// }
