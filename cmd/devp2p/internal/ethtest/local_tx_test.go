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

type fakePeer {

}

func (p *fakePeer) loop() {
	for {
		select {
		case <-NewPooledTransactions:
			// report them
			break
		case <-NewTransactionHashes:
			// report them
			break
		}
	}
}

func generateSignedLocalTx(account common.Addr, nonce uint64, maxPriorityFee uint64) types.Transaction {

}

// generate txs from a lot of accounts, with a lot of txs per account
func generateTestTxs() []types.Transaction {

}

// check that the tx hashes announce/broadcast were sent from different sender accounts
func checkUniqueSenders(txHashes) bool {
}

// wait for test transactions to be announced or propagated
func waitForTxs(txs map[common.Hash]types.Transaction) {
	for {
		select {
		case txHashes := <-p.ReportAnnounce:
			if !checkUniqueSenders(txHashes) {
				return errors.New("list of senders for transactions in announce/broadcast should be unique")
			}

			// TODO: check that delay from last announcement was sufficient
		case txHashes := <-p.ReportBroadcast:
			if !checkUniqueSenders(txHashes) {
				return errors.New("list of senders for transactions in announce/broadcast should be unique")
			}
			// TODO: check that delay from last announcement was sufficient
		case timeout := <-p.timeout:
			return errors.New("timeout without all txs being announced/broadcasted")
		}
	}
}

func (s *Suite) TestLocalTxBasic(t *utesting.T) {
	testTxs := generateTestTxs()

	// create geth node
	// create a few peer cxns

	gethNode.InsertTxs(testTxs)

	// check that they were announced/broadcasted properly
	waitForTxs(toMap(testTxs))

	// check that hashes were broadcasted to square root subset of peers
	
	// ideas (?):
	// check that all accounts that were direct-delivered have the max nonce
	// check that all accounts that were announced have the txHash for the max nonce
}

func (s *Suite) TestLocalTxReplacement(t *utesting.T) {

}
