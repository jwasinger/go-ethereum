package elhook

import (
	"context"
	"fmt"
)

type ELClientHook struct {
	engineAPI *EngineAPI
}

func (c *ELClientHook) Connect(ctx context.Context, httpEndpoint string) error {
	engineAPI, err := NewWithConnection(ctx)
	if err != nil {
		panic(err)
		return err
	}

	capsRequested := []string{"engine_newPayloadV1"}
	caps, err := engineAPI.ExchangeCapabilities(ctx, capsRequested)
	if err != nil {
		panic(err)
	}

	fmt.Println(caps)

	return nil
}

func (c *ELClientHook) Run() {
	for {
		select {
		// newHead:
		// 	if not received from EL client, send forkChoiceUpdated to EL
		// 
		}
	}
}
