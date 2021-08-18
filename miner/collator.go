type BlockState interface {

}

type AddTransactionError int
const (
    ErrInterrupt AddTransactionError = iota
    ErrGasLimitReached AddTransactionError
    ErrNonceTooLow AddTransactionError
    ErrNonceTooHigh AddTransactionError
    ErrTxTypeNotSupported AddTransactionError
    ErrStrange AddTransactionError
    ErrGasFeeCapTooLow AddTransactionError
    ErrInterrupt AddTransactionError
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

func (bs *blockState) AddTransaction() (AddTransactionError, *types.Receipt) {
    snapshot := bs.state.Snapshot()

    if bs.interrupt != nil && atomic.Load(bs.interrupt) != CommitInterruptNone {
        return ErrInterrupt, nil
    }

    if gasPool.Gas() < params.TxGas {
        return ErrGasLimitReached, nil
    }

    from, _ := types.Sender(signer, tx)
    // Check whether the tx is replay protected. If we're not in the EIP155 hf
    // phase, start ignoring the sender until we do.
    if tx.Protected() && !chainConfig.IsEIP155(header.Number) {
        return ErrTxTypeNotSupported, nil
    }

    gasPrice, err := tx.EffectiveGasTip(bs.work.env.header.BaseFee)
    if err != nil {
        return ErrGasFeeCapTooLow, nil
    }

    state.Prepare(tx.Hash(), tcount)
    txLogs, err = commitTransaction(chain, chainConfig, bs.env, tx, bs.Coinbase())
    if err != nil {
        switch {
        case errors.Is(err, core.ErrGasLimitReached):
            // this should never be reached.
            // should be caught above
            return ErrGasLimitReached, nil
        case errors.Is(err, core.ErrNonceTooLow):
            return ErrNonceTooLow, nil
        case errors.Is(err, core.ErrNonceTooHigh):
            return ErrNonceTooHigh, nil
        case errors.Is(err, core.ErrTxTypeNotSupported):
            // TODO check that this unspported tx type is the same as the one caught above
            return ErrTxTypeNotSupported, nil
        default:
            return ErrStrange, nil
        }
    } else {
        bs.snapshots = append(bs.snapshots, snapshot)
        bs.coalescedLogs = append(coalescedLogs, txLogs)
        bs.env.tcount++
    }

    return nil, bs.env.receipts[len(bs.env.receipts) - 1]
}

func (bs *blockState) RevertTransaction() {
    if len(bs.snapshots) == 0 {
        return
    }
    bs.state.revertToSnapshot(bs.snapshots[len(bs.snapshots) - 1])
    bs.snapshots = bs.snapshots[:len(bs.snapshots) - 1]
    bs.coalescedLogs =  bs.coalescedLogs[:len(bs.coalescedLogs) - 1]
    bs.env.tcount--
    bs.env.transactions = bs.env.transactions[:len(bs.env.transactions) - 1]
    bs.env.receipts = bs.env.receipts[:len(bs.env.receipts) - 1]
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
