type MEVCollator struct {
    // TODO keep active bundles
    counter uint64
}

func computeBundleGas(bundle MEVBundle, bs BlockState) {

}

func (c *MEVCollator) CollateBlock(bs BlockState, pool Pool, state ReadOnlyState) {
    // create a copy of the BlockState and send it to each worker.
    if c.counter == math.MaxUint64 {
        c.counter = 0
    } else {
        c.counter++
    }

    // 1) simulate each bundle in this go-routine (TODO see if doing it in parallel is worth it in a future iteration)

    // 2) concurrently build 0..N-1 blocks with 0..N-1 max merged bundles

    // 3) as blocks from 2) come in, we flush them to the sealer if they are more profitable than the last most-profitable block seen.
}

func (c *MEVCollator) Start() {

}

func (c *MEVCollator) Close() {

}
