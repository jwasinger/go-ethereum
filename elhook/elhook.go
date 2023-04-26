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

func (c *ELClientHook) Connect(ctx context.Context, httpEndpoint string) error {
	engineAPI, err := NewWithConnection(ctx)
	if err != nil {
		panic(err)
		return err
	}

	// TODO: we provide one method and it returns all supported ones right?
	capsRequested := []string{"engine_newPayloadV1"}
	caps, err := engineAPI.ExchangeCapabilities(ctx, capsRequested)
	if err != nil {
		return err
	}

	if !checkCaps(caps) {
		return errors.New("doesn't have required capability")
	}

	return nil
}

func (c *ELClientHook) Run() {
	for {
		select {
		// newHead:
		// 	if not received from EL client, send forkChoiceUpdated to EL
		//	
		//	if we can sign:
		//		send getPayload to EL
		//		if we aren't in-turn: jiggle and then report the block
		// timer:
		//	if we can sign:
		//		send getPayload to EL
		//		if we aren't in-turn: jiggle and then report the block
		// 
		// quit:
		//
		}
	}
}
