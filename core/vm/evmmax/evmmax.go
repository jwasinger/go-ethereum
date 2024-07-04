package evmmax

import (
	"encoding/binary"
	"errors"
	"math/big"
	"math/bits"
)

type arithFunc func(m *ModState, out, x, y []byte)

type ModState struct {
	modInv   uint64
	Mod      []byte
	ModInt   *big.Int
	RSquared []byte
	One      []byte

	AddMod  arithFunc
	SubMod  arithFunc
	MulMont arithFunc

	ElemSize uint
	Memory   []byte
	ValsUsed uint
}

func NewModState(modulus []byte, valsUsed uint) (*ModState, error) {
	if len(modulus) == 0 {
		return nil, errors.New("modulus length must not be zero")
	}
	if modulus[len(modulus)-1]%2 != 1 {
		return nil, errors.New("modulus must be odd")
	}

	// TODO ensure modulus length less than max

	if valsUsed > 256 {
		return nil, errors.New("too many values used")
	}

	// TODO ensure valsUsed != 0 ?

	mod_len_padded := len(modulus) + len(modulus)%8

	if mod_len_padded != 48 {
		return nil, errors.New("modulus must be 321-384 bits")
	}

	e := ModState{}
	e.ElemSize = uint(mod_len_padded)
	e.ValsUsed = valsUsed

	// store values in little-endian format internally to avoid byteswapping overhead
	e.One = make([]byte, e.ElemSize)
	e.One[0] = 1

	e.ModInt = new(big.Int).SetBytes(modulus)

	mod := make([]byte, mod_len_padded)
	copy(mod[mod_len_padded-len(modulus):], modulus[:])
	e.Mod = make([]byte, mod_len_padded)

	copy(e.Mod, mod)
	// byteswap to little-endian
	for i := 0; i < len(e.Mod)/2; i++ {
		e.Mod[len(e.Mod)-i-1], e.Mod[i] = e.Mod[i], e.Mod[len(e.Mod)-i-1]
	}

	rSquared := big.NewInt(1)
	rSquared.Lsh(rSquared, 8*e.ElemSize)
	rSquared.Mod(rSquared, e.ModInt)
	rSquared.Mul(rSquared, rSquared)
	rSquared.Mod(rSquared, e.ModInt)

	rSquaredBytes := rSquared.Bytes()
	e.RSquared = make([]byte, mod_len_padded)
	copy(e.RSquared[mod_len_padded-len(modulus):], rSquaredBytes[:])
	// byteswap to little-endian
	for i := 0; i < int(e.ElemSize)/2; i++ {
		e.RSquared[len(e.Mod)-i-1], e.RSquared[i] = e.RSquared[i], e.RSquared[len(e.Mod)-i-1]
	}

	mod_uint64 := uint(binary.BigEndian.Uint64(mod[len(mod)-8:]))
	e.modInv = uint64(negModInverse(mod_uint64))

    // TODO: configure asm or fallback choice with build tags
    e.AddMod = AddMod384Fallback
    e.SubMod = SubMod384Fallback
    e.MulMont = MulMont384Fallback

	e.Memory = make([]byte, mod_len_padded*int(valsUsed))

	return &e, nil
}

func negModInverse(mod uint) uint {
	k0 := 2 - mod
	t := mod - 1
	for i := 1; i < bits.UintSize; i <<= 1 {
		t *= t
		k0 *= (t + 1)
	}
	k0 = -k0
	return k0
}

func (m *ModState) LoadValues(dst []byte, startIdx uint) {
	ElemSize := int(m.ElemSize)
	numElts := len(dst) / int(ElemSize)
	valsBytes := m.Memory[int(startIdx)*ElemSize : (int(startIdx)+numElts)*ElemSize]
	for i := 0; i < numElts; i++ {
		valBytes := valsBytes[i*ElemSize : (i+1)*ElemSize]
		dstBytes := dst[i*ElemSize : (i+1)*ElemSize]
		// convert value to canonical form
		m.MulMont(m, dstBytes, valBytes, m.One)
		// byteswap the value to big-endian
		for j := 0; j < ElemSize/2; j++ {
			dstBytes[ElemSize-j-1], dstBytes[j] = dstBytes[j], dstBytes[ElemSize-j-1]
		}
	}
}

func (m *ModState) StoreValues(src []byte, startIdx uint) error {
	ElemSize := int(m.ElemSize)
	numElts := len(src) / ElemSize
	valsBytes := m.Memory[int(startIdx)*ElemSize : (int(startIdx)+numElts)*ElemSize]

	for i := 0; i < numElts; i++ {
		srcValBytes := src[i*ElemSize : (i+1)*ElemSize]
		if !m.LtMod(srcValBytes) {
			return errors.New("value must be less than the modulus")
		}
		// byteswap value to little-endian
		for j := 0; j < ElemSize/2; j++ {
			srcValBytes[len(srcValBytes)-j-1], srcValBytes[j] = srcValBytes[j], srcValBytes[len(srcValBytes)-j-1]
		}
		// convert value to Montgomery form
		m.MulMont(m, valsBytes[i*ElemSize:(i+1)*ElemSize], srcValBytes, m.RSquared)
	}
	return nil
}

type EVMMAXState struct {
	ActiveModulus *ModState
	moduli        map[uint]ModState
}

func NewEVMMAXState() *EVMMAXState {
	return &EVMMAXState{
		nil,
		make(map[uint]ModState),
	}
}

func (e *EVMMAXState) GetModState(modIdx uint) *ModState {
	if res, ok := e.moduli[modIdx]; ok {
		return &res
	}

	return nil
}

func (e *EVMMAXState) Setup(modIdx, valsUsed uint, modulus []byte) error {
	if valsUsed > 256 {
		return errors.New("cannot use more than 256 values")
	}

	if _, ok := e.moduli[modIdx]; !ok {
		modState, err := NewModState(modulus, valsUsed)
		if err != nil {
			return err
		}
		e.moduli[modIdx] = *modState
	}

	modState := e.moduli[modIdx]
	e.ActiveModulus = &modState
	return nil
}

func (e *EVMMAXState) GetElemSize(modIdx uint) uint {
	return e.moduli[modIdx].ElemSize
}

func (e *EVMMAXState) GetSlotBytes(modIdx uint) []byte {
	return e.moduli[modIdx].Memory
}

func (m *ModState) LtMod(val []byte) bool {
	valInt := new(big.Int).SetBytes(val)
	return valInt.Cmp(m.ModInt) < 0
}
