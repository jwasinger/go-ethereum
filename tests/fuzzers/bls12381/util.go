package bls

import (
	"bytes"
    "fmt"

	gnark "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fp"
	"io"
	"github.com/ethereum/go-ethereum/crypto/bls12381"
	blst "github.com/supranational/blst/bindings/go"
	"github.com/ethereum/go-ethereum/common"
    "math/big"
    "crypto/rand"
)

func massageBLST(in []byte) []byte {
	out := make([]byte, len(in))
	len := 12 * 48
	// 1
	copy(out[0:], in[len-1*48:len])
	copy(out[1*48:], in[len-2*48:len-1*48])
	// 2
	copy(out[6*48:], in[len-3*48:len-2*48])
	copy(out[7*48:], in[len-4*48:len-3*48])
	// 3
	copy(out[2*48:], in[len-5*48:len-4*48])
	copy(out[3*48:], in[len-6*48:len-5*48])
	// 4
	copy(out[8*48:], in[len-7*48:len-6*48])
	copy(out[9*48:], in[len-8*48:len-7*48])
	// 5
	copy(out[4*48:], in[len-9*48:len-8*48])
	copy(out[5*48:], in[len-10*48:len-9*48])
	// 6
	copy(out[10*48:], in[len-11*48:len-10*48])
	copy(out[11*48:], in[len-12*48:len-11*48])
	return out
}

func GetG1Points(input io.Reader) (*bls12381.PointG1, *gnark.G1Affine, *blst.P1Affine, error) {
	// sample a random scalar
	s, err := randomScalar(input, fp.Modulus())
	if err != nil {
		return nil, nil, nil, err
	}

	// compute a random point
	cp := new(gnark.G1Affine)
	_, _, g1Gen, _ := gnark.Generators()
	cp.ScalarMultiplication(&g1Gen, s)
	cpBytes := cp.Marshal()

	// marshal gnark point -> geth point
	g1 := bls12381.NewG1()
	kp, err := g1.FromBytes(cpBytes)
	if err != nil {
		panic(fmt.Sprintf("Could not marshal gnark.G1 -> geth.G1: %v", err))
	}
	if !bytes.Equal(g1.ToBytes(kp), cpBytes) {
		panic("bytes(gnark.G1) != bytes(geth.G1)")
	}

	// marshal gnark point -> blst point
	scalar := new(blst.Scalar).FromBEndian(common.LeftPadBytes(s.Bytes(), 32))
	p1 := new(blst.P1Affine).From(scalar)
	if !bytes.Equal(p1.Serialize(), cpBytes) {
		panic("bytes(blst.G1) != bytes(geth.G1)")
	}

	return kp, cp, p1, nil
}

func GetG2Points(input io.Reader) (*bls12381.PointG2, *gnark.G2Affine, *blst.P2Affine, error) {
	// sample a random scalar
	s, err := randomScalar(input, fp.Modulus())
	if err != nil {
		return nil, nil, nil, err
	}

	// compute a random point
	cp := new(gnark.G2Affine)
	_, _, _, g2Gen := gnark.Generators()
	cp.ScalarMultiplication(&g2Gen, s)
	cpBytes := cp.Marshal()

	// marshal gnark point -> geth point
	g2 := bls12381.NewG2()
	kp, err := g2.FromBytes(cpBytes)
	if err != nil {
		panic(fmt.Sprintf("Could not marshal gnark.G2 -> geth.G2: %v", err))
	}
	if !bytes.Equal(g2.ToBytes(kp), cpBytes) {
		panic("bytes(gnark.G2) != bytes(geth.G2)")
	}

	// marshal gnark point -> blst point
	// Left pad the scalar to 32 bytes
	scalar := new(blst.Scalar).FromBEndian(common.LeftPadBytes(s.Bytes(), 32))
	p2 := new(blst.P2Affine).From(scalar)
	if !bytes.Equal(p2.Serialize(), cpBytes) {
		panic("bytes(blst.G2) != bytes(geth.G2)")
	}

	return kp, cp, p2, nil
}

func randomScalar(r io.Reader, max *big.Int) (k *big.Int, err error) {
	for {
		k, err = rand.Int(r, max)
		if err != nil || k.Sign() > 0 {
			return
		}
	}
}
