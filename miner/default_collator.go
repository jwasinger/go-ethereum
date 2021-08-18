type DefaultCollator struct {}

func (c *defaultCollator) Collateblock(bs blockstate, pool pool) {

}

func submitTransactions(bs blockState, txs *types.TransactionsByPriceAndNonce) bool {
   for {
		// If we don't have enough gas for any further transactions then we're done
		available := bs.Gas()
		if available < params.TxGas {
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Enough space for this tx?
		if available < tx.Gas() {
			txs.Pop()
			continue
		}

		err, receipt = bs.AddTransaction(tx)
        if err != nil {
            // only abort if the interrupt was triggered
        }
   }
}
