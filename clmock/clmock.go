// Copyright 2023 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package clmock

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/catalyst"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
)

type CLMock struct {
	ctx         context.Context
	cancel      context.CancelFunc
	stack       *node.Node
	eth         *eth.Ethereum
	blockPeriod time.Duration
}

func NewCLMock(stack *node.Node, eth *eth.Ethereum) *CLMock {
	chainConfig := eth.APIBackend.ChainConfig()

	if chainConfig.Dev == nil {
		log.Crit("incompatible pre-existing chain configuration")
	}

	return &CLMock{
		stack:       stack,
		eth:         eth,
		blockPeriod: time.Duration(chainConfig.Dev.Period) * time.Second,
	}
}

// Start invokes the clmock life-cycle function in a goroutine
func (c *CLMock) Start() error {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	go c.clmockLoop()
	return nil
}

// Stop halts the clmock service
func (c *CLMock) Stop() error {
	c.cancel()
	return nil
}

// clmockLoop manages the lifecycle of clmock.
// it drives block production, taking the role of a CL client and interacting with Geth via the engine API
func (c *CLMock) clmockLoop() {
	// TODO: (randomly placed here as a reminder to note it somewhere more prominent:
	// how do we sync node shutdown with this separate go-routine?
	// does it matter?  the worst that can happen is we get some weird error messages on node shutdown that might throw users off
	ticker := time.NewTicker(time.Millisecond * 500)
	lastBlockTime := time.Now()

	var curForkchoiceState engine.ForkchoiceStateV1
	var prevRandaoVal common.Hash
	var suggestedFeeRecipient common.Address

	// TODO: the following seems like a pretty sketchy/dangerous way to retrieve the ConsensusAPI
	// unsure of a cleaner way
	engineAPI := catalyst.NewConsensusAPI(c.eth)

	header := c.eth.BlockChain().CurrentHeader()

	curForkchoiceState = engine.ForkchoiceStateV1{
		HeadBlockHash:      header.Hash(),
		SafeBlockHash:      header.Hash(),
		FinalizedBlockHash: header.Hash(),
	}

	// if genesis block, send forkchoiceUpdated to trigger transition to PoS
	if header.Number.Cmp(big.NewInt(0)) == 0 {
		if _, err := engineAPI.ForkchoiceUpdatedV1(curForkchoiceState, nil); err != nil {
			log.Crit("failed to initiate PoS transition for genesis via Forkchoiceupdated", "err", err)
		}
	}

	for {
		select {
		case <-c.ctx.Done():
			break
		case curTime := <-ticker.C:
			if curTime.After(lastBlockTime.Add(c.blockPeriod)) {
				// trigger block building (via forkchoiceupdated)
				fcState, err := engineAPI.ForkchoiceUpdatedV1(curForkchoiceState, &engine.PayloadAttributes{
					Timestamp:             uint64(curTime.Unix()),
					Random:                prevRandaoVal,
					SuggestedFeeRecipient: suggestedFeeRecipient,
				})

				if err != nil {
					log.Crit("failed to trigger block building via forkchoiceupdated", "err", err)
				}

				var payload *engine.ExecutableData

				buildTicker := time.NewTicker(50 * time.Millisecond)
				// build the payload
				for {
					var done bool
					select {
					case <-buildTicker.C:
						payload, err = engineAPI.GetPayloadV1(*fcState.PayloadID)
						if err != nil {
							// the payload is still building, wait a bit and check again
							continue
						}
						done = true
						break
					case <-c.ctx.Done():
						return
					}
					if done {
						break
					}
				}

				// don't create a block if there are no transactions
				if len(payload.Transactions) == 0 {
					time.Sleep(c.blockPeriod)
					continue
				}

				// mark the payload as canonical
				if _, err = engineAPI.NewPayloadV1(*payload); err != nil {
					log.Crit("failed to mark payload as canonical", "err", err)
				}

				newForkchoiceState := &engine.ForkchoiceStateV1{
					HeadBlockHash:      payload.BlockHash,
					SafeBlockHash:      payload.BlockHash,
					FinalizedBlockHash: payload.BlockHash,
				}

				// mark the block containing the payload as canonical
				_, err = engineAPI.ForkchoiceUpdatedV1(*newForkchoiceState, nil)
				if err != nil {
					log.Crit("failed to mark block as canonical", "err", err)
				}
				lastBlockTime = time.Now()
				curForkchoiceState = *newForkchoiceState
			}
		}
	}
}
