type BlockState interface {

}

// Pool is an interface to the transaction pool
type Pool interface {
	Pending(bool) (map[common.Address]types.Transactions, error)
	Locals() []common.Address
}

type blockState struct {
    worker *worker
    commitMu *sync.Mutex
    start time.Time
    snapshots []*types.StateDB
}

func (bs *blockState) AddTransaction() {

}

func (bs *blockState) RevertTransaction() {

}

func (bs *blockState) Commit() {
    bs.commitMu.Lock()
    defer bs.commitMu.Unlock()

    bs.worker.commit(bs.worker.copy(), bs.worker.fullTaskHook, true, bs.start)
}

func (bs *blockState) Copy() BlockState {
    snapshotCopies := []*types.StateDB
    for i := 0; i < len(bs.snapshots); i++ {
        snapshotCopies = append(snapshotCopies, bs.snapshots[i].Copy())
    }

    return &blockState {
        bs.worker,
        &bs.commitMu,
        bs.start,
        snapshotCopies,
    }
}

type DefaultCollator struct {}

func (c *DefaultCollator) CollateBlock(bs BlockState, pool Pool) {

}
