type collatorBlockState struct {
    env *environment
    committed bool
    shouldSeal bool
}

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

func (bs *collatorBlockState) AddTransactions() {

}

func (bs *collatorBlockState) Copy() *collatorBlockState {

}
