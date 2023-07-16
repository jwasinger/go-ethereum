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
	peers *peerSet
	chain *core.Blockchain
	shutdownCh chan struct{}
	chainHeadCh chan<- core.ChainHeadEvent
	chainHeadSub event.Subscription
}

func newLocalsTxState(chain *core.Blockchain, peers *peerSet) {
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
	curState := l.blockchain.State()
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
	allPeers := l.peerSet.allEthPeers()
	directBroadcastPeers := allPeers[:sqrt(len(allPeers))]
	for _, peer := range directBroadcastPeers {
		var txsToBroadcast []*types.Transaction
		// TODO: cache accounts modified by newLocalTxs so that we
		// don't iterate the entire account set here
		for addr, txs := range l.accounts {
			if l.sentRecently(peer, addr) {
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
	)

	if curTxs, ok := l.accounts[sender]; !ok {
		l.accounts[sender] = txs
		return
	}

	// insert txs into the sender's queue, keeping the resulting array nonce-ordered and
	// replacing pre-existing txs if there is a tx with same nonce

	res := []*types.Transaction{}
	curIdx, txsIdx := 0, 0
	for ; ; curIdx < len(curTxs) || txsIdx < len(txs) {
		if curIdx > len(curTxs) {
			res = append(res, curTxs[curIdx]
			curIdx++
			continue
		}
		if txsIdx > len(txs) {
			res = append(res, curTxs[txsIdx]
			txsIdx++
			continue
		}

		curTx := l.accounts[sender][curIdx]
		tx := txs[txsIdx]
		if curTx.Nonce > tx.Nonce {
			res = append(res, tx)
			txsIdx++
		} else if curTx.Nonce < tx.Nonce {
			res = append(res, curTx)
			curIdx++
		} else {
			res = append(res, tx)
			curIdx++
			txsIdx++
		}
	}
	l.accounts[sender] = res
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
