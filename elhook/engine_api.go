package elhook

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
)

type EngineAPI struct {
	client *rpc.Client
}

const (


	exchangeCapabilitiesMethod = "engine_exchangeCapabilities"
)

func NewWithConnection(ctx context.Context) (*EngineAPI, error) {
	var testSecret = [32]byte{94, 111, 36, 109, 245, 74, 43, 72, 202, 33, 205, 86, 199, 174, 186, 77, 165, 99, 13, 225, 149, 121, 125, 249, 128, 109, 219, 163, 224, 176, 46, 233}
	var testEndpoint = "http://127.0.0.1:8551"

	auth := node.NewJWTAuth(testSecret)
	client, err := rpc.DialOptions(ctx, testEndpoint, rpc.WithHTTPAuth(auth))
	if err != nil {
		fmt.Println("err1")
		fmt.Println(err)
		return nil, err
	}

	return &EngineAPI{client}, nil
}

func (e *EngineAPI) ExchangeCaps(ctx context.Context) ([]string, error) {
	var result *[]string

	err := e.client.CallContext(ctx, &result, exchangeCapabilitiesMethod, nil)
	if err != nil {
		fmt.Println("error")
		fmt.Println(err)
	}

	return []string{}, nil
}
