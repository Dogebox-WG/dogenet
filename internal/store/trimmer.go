package store

import (
	"log"
	"time"

	"code.dogecoin.org/dogenet/internal/spec"
	"code.dogecoin.org/governor"
)

func NewStoreTrimmer(store spec.Store) governor.Service {
	return &StoreTrimmer{
		store: store,
	}
}

type StoreTrimmer struct {
	governor.ServiceCtx
	store spec.Store
}

// goroutine
func (sv *StoreTrimmer) Run() {
	store := sv.store.WithCtx(sv.Context)
	for {
		if sv.Sleep(1 * time.Hour) { // once an hour is enough
			return // stopping
		}
		advanced, remCore, remNode, err := store.TrimNodes()
		if err != nil {
			log.Printf("[store] TrimNodes: %v", err)
		} else {
			if advanced {
				log.Printf("[store] TrimNodes: day-count has advanced.")
			}
			log.Printf("[store] TrimNodes: trimmed %v core nodes, %d network nodes", remCore, remNode)
		}
	}
}
