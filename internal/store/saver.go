package store

// GOB File Database

// const GobFilePath = "netmap.gob"

// func LoadStore() spec.Store {
// 	// load the previously saved state.
// 	db := New()
// 	err := db.LoadFrom(GobFilePath)
// 	if err != nil && !errors.Is(err, os.ErrNotExist) {
// 		log.Println("cannot read Gob file:", err)
// 		os.Exit(1)
// 	}
// 	return db
// }

// // Map Saver service

// type DBSaver struct {
// 	governor.ServiceCtx
// 	store spec.Store
// }

// func NewStoreSaver(store spec.Store) governor.Service {
// 	return &DBSaver{store: store}
// }

// func (m *DBSaver) Run() {
// 	m0, q0 := m.store.CoreStats()
// 	for {
// 		// save the map every 5 minutes, if modified
// 		if m.Sleep(5 * time.Minute) {
// 			return
// 		}
// 		m.store.TrimNodes()
// 		m1, q1 := m.store.CoreStats()
// 		if m1 != m0 || q1 != q0 { // has changed?
// 			m.saveNow()
// 			m0 = m1
// 			q0 = q1
// 		}
// 	}
// }

// func (m *DBSaver) saveNow() {
// 	defer func() {
// 		if err := recover(); err != nil {
// 			log.Printf("error saving network map: %v", err)
// 		}
// 	}()
// 	m.store.Persist(GobFilePath)
// }
