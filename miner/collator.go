type BlockState interface {

}

type EVMError int
const (
    reverted = iota
    //
)

type AddTransactionError int
const (
    ErrInterrupt AddTransactionError = iota
    ErrGasLimitReached AddTransactionError
)

// Pool is an interface to the transaction pool
type Pool interface {
	Pending(bool) (map[common.Address]types.Transactions, error)
	Locals() []common.Address
}

type blockState struct {
    worker *worker
    env *environment
    commitMu *sync.Mutex
    start time.Time
    snapshots []int
    state *types.StateDB
}

func (bs *blockState) AddTransaction() (EVMError, AddTransactionError, *types.Receipt) {
    snapshot := bs.state.Snapshot()

    if bs.interrupt != nil && atomic.Load(bs.interrupt) != CommitInterruptNone {
        return nil, ErrInterrupt, nil
    }

    if gasPool.Gas() < params.TxGas {

    }
    from, _ := types.Sender(signer, tx)
    // Check whether the tx is replay protected. If we're not in the EIP155 hf
    // phase, start ignoring the sender until we do.
    if tx.Protected() && !chainConfig.IsEIP155(header.Number) {

    }

    gasPrice, err := tx.EffectiveGasTip(bs.work.env.header.BaseFee)
    if err != nil {

    }

    state.Prepare(tx.Hash(), tcount)
    txLogs, err = commitTransaction(chain, chainConfig, bs.work.env, tx, bs.Coinbase())

    if err != nil {

    } else {
        bs.snapshots = append(bs.snapshots, snapshot)
    }
}

func (bs *blockState) RevertTransaction() {

}

func (bs *blockState) Commit() bool {
    bs.commitMu.Lock()
    defer bs.commitMu.Unlock()

    if bs.done {
        return false
    }

    if bs.interrupt != nil && atomic.LoadInt32(bs.interrupt) != CommitInterruptNone {
        bs.done = true
        return false
    }

    bs.done = true
    bs.worker.commit(bs.worker, bs.worker.fullTaskHook, true, bs.start)

    bs.worker.current.discard()
    bs.worker.current = bs.env
    return true
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

func (c *defaultCollator) Collateblock(bs blockstate, pool pool) {

}

func submitTransactions(bs blockState, txs *types.TransactionsByPriceAndNonce) bool {
   for {
		// If we don't have enough gas for any further transactions then we're done
		available := bs.Gas()
		if available < params.TxGas {
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Enough space for this tx?
		if available < tx.Gas() {
			txs.Pop()
			continue
		}

		evmErr, addTxErr, receipt = bs.AddTransaction(tx)
		if evmErr != nil {

		}

        if addTxErr != nil {
            // only abort if the interrupt was triggered
        }
   }
}
