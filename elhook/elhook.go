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
		fmt.Println("err1")
		fmt.Println(err)
		return err
	}

	_ = engineAPI

	panic("success")
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
