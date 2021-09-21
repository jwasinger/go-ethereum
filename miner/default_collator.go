type DefaultCollator struct {
    recommitMu sync.Mutex
    recommit time.Duration
    minRecommit time.Duration
    miner MinerState
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

func (c *DefaultCollator) fillTransactions(bs BlockState, timer time.Timer) {

}

func (c* DefaultCollator) workCycle() {
        c.timerMu.Rlock()
        curRecommit := c.recommit
        c.timerMu.Unlock()

        timer := time.NewTimer(curRecommit)

        for {
                bs := emptyBs.Copy()
                recommitOccured := c.fillTransactions(bs, timer)
                bs.Commit()
                shouldContinue := false

                select {
                    case <-timer.C:
                        select {
                        case <-ctx.Done():
                            return
                        case <-c.exitCh:
                            return
                        }

                        // adjust the recommit upwards and continue
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
                    // TODO in current geth miner, block recommit only happens when:
                    //      w.isRunning() && (w.chainConfig.Clique == nil || w.chainConfig.Clique.Period > 0)
                    // adjust the API to read those configs?

                    // TODO adjust the timer downwards and continue the loop
                    c.adjustRecommit(bs, false)
                case <-c.exitCh:
                    return
                }
        }
}

func (c *DefaultCollator) SetRecommit(interval time.Duration) {
    c.timerMu.WLock()
    defer c.timerMu.Unlock()

    c.recommit, c.minRecommit = interval, interval
}

func (c *DefaultCollator) CollateBlocks(blockCh chan-> BlockCollatorWork, exitCh chan-> struct{}) {
    for {
            select {
            case <-exitCh;
                return
            case cycleWork := <-blockCh:
                c.workCycle(cycleWork)
            }
    }
}
