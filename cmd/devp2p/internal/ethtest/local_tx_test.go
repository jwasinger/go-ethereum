package ethtest

// local_tx_test creates a geth node and establishes a lot of peer connections.
// transactions are created, signed and inserted into the node via RPC.
// tests ensure that local transactions are broadcasted/announced with delays

// b/c this is more of a black-box test, it's probably going to be put into own suite.
// it's easier to use the existing utility code in the eth suite for now

// automatically assume that the engine API is available, assume fixed jwt secret

// load pre-generated test private keys from a file under testdata
func loadAccountKeypairs() {

}

func generateSignedLocalTx(account common.Addr, nonce uint64, maxPriorityFee uint64) types.Transaction {
	return []types.Transaction{}
}

// generate txs from a lot of accounts, with a lot of txs per account
func generateTestTxs() []types.Transaction {
	return []types.Transaction{}
}

// check that the tx hashes announce/broadcast were sent from different sender accounts
func checkUniqueSenders(txHashes) bool {
	return false
}

type peerTxReport struct {
	peerIdx int
	hash common.Hash
}

// wait for test transactions to be announced or propagated
func waitForTxs(txSendCh, txHashAnnounceCh chan peerTxReport, txs map[common.Hash]types.Transaction) {
	timeout := time.NewTimer(10 * time.Seconds)
	for {
		select {
		case report := <-txSendCh:
			txHashes := report.Hashes
			if !checkUniqueSenders(txHashes) {
				return errors.New("list of senders for transactions in announce/broadcast should be unique")
			}
			// validate the txs (nonce is the next one we want for the given account, no multiple txs from same acct, there was proper delay)

			// check that delay from last announcement was sufficient
			// add it to tx hashes result map
			// if both result maps are full, return
		case report := <-txHashAnnounceCh:
			txHashes := report.Hashes
			if !checkUniqueSenders(txHashes) {
				return errors.New("list of senders for transactions in announce/broadcast should be unique")
			}
			// validate the tx hashes (nonce is the next one we want for the given account, no multiple txs from same acct, there was proper delay)
			// check that delay from last announcement was sufficient
			// add it to tx hashes result map
			// if both result maps are full, return
		case <-timeout.C:
			return errors.New("timeout without all txs being announced/broadcasted")
		}
	}
}


func peerLoop(peerIdx int, c *Conn, txsCh, txHashesCh chan peerTxReport) {
	for {
		switch msg := c.Read().(type) {
		case *Ping:
			// pong (TODO: see how often this should happen)
			panic("no pong!")
		case *PooledTransactionHashes:
			txHashesCh <- peerTxReport{peerIdx, PooledTransactionHashes}
		case *NewTransactions:
			var hashes []common.Hash
			for _, tx := range NewTransactions {
				hashes = append(hashes, tx)
			}
			txsCh <- peerTxReport{peerIdx, hashes}
		default:
			return
		}
	}
}

func (s *Suite) TestLocalTxBasic(t *utesting.T) {
	var peer1, peer2 Conn
	// create geth node
	// create a few peer cxns
	peer1, err := s.dial()
	if err != nil {
		t.Fatal("fuck1")
	}

	peer2, err := s.dial()
	if err != nil {
		t.Fatal("fuck2")
	}

	defer peer1.close()
	defer peer2.close()

	peer1.statusExchange(s.chain, nil)
	peer2.statusExchange(s.chain, nil)

	txsCh := make(chan peerTxReport)
	txHashesCh := make(chan peerTxReport)

	go peerLoop(0, peer1, txsCh, txHashesCh)
	go peerLoop(1, peer2, txsCh, txHashesCh)

	// insert txs from many local accounts, many txs per account
	// testTxs := generateTestTxs()
	// gethNode.InsertLocalTxs(testTxs)

	// optional:  make a peer broadcast transactions to us and test remote tx propagation

	for {
		select {
		case txs := <-txsCh:

			// check there was proper delay
			// fill the result txs map
			// if the hashes map and txs map are full: return
		case hashes := <-txHashesCh:
			// validate the txHashes (nonce is the next one we want, no multiple txs from same acct, there was proper delay)
			// check there was proper delay
			// fill the result txHashes map
			// if the hashes map and txs map are full: return
		case <-timeoutCh:
			// fail test
		}
	}

	// check that hashes were broadcasted to square root subset of peers

	// ideas (?):
	// check that all accounts that were direct-delivered have the max nonce
	// check that all accounts that were announced have the txHash for the max nonce
}

/*
func (s *Suite) TestLocalTxReplacement(t *utesting.T) {

}
*/
