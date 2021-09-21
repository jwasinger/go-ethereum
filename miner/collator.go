type CollatorAPI interface {
	Version() string
	Service() interface{}
}

// Pool is an interface to the transaction pool
type Pool interface {
	Pending(bool) (map[common.Address]types.Transactions, error)
	Locals() []common.Address
}

/*
	BlockState represents an under-construction block.  An instance of
	BlockState is passed to CollateBlock where it can be filled with transactions
	via BlockState.AddTransaction() and submitted for sealing via
	BlockState.Commit().
	Operations on a single BlockState instance are not threadsafe.  However,
	instances can be copied with BlockState.Copy().
*/
type BlockState interface {
	/*
		adds a single transaction to the blockState.  Returned errors include ..,..,..
		which signify that the transaction was invalid for the current EVM/chain state.
		ErrRecommit signals that the recommit timer has elapsed.
		ErrNewHead signals that the client has received a new canonical chain head.
		All subsequent calls to AddTransaction fail if either newHead or the recommit timer
		have occured.
		If the recommit interval has elapsed, the BlockState can still be committed to the sealer.
	*/
	AddTransactions(tx types.Transactions) (error, types.Receipts)

	/*
		removes a number of transactions from the block resetting the state to what
		it was before the transactions were added.  If count is greater than the number
		of transactions in the block,  returns
	*/
	RevertTransactions(count uint) error

	/*
		returns true if the Block has been made the current sealing block.
		returns false if the newHead interrupt has been triggered.
		can also return false if the BlockState is no longer valid (the call to CollateBlock
		which the original instance was passed has returned).
	*/
	Commit() bool
	Copy() BlockState
	State() vm.StateReader
	Signer() types.Signer
	Header() *types.Header
	/*
		the account which will receive the block reward.
	*/
	Etherbase() common.Address
}

type MinerState interface {
    IsRunning() bool
    ChainConfig() params.ChainConfig
    // TODO method to get fresh block?
}

type minerState struct {
    chainConfig
}

type BlockCollatorWork struct {
    Ctx context.Context
    Block *BlockState
}

type Collator interface {
    CollateBlocks(blockCh chan-> BlockCollatorWork, exitCh chan-> struct{})
}

type collatorBlockState struct {
    env *environment
    committed bool
    shouldSeal bool
	state     *state.StateDB
	txs      []*types.Transaction
	receipts []*types.Receipt
	tcount    int            // tx count in cycle
	gasPool   *core.GasPool  // available gas used to pack transactions

	start      time.Time
	logs       []*types.Log
	committed  bool
	snapshots  []int
}

var (
	ErrAlreadyCommitted         = errors.New("can't mutate BlockState after calling Commit()")

	// errors which indicate that a given transaction cannot be
	// added at a given block or chain configuration.
	ErrGasLimitReached    = errors.New("gas limit reached")
	ErrNonceTooLow        = errors.New("tx nonce too low")
	ErrNonceTooHigh       = errors.New("tx nonce too high")
	ErrTxTypeNotSupported = errors.New("tx type not supported")
	ErrGasFeeCapTooLow    = errors.New("gas fee cap too low")
	// error which encompasses all other reasons a given transaction
	// could not be added to the block.
	ErrStrange = errors.New("strange error")
)

func (bs *collatorBlockState) Commit() {
    if bs.committed {
        return
    }
    bs.env.worker.currentMu.Lock()
    defer bs.env.worker.currentMu.Unlock()
    if bs.env.ctx != nil && bs.env.ctx.Done() {
        return
    }

    // todo make next 2 lines a function of environment (e.g. commitBS())...?
    bs.env.current = bs
    if bs.shouldSeal {
        bs.env.worker.commit(bs.env.copy(), nil, true, time.Now())
    }

    bs.committed = true
}

func copyLogs(logs []*types.Log) []*types.Log {
	result := make([]*types.Log, len(logs))
	for _, l := range logs {
		logCopy := types.Log{
			Address:     l.Address,
			BlockNumber: l.BlockNumber,
			TxHash:      l.TxHash,
			TxIndex:     l.TxIndex,
			Index:       l.Index,
			Removed:     l.Removed,
		}
		for _, t := range l.Topics {
			logCopy.Topics = append(logCopy.Topics, t)
		}
		logCopy.Data = make([]byte, len(l.Data))
		copy(logCopy.Data[:], l.Data[:])

		result = append(result, &logCopy)
	}

	return result
}

