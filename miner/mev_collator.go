type MEVCollator struct {
    // TODO keep active bundles
    counter uint64
}

type MevBundle struct {
    Txs               Transactions
    BlockNumber       *big.Int
    MinTimestamp      uint64
    MaxTimestamp      uint64
    RevertingTxHashes []common.Hash
}

type simulatedBundle struct {
    mevGasPrice       *big.Int
    totalEth          *big.Int
    ethSentToCoinbase *big.Int
    totalGasUsed      uint64
    originalBundle    MevBundle
}

func computeBundleGas(bundle MEVBundle, bs BlockState) err, simulatedBundle {
	err, receipts := bs.AddTransactions(bundle.Transactions)
    if err != nil {
        return err
    }
}

func (c *MEVCollator) CollateBlock(bs BlockState, pool Pool, state ReadOnlyState) {
    // create a copy of the BlockState and send it to each worker.
    if c.counter == math.MaxUint64 {
        c.counter = 0
    } else {
        c.counter++
    }

    // TODO signal to our "normal" worker to start building a normal block

    bundles, err := c.eligibleBundles(bs)
    if err != nil {
        log.Error("failed to fetch eligible bundles", "err", err)
        return
    }

    if len(bundles) > 0 {
        simulatedBundles := make([]simulatedBundle, len(bundles))

        // 1) simulate each bundle in this go-routine (TODO see if doing it in parallel is worth it in a future iteration)
        simulatedBundles := simulateBundles(bs, bundles)

        // 2) concurrently build 0..N-1 blocks with 0..N-1 max merged bundles
        for i := 0; i < len(bundles); i++ {
            c.bundleWorkers[i].newWorkCh <- bundlerWork{blockState: bs, counter: counter}
        }
    }

    bundlesReceived := 0
    for {
        select {
        case resp := <-c.workResponseCh:
            // don't care about responses that are stale:
            //  responses from previous calls to CollateBlock
            if resp.counter != counter {
                break
            }
            // workers set the blockState to nil in the response if they were
            // interrupted (recommit interrupt, new chain canon head received)
            if resp.blockState == nil {
                return
            }
            bundlesReceived++
            // only interrupt the sealer if a more profitable block is found
            if bestProfit.Cmp(resp.profit) < 0 {
                bestProfit.Set(resp.profit) // copy here just to be overly safe until POC is working
                resp.blockState.Commit()
            }
            // we're done when all the eligible bundle blocks have been returned
            // and the standard-strategy block has been returned
            if bundlesReceived == len(bundles) + 1 {
                return
            }
        }
    }
}

func (c *MEVCollator) Start() {

}

func (c *MEVCollator) Close() {

}
