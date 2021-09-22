package miner

import (
	"errors"
    "time"
    "sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

type DefaultCollator struct {
    recommitMu sync.Mutex
    recommit time.Duration
    minRecommit time.Duration
    miner MinerState
    exitCh <-chan struct{}
}

// recalcRecommit recalculates the resubmitting interval upon feedback.
func recalcRecommit(minRecommit, prev time.Duration, target float64, inc bool) time.Duration {
	var (
		prevF = float64(prev.Nanoseconds())
		next  float64
	)
	if inc {
		next = prevF*(1-intervalAdjustRatio) + intervalAdjustRatio*(target+intervalAdjustBias)
		max := float64(maxRecommitInterval.Nanoseconds())
		if next > max {
			next = max
		}
	} else {
		next = prevF*(1-intervalAdjustRatio) + intervalAdjustRatio*(target-intervalAdjustBias)
		min := float64(minRecommit.Nanoseconds())
		if next < min {
			next = min
		}
	}
	return time.Duration(int64(next))
}

func (c *DefaultCollator) adjustRecommit(bs BlockState, inc bool) {
	c.recommitMu.Lock()
	defer c.recommitMu.Unlock()
	if inc {
		before := recommit
		ratio := float64(gasLimit-w.current.gasPool.Gas()) / float64(gasLimit)
		if ratio < 0.1 {
			ratio = 0.1
		}

		target := float64(recommit.Nanoseconds()) / ratio
		c.recommit = recalcRecommit(minRecommit, recommit, target, true)
		log.Trace("Increase miner recommit interval", "from", before, "to", recommit)
	} else {
		before := recommit
		c.recommit = recalcRecommit(minRecommit, recommit, float64(minRecommit.Nanoseconds()), false)
		log.Trace("Decrease miner recommit interval", "from", before, "to", recommit)
	}
}

func submitTransactions(ctx context.Context, bs BlockState, txs *types.TransactionsByPriceAndNonce) {
	header := bs.Header()
	availableGas := header.GasLimit
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if timer != nil {
			select {
			case <-timer.C:
				return
			default:
			}
		}

		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Enough space for this tx?
		if availableGas < tx.Gas() {
			txs.Pop()
			continue
		}
		from, _ := types.Sender(bs.Signer(), tx)

		err, receipts := bs.AddTransactions(types.Transactions{tx})
		switch {
		case errors.Is(err, ErrGasLimitReached):
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case errors.Is(err, ErrNonceTooLow):
			// New head notification data race between the transaction pool and miner, shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case errors.Is(err, ErrNonceTooHigh):
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case errors.Is(err, nil):
			availableGas = header.GasLimit - receipts[0].CumulativeGasUsed
			// Everything ok, collect the logs and shift in the next transaction from the same account
			txs.Shift()

		case errors.Is(err, ErrTxTypeNotSupported):
			// Pop the unsupported transaction without shifting in the next from the account
			log.Trace("Skipping unsupported transaction type", "sender", from, "type", tx.Type())
			txs.Pop()
		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	return
}

func (c *DefaultCollator) fillTransactions(ctx context.Context, bs BlockState, timer time.Timer, exitch <-chan struct{}) {
	header := bs.Header()
	txs, err := c.pool.Pending(true)
	if err != nil {
		log.Error("could not get pending transactions from the pool", "err", err)
		return
	}
	if len(txs) == 0 {
		return
	}
	// Split the pending transactions into locals and remotes
	localTxs, remoteTxs := make(map[common.Address]types.Transactions), txs
	for _, account := range c.pool.Locals() {
		if accountTxs := remoteTxs[account]; len(accountTxs) > 0 {
			delete(remoteTxs, account)
			localTxs[account] = accountTxs
		}
	}
	if len(localTxs) > 0 {
		if submitTransactions(bs, types.NewTransactionsByPriceAndNonce(bs.Signer(), localTxs, header.BaseFee)) {
			return true
		}
	}
	if len(remoteTxs) > 0 {
		if submitTransactions(bs, types.NewTransactionsByPriceAndNonce(bs.Signer(), remoteTxs, header.BaseFee)) {
			return true
		}
	}

	bs.Commit()

	return
}

func (c* DefaultCollator) workCycle() {
        for {
                c.recommitMu.Rlock()
                curRecommit := c.recommit
                c.recommitMu.Unlock()
                timer := time.NewTimer(curRecommit)

                bs := emptyBs.Copy()
                c.fillTransactions(bs, timer)
                bs.Commit()
                shouldContinue := false

                select {
                    case <-timer.C:
                        select {
                        case <-ctx.Done():
                            return
                        case <-c.exitCh:
                            return
						default:
                        }

                        c.adjustRecommit(bs, true)
                        shouldContinue = true
                    default:
                }

                if shouldContinue {
                    continue
                }

                select {
                case ctx.Done():
                    return
                case <-timer.C:
					// If mining is running resubmit a new work cycle periodically to pull in
					// higher priced transactions. Disable this overhead for pending blocks.
				    chainConfig := c.miner.ChainConfig()
					if c.miner.IsRunning() && (chainConfig.Clique == nil || chainConfig.Clique.Period > 0) {
                        c.adjustRecommit(bs, false)
					} else {
						return
					}
                case <-c.exitCh:
                    return
                }
        }
}

func (c *DefaultCollator) SetRecommit(interval time.Duration) {
    c.recommitMu.WLock()
    defer c.recommitMu.Unlock()

    c.recommit, c.minRecommit = interval, interval
}

func (c *DefaultCollator) CollateBlocks(miner MinerState) {
    c.miner = miner
    c.exitCh = exitCh
    for {
            select {
            case <-exitCh:
                return
            case cycleWork := <-blockCh:
                c.workCycle(cycleWork)
            }
    }
}