// copyReceipts makes a deep copy of the given receipts.
func copyReceipts(receipts []*types.Receipt) []*types.Receipt {
	result := make([]*types.Receipt, len(receipts))
	for i, l := range receipts {
		cpy := *l
		result[i] = &cpy
	}
	return result
}

func (bs *collatorBlockState) Copy() *collatorBlockState {
	cpy := collatorBlockState{
		env: bs.env,
		shouldSeal: bs.shouldSeal,
		state: bs.state.Copy(),
        tcount: bs.tcount,
        committed: bs.committed,
		logs: copyLogs(bs.logs),
		receipts: copyReceipts(bs.receipts),
	}

    if env.gasPool != nil {
            cpy.gasPool = new(core.GasPool)
            cpy.gasPool = *env.gasPool
    }
    cpy.txs = make([]*types.Transaction, len(bs.txs))
    copy(cpy.txs, bs.txs)
	cpy.snapshots = make([]int, len(bs.snapshots)
	copy(cpy.snapshots, bs.snapshots)

    return &cpy
}

func (bs *blockState) AddTransactions(txs types.Transactions) (error, types.Receipts) {
	tcount := 0
	var retErr error = nil

	if len(txs) == 0 {
		return ErrZeroTxs, nil
	}

	if bs.committed {
		return ErrAlreadyCommitted, nil
	}

	for _, tx := range txs {
		if bs.env.gasPool.Gas() < params.TxGas {
			return ErrGasLimitReached, nil
		}

		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.Protected() && !bs.worker.chainConfig.IsEIP155(bs.env.header.Number) {
			return ErrTxTypeNotSupported, nil
		}

		// TODO can this error also be returned by commitTransaction below?
		_, err := tx.EffectiveGasTip(bs.env.header.BaseFee)
		if err != nil {
			return ErrGasFeeCapTooLow, nil
		}

		snapshot := bs.env.state.Snapshot()
		bs.env.state.Prepare(tx.Hash(), bs.env.tcount+tcount)
		txLogs, err := bs.worker.commitTransaction(bs.env, tx, bs.env.etherbase)
		if err != nil {
			switch {
			case errors.Is(err, core.ErrGasLimitReached):
				// this should never be reached.
				// should be caught above
				retErr = ErrGasLimitReached
			case errors.Is(err, core.ErrNonceTooLow):
				retErr = ErrNonceTooLow
			case errors.Is(err, core.ErrNonceTooHigh):
				retErr = ErrNonceTooHigh
			case errors.Is(err, core.ErrTxTypeNotSupported):
				// TODO check that this unspported tx type is the same as the one caught above
				retErr = ErrTxTypeNotSupported
			default:
				retErr = ErrStrange
			}

			bs.logs = bs.logs[:len(bs.logs)-tcount]
			bs.env.state.RevertToSnapshot(bs.snapshots[len(bs.snapshots)-tcount])
			bs.snapshots = bs.snapshots[:len(bs.snapshots)-tcount]

			return retErr, nil
		} else {
			bs.logs = append(bs.logs, txLogs...)
			bs.snapshots = append(bs.snapshots, snapshot)
			tcount++
		}
	}

	retReceipts := bs.env.receipts[bs.env.tcount:]
	bs.env.tcount += tcount

	return nil, retReceipts
}

func (bs *blockState) RevertTransactions(count uint) error {
	if bs.committed {
		return ErrCommitted
	} else if int(count) > len(bs.snapshots) {
		return ErrTooManyTxs
	} else if count == 0 {
		return ErrZeroTxs
	}
	bs.state.RevertToSnapshot(bs.snapshots[len(bs.snapshots)-int(count)])
	bs.snapshots = bs.snapshots[:len(bs.snapshots)-int(count)]
	return nil
}

// TODO move this to be a method of environment
/*
func (bs *collatorBlockState) Interrupted() bool {
	if bs.env.ctx != nil {
			select {
			case <-bs.env.ctx.Done():
				return true
			default:
				return false
			}
	}

	return false
}
*/

func (bs *blockState) State() vm.StateReader {
	return bs.env.state
}

func (bs *blockState) Signer() types.Signer {
	return bs.env.signer
}
