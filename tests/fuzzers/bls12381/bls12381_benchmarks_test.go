package bls

import (
	"bytes"
	"crypto/rand"
	"fmt"
	gnark "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/ethereum/go-ethereum/crypto/bls12381"
	blst "github.com/supranational/blst/bindings/go"
	"testing"
)

func BenchmarkPairing(b *testing.B) {
	input := rand.Reader
	// get random G1 points
	kpG1, cpG1, blG1, err := GetG1Points(input)
	if err != nil {
		panic(err)
	}

	// get random G2 points
	kpG2, cpG2, blG2, err := GetG2Points(input)
	if err != nil {
		panic(err)
	}

	// compute pairing using geth
	engine := bls12381.NewPairingEngine()
	engine.AddPair(kpG1, kpG2)
	kResult := engine.Result()

	b.Run("blst", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			blstResult := blst.Pairing1_MillerLoopAndFinalExp(blG2, blG1)
			res := massageBLST(blstResult.ToBendian())
			if !(bytes.Equal(res, bls12381.NewGT().ToBytes(kResult))) {
				panic("pairing mismatch blst / geth")
			}
		}
	})

	b.Run("gnark", func(b *testing.B) {
		for i := 0; i < b.N; i++ {

			// compute pairing using gnark
			cResult, err := gnark.Pair([]gnark.G1Affine{*cpG1}, []gnark.G2Affine{*cpG2})
			if err != nil {
				panic(fmt.Sprintf("gnark/bls12381 encountered error: %v", err))
			}

			// compare result
			if !(bytes.Equal(cResult.Marshal(), bls12381.NewGT().ToBytes(kResult))) {
				panic("pairing mismatch gnark / geth ")
			}
		}
	})
}

func BenchmarkG1Add(b *testing.B) {

}

func BenchmarkG1Mul(b *testing.B) {

}

func BenchmarkG2Add(b *testing.B) {

}

func BenchmarkG2Mul(b *testing.B) {

}
