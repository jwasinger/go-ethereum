package elhook

import (
	"context"
	"fmt"
	"time"

	"net/http"
	"github.com/ethereum/go-ethereum/rpc"
)

type ELClientHook struct {

}

func (c *ELClientHook) Connect(ctx context.Context, httpEndpoint string) error {
	httpEndpoint = "http://127.0.0.1:8545"
	client, err := rpc.DialOptions(ctx, httpEndpoint, rpc.WithHTTPClient(&http.Client{
                Timeout: 10 * time.Second,
        }))
	if err != nil {
		fmt.Println("err1")
		fmt.Println(err)
		return err
	}

/*
	auth := ""
	client.SetHeader("Authorization", auth)
*/

	engine_exchangeCapabilities_method := "engine_exchangeCapabilities"
	var result *[]string

	// 1) check caps: ensure the engine api methods we need are enabled
	err = client.CallContext(ctx, &result, engine_exchangeCapabilities_method, nil)
	if err != nil {
		fmt.Println("error")
		fmt.Println(err)
	}

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
