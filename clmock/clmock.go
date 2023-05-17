package clmock

type CLMock struct {
	ctx context.Context
}

func (c *CLMock) Start() error {
	go clmockLoop()
	return nil
}

func clmockLoop() {
	ticker := time.NewTicker(time.Milliseconds * 500)
	blockPeriod := time.Seconds * 12
	lastBlockTime := time.Now()

	var curForkchoiceState *engine.ForkChoiceStateV1
	var prevRandaoVal common.Hash
	var suggestedFeeRecipient common.Address

	for {
	select _ := <-c.ctx.Done():
		break
	case curTime := <-ticker.C:
		if curTime.After(lastBlockTime.Add(blockPeriod)) {
			// get the current head and populate curForkchoiceState

			// send forkchoiceupdated (to trigger block building)
			fcState, err := engine_api.ForkChoiceUpdatedV1(curForkchoiceState, engine.PayloadAttributes{
				Timestamp: curTime,
				Random: prevRandaoVal,
				SuggestedFeeRecipient: suggestedFeeRecipient,
			})

			if err != nil {
				// TODO: log error and hard-quit
				panic(err)
			}

			var payload *engine.ExecutableData

			// spin a bit until the payload is built
			for {
			select {
			case _ := <-buildTicker.C:
				// try and get the payload
				payload, err := engine_api.GetPayloadV1(fcState.PayloadID)
				if err != nil {
					// TODO: if err is that the payload is still building, continue spinning
					panic(err)
				}
			case _ := <-c.ctx.Done():
				return
			}

			// short-circuit if the payload didn't have transactions
			if len(payload.Transactions) == 0 {
				// TODO: more intelligent handling here
				continue
			}

			// mark the payload as canon
			if err = engine_api.NewPayloadV1(payload); err != nil {
				panic(err)
			}

			// send Forkchoiceupdated (if the payload had transactions)
			_, err = engine_api.ForkChoiceUpdatedV1(engine.ForkChoiceStateV1{
				HeadBlockHash: payload.BlockHash,
				SafeBlockHash: fcState.Safe.Hash,
				FinalizedBlockHash: fcState.Finalized.Hash,
			}, nil)
			if err != nil {
				panic(err)
			}
		}
	}
}

func (c *CLMock) Stop() error {
	c.ctx.Cancel()
	return nil
}
