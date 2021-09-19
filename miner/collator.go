type collatorBlockState struct {

}

func (bs *collatorBlockState) Commit() {
    if bs.committed {
        return
    }
    bs.env.worker.currentMu.Lock()
    if bs.env.ctx.Done() {
        return
    }
    // todo make next 2 lines a function of environment (e.g. commitBS())...?
    bs.env.current = bs
    bs.env.worker.commit(bs.env.copy(), nil, true, time.Now())

    bs.env.worker.currentMu.Unlock()
}

func (bs *collatorBlockState) Copy() *collatorBlockState {

}
