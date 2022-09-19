package bls

import (
	blst "github.com/supranational/blst/bindings/go"
	"bytes"
    "testing"
)

func BenchmarkPairing(b *testing.B) {
	input := bytes.NewReader([]byte{0})
	// get random G1 points
	kpG1, cpG1, blG1, err := getG1Points(input)
	if err != nil {
		return 0
	}

	// get random G2 points
	kpG2, cpG2, blG2, err := getG2Points(input)
	if err != nil {
		return 0
	}

	// compute pairing using blst
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        blstResult := blst.Fp12MillerLoop(blG2, blG1)
        blstResult.FinalExp()
        res := massageBLST(blstResult.ToBendian())
        if !(bytes.Equal(res, bls12381.NewGT().ToBytes(kResult))) {
            panic("pairing mismatch blst / geth")
        }
    }
}
