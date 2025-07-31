package core

import (
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/sync/errgroup"
	"runtime"
	"sync/atomic"
)

type balPrefetcher struct{}

func (p *balPrefetcher) Prefetch(block *types.Block, db *state.StateDB, interrupt *atomic.Bool) {
	al := block.Body().AccessList

	var workers errgroup.Group

	workers.SetLimit(runtime.NumCPU() / 2)

	for _, accesses := range al.Accesses {
		statedb := db.Copy()
		workers.Go(func() error {
			statedb.GetBalance(accesses.Address)
			for _, storageAccess := range accesses.StorageWrites {
				if interrupt != nil && interrupt.Load() {
					return nil
				}
				statedb.GetState(accesses.Address, storageAccess.Slot)
			}
			for _, storageRead := range accesses.StorageReads {
				if interrupt != nil && interrupt.Load() {
					return nil
				}
				statedb.GetState(accesses.Address, storageRead)
			}
			if interrupt != nil && interrupt.Load() {
				return nil
			}
			statedb.GetCode(accesses.Address)
			return nil
		})
	}
	workers.Wait()
}
