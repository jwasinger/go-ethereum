// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

//go:build gofuzz
// +build gofuzz

package bls

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"

	gnark "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fp"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/bls12381"
	blst "github.com/supranational/blst/bindings/go"
)

func FuzzCrossPairing(data []byte) int {
	input := bytes.NewReader(data)

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

	// compute pairing using geth
	engine := bls12381.NewPairingEngine()
	engine.AddPair(kpG1, kpG2)
	kResult := engine.Result()

	// compute pairing using gnark
	cResult, err := gnark.Pair([]gnark.G1Affine{*cpG1}, []gnark.G2Affine{*cpG2})
	if err != nil {
		panic(fmt.Sprintf("gnark/bls12381 encountered error: %v", err))
	}

	// compare result
	if !(bytes.Equal(cResult.Marshal(), bls12381.NewGT().ToBytes(kResult))) {
		panic("pairing mismatch gnark / geth ")
	}

	// compute pairing using blst
	blstResult := blst.Fp12MillerLoop(blG2, blG1)
	blstResult.FinalExp()
	res := massageBLST(blstResult.ToBendian())
	if !(bytes.Equal(res, bls12381.NewGT().ToBytes(kResult))) {
		panic("pairing mismatch blst / geth")
	}

	return 1
}

func FuzzCrossG1Add(data []byte) int {
	input := bytes.NewReader(data)

	// get random G1 points
	kp1, cp1, bl1, err := getG1Points(input)
	if err != nil {
		return 0
	}

	// get random G1 points
	kp2, cp2, bl2, err := getG1Points(input)
	if err != nil {
		return 0
	}

	// compute kp = kp1 + kp2
	g1 := bls12381.NewG1()
	kp := bls12381.PointG1{}
	g1.Add(&kp, kp1, kp2)

	// compute cp = cp1 + cp2
	_cp1 := new(gnark.G1Jac).FromAffine(cp1)
	_cp2 := new(gnark.G1Jac).FromAffine(cp2)
	cp := new(gnark.G1Affine).FromJacobian(_cp1.AddAssign(_cp2))

	// compare result
	if !(bytes.Equal(cp.Marshal(), g1.ToBytes(&kp))) {
		panic("G1 point addition mismatch gnark / geth ")
	}

	bl3 := blst.P1AffinesAdd([]*blst.P1Affine{bl1, bl2})
	if !(bytes.Equal(cp.Marshal(), bl3.Serialize())) {
		panic("G1 point addition mismatch blst / geth ")
	}

	return 1
}

func FuzzCrossG2Add(data []byte) int {
	input := bytes.NewReader(data)

	// get random G2 points
	kp1, cp1, bl1, err := getG2Points(input)
	if err != nil {
		return 0
	}

	// get random G2 points
	kp2, cp2, bl2, err := getG2Points(input)
	if err != nil {
		return 0
	}

	// compute kp = kp1 + kp2
	g2 := bls12381.NewG2()
	kp := bls12381.PointG2{}
	g2.Add(&kp, kp1, kp2)

	// compute cp = cp1 + cp2
	_cp1 := new(gnark.G2Jac).FromAffine(cp1)
	_cp2 := new(gnark.G2Jac).FromAffine(cp2)
	cp := new(gnark.G2Affine).FromJacobian(_cp1.AddAssign(_cp2))

	// compare result
	if !(bytes.Equal(cp.Marshal(), g2.ToBytes(&kp))) {
		panic("G2 point addition mismatch gnark / geth ")
	}

	bl3 := blst.P2AffinesAdd([]*blst.P2Affine{bl1, bl2})
	if !(bytes.Equal(cp.Marshal(), bl3.Serialize())) {
		panic("G1 point addition mismatch blst / geth ")
	}

	return 1
}

func FuzzCrossG1MultiExp(data []byte) int {
	var (
		input        = bytes.NewReader(data)
		gethScalars  []*big.Int
		gnarkScalars []fr.Element
		gethPoints   []*bls12381.PointG1
		gnarkPoints  []gnark.G1Affine
	)
	// n random scalars (max 17)
	for i := 0; i < 17; i++ {
		// note that geth/crypto/bls12381 works only with scalars <= 32bytes
		s, err := randomScalar(input, fr.Modulus())
		if err != nil {
			break
		}
		// get a random G1 point as basis
		kp1, cp1, _, err := getG1Points(input)
		if err != nil {
			break
		}
		gethScalars = append(gethScalars, s)
		var gnarkScalar = &fr.Element{}
		gnarkScalar = gnarkScalar.SetBigInt(s).FromMont()
		gnarkScalars = append(gnarkScalars, *gnarkScalar)

		gethPoints = append(gethPoints, new(bls12381.PointG1).Set(kp1))
		gnarkPoints = append(gnarkPoints, *cp1)
	}
	if len(gethScalars) == 0 {
		return 0
	}
	// compute multi exponentiation
	g1 := bls12381.NewG1()
	kp := bls12381.PointG1{}
	if _, err := g1.MultiExp(&kp, gethPoints, gethScalars); err != nil {
		panic(fmt.Sprintf("G1 multi exponentiation errored (geth): %v", err))
	}
	// note that geth/crypto/bls12381.MultiExp mutates the scalars slice (and sets all the scalars to zero)

	// gnark multi exp
	cp := new(gnark.G1Affine)
	cp.MultiExp(gnarkPoints, gnarkScalars)

	// compare result
	if !(bytes.Equal(cp.Marshal(), g1.ToBytes(&kp))) {
		panic("G1 multi exponentiation mismatch gnark / geth ")
	}

	return 1
}
