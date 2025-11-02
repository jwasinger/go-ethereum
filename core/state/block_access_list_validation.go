package state

import "github.com/ethereum/go-ethereum/common"

type blockAccessListValidator struct {
}

func (a *blockAccessListValidator) HashRoot() common.Hash {
	return common.Hash{}
}

func (a *blockAccessListValidator) StateUpdate() {

}
