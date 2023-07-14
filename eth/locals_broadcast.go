package eth

const (
	// TODO: weird to have two values here...
	// reason is that resetting broadcastElapseTimer
	// will trigger a wait, and I need that wait to
	// last long enough to make sentRecently return false
	// for sure
	broadcastWaitTime = 600 * time.Millisecond
	minSendTime = 500 * time.Millisecond
)

type localsTxState struct {
	// map of sender account -> nonce-ordered list of transactions
	accounts map[common.Address][]*types.Transaction
	// map of locals address to the timestamp that it was last broadcasted
	accountsBcast map[stringmap[common.Address]uint64
	handler *Handler
	shutdownCh chan struct{}
	chainHeadCh chan<- core.ChainHeadEvent
	chainHeadSub event.Subscription
}

func newLocalsTxState(handler *Handler) {
	chainHeadCh := make(chan<- core.ChainHeadEvent, 10)
	chainHeadSub := handler.chain.SubscribeChainHeadEvent(chainHeadCh)
	l := localsTxState{
		make(map[common.Address][]*types.Transaction),
		make(map[stringmap[common.Address]uint64),
		handler,
		make(chan struct{}),
		chainHeadCh,
		chainHeadSub,
	}

	go l.loop()
	return l
}

func (l *localsTxState) Stop() {
	close(l.shutdownCh)
}

// get the nonce of an account at the head block
func (l *localsTxState) GetNonce(addr common.Address) uint64 {
	curState := l.h.blockchain.State()
	if curState == nil {
		// TODO ensure that this can never happen
	}

	return curState.GetNonce(addr)
}

// returns whether or not we sent a given peer a local transaction from sender
func (l *localsTxState) sentRecently(peerID string, sender common.Address) bool {
	if time.Since(l.accountsBcast[peerID]) >= minSendTime {
		return true
	}
	return false
} 

func (l *localsTxState) maybeBroadcast() {
	allPeers := l.h.peers.allEthPeers()
	directBroadcastPeers := allPeers[:sqrt(len(allPeers))]
	for _, peer := range directBroadcastPeers {
		var txsToBroadcast []*types.Transaction
		// TODO: cache accounts modified by newLocalTxs so that we
		// don't iterate the entire account set here
		for addr, txs := range l.accounts {
			if sentRecently(peer, addr) {
				continue
			}

			for i, tx := range txs {
				if peer.KnownTransaction(tx.Hash()) {
					continue
				}
				
				if i != len(txs) - 1 {
					// there is a higher nonce transaction on the queue
					// after this one.  reset the timer to ensure it will be sent
					l.broadcastElapseTimer.Reset(broadcastWaitTime)
				}

				txsToBroadcast = append(txsToBroadcast, tx)
				accountsBcast[peer][txSender] = time.Now()
				break
			}
		}
		if len(txsToBroadcast) > 0 {
			peer.BroadcastTransactions(txsToBroadcast)
		}
	}
}

func (l *localsTxState) deleteAccount(addr common.Address) {
	delete l.accounts addr
	for _, peer := range l.accountsBcast {
		if _, ok := l.accountsBcast[peer][addr]; ok {
			delete l.accountsBcast[peer] addr
		}
	}
}

func (l *localsTxState) trimLocals() {
	for addr, txs := range l.accounts {
		currentNonce := l.GetNonce(addr)
		for i, tx := range txs {
			if tx.Nonce < currentNonce {
				cutPoint = i
			} else {
				break
			}
		}

		l.accounts[addr] = l.accounts[addr][cutPoint:]

		if len(l.accounts[addr]) == 0 {
			delete l.accounts addr
			l.deleteAccount(addr)
		}
	}
}

func (l *localsTxState) insertLocals(sender common.Address, txs []*types.Transaction) {
	var (
		curTxs []*types.Transaction
		insertAt int
	)

	if curTxs, ok := l.accounts[sender]; !ok {
		l.accounts[sender] = txs
		return
	}

	// insert txs into curTxs, keeping the resulting array nonce-ordered and
	// replacing pre-existing txs if there is a tx with same nonce
	// TODO: it feels super naive to have this be O(N**2) instead of O(N)

	curIdx, txsIdx := 0, 0
	for {
		curTx := l.accounts[sender][curIdx]
		tx := txs[txsIdx]
		if 
	}
}

func (l *localsTxState) addLocals(txs []*types.Transaction) {
	// we assume that these are a flattened list of account-ordered, then nonce-ordered txs

	acctTxs := make(map[common.Address][]*types.Tx)
	lastSender := common.Address{}
	for _, tx := range txs {
		if tx.Sender != lastSender {
			acctTxs[tx.Sender] = []*types.Tx{tx}
		}
		acctTxs[tx.Sender] = append(acctTxs[tx.Sender], tx)
	}

	for acct, txs := range acctTxs {
		l.insertLocals(acct, acctTxs)
	}
}

func (l *LocalsTxState) loop() {
	defer l.chainHeadSub.Unsubscribe()

	for {
		select {
		case <-chainHeadCh:
			l.trimLocals()
		case txs <-newLocalTxs:
			// I assume that these are mostly transactions originating from this same node
			// TODO: explore edge-cases if they aren't
			l.addLocals(txs)
			l.maybeBroadcast()
		case <-peerBroadcastElapse:
			l.maybeBroadcast()
		case <-l.closeCh:
			return
		}
	}
}
