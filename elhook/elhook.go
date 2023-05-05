package elhook

import (
	"context"
	"errors"
)

type ELClientHook struct {
	engineAPI *EngineAPI
}

func checkCaps(caps []string) bool {
	allMethods := requiredEngineAPIMethods()
	for supportedMethod := range allMethods {
		// TODO nested for-loop bad.  turn caps into a map
		isSupported := false
		for _, providedMethod := range caps {
			if providedMethod == supportedMethod {
				isSupported = true
				break
			}
		}

		if !isSupported {
			panic(supportedMethod)
			return false
		}
	}
	return true
}

func NewELConnector(ctx context.Context, httpEndpoint string) (*ELClientHook, error) {
	engineAPI, err := NewWithConnection(ctx)
	if err != nil {
		panic(err)
		return nil, err
	}
	return &ELClientHook{engineAPI}, nil
}

func (c *ELClientHook) Start() error {
	// TODO: we provide one method and it returns all supported ones right?
	capsRequested := []string{"engine_newPayloadV1"}
	caps, err := engineAPI.ExchangeCapabilities(ctx, capsRequested)
	if err != nil {
		return err
	}

	if !checkCaps(caps) {
		return errors.New("doesn't have required capability")
	}

	go connectionLifeCycle()

	return nil
}

// meant to be run in its own go-routine
func (c *ELClientHook) connectionLifeCycle() error {
	for {
		err := c.runConnection()
		if err == connection_lost {
			fmt.Println("connection lost attempting to reconnect in 5 seconds")
			time.Sleep(5 * time.Second)
			continue
		}
		panic(err)
	}
}

func (c *ELClientHook) runConnection() error {
	for {
		select {
		headBlk := <- c.newHeadCh:
			if el.headBlock != headBlock {
				if err := c.engineAPI.ForkChoiceUpdated(...); err != nil {
					return err
				}
			}
		_ := <-c.timerCh:
			if el_is_syncing() {
				continue
			}
			if we_can_sign {
				// TODO: probably want to move this into a separate go-routine that can be interrupted when a new head block is received
				payload := c.engineAPI.GetPayload(...)
				block := convertPayloadToBlock(payload)
				// TODO forward block to inserter
			}
		_ := <-c.quitCh:
			return nil
		}
	}
}
