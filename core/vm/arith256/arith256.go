package arith256

// /!\ WARNING /!\
// this code has not been audited and is provided as-is. In particular,
// there is no security guarantees such as constant time implementation
// or side-channel attack resistance
// /!\ WARNING /!\

import (
	"math/big"
	"math/bits"
	"unsafe"
	//    "fmt"
)

type Element [4]uint64

// Limbs number of 64 bits words needed to represent Element
const Limbs = 4

// Bytes number bytes needed to represent Element
const Bytes = Limbs * 8

// convert little-endian ordered, little-endian limbs to a base-10 string representation
func (e *Element) String() string {
	result := big.NewInt(0)

	for i := range e {
		accum := new(big.Int)
		exp := new(big.Int)
		limb := new(big.Int)

		exp.SetString("10000000000000000", 16)
		exp.Exp(exp, big.NewInt(int64(i)), nil)
		limb.SetUint64(e[i])

		accum.Mul(limb, exp)
		result.Add(result, accum)
	}

	return result.String()
}

func ElementFromString(s string) *Element {
	// hacky
	val := new(big.Int)
	val.SetString(s, 10)
	val_bytes := val.Bytes()
	if len(val_bytes) > 32 {
		panic("val must fit in 6 x 64bit limbs")
	}

	var fill_len int = 32 - len(val_bytes)
	if fill_len > 0 {
		fill_bytes := make([]byte, fill_len, fill_len)
		val_bytes = append(fill_bytes, val_bytes...)
	}

	// reverse so that elements are little endian
	for i, j := 0, len(val_bytes)-1; i < j; i, j = i+1, j-1 {
		val_bytes[i], val_bytes[j] = val_bytes[j], val_bytes[i]
	}

	return (*Element)(unsafe.Pointer(&val_bytes[0]))
}

// Mul z = x * y mod q
// see https://hackmd.io/@zkteam/modular_multiplication
func MulMod(z, x, y, mod *Element, modinv uint64) {
	_mulGeneric(z, x, y, mod, modinv)
}

func (e *Element) MulModMont(x, y, mod *Element, inv uint64) {
	MulMod(e, x, y, mod, inv)
}

func (e *Element) ToMont(mod, r_squared *Element, inv uint64) {
	// TODO calculate r_squared using modulus: ( ( 1 << (limb_size * 8) ) ** 2 ) % mod
	e.MulModMont(e, r_squared, mod, inv)
}

func (e *Element) ToNorm(mod *Element, inv uint64) {
	one := ElementFromString("1")
	e.MulModMont(e, one, mod, inv)
}

func (e *Element) Eq(other *Element) bool {
	for i := 0; i < 4; i++ {
		if e[i] != other[i] {
			return false
		}
	}

	return true
}

// Add z = x + y % mod
func AddMod(z, x, y, mod *Element) {
	_addGeneric(z, x, y, mod)
}

// Sub  z = x - y % mod
func SubMod(z, x, y, mod *Element) {
	_subGeneric(z, x, y, mod)
}

func _mulGeneric(z, x, y, mod *Element, modinv uint64) {

	var t [4]uint64
	var c [3]uint64
	{
		// round 0
		v := x[0]
		c[1], c[0] = bits.Mul64(v, y[0])
		m := c[0] * modinv
		c[2] = madd0(m, mod[0], c[0])
		c[1], c[0] = madd1(v, y[1], c[1])
		c[2], t[0] = madd2(m, mod[1], c[2], c[0])
		c[1], c[0] = madd1(v, y[2], c[1])
		c[2], t[1] = madd2(m, mod[2], c[2], c[0])
		c[1], c[0] = madd1(v, y[3], c[1])
		t[3], t[2] = madd3(m, mod[3], c[0], c[2], c[1])
	}
	{
		// round 1
		v := x[1]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * modinv
		c[2] = madd0(m, mod[0], c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, mod[1], c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, mod[2], c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		t[3], t[2] = madd3(m, mod[3], c[0], c[2], c[1])
	}
	{
		// round 2
		v := x[2]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * modinv
		c[2] = madd0(m, mod[0], c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, mod[1], c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, mod[2], c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		t[3], t[2] = madd3(m, mod[3], c[0], c[2], c[1])
	}
	{
		// round 3
		v := x[3]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * modinv
		c[2] = madd0(m, mod[0], c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], z[0] = madd2(m, mod[1], c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], z[1] = madd2(m, mod[2], c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		z[3], z[2] = madd3(m, mod[3], c[0], c[2], c[1])
	}

	// TODO make following constant time/measure slowdown of constant time vs worst-case non-constant time
	// if z > q --> z -= q
	if !(z[3] < mod[3] || (z[3] == mod[3] && (z[2] < mod[2] || (z[2] == mod[2] && (z[1] < mod[1] || (z[1] == mod[1] && (z[0] < mod[0] || (z[0] == mod[0] && (z[0] < mod[0]))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], mod[0], b)
		z[1], b = bits.Sub64(z[1], mod[1], b)
		z[2], b = bits.Sub64(z[2], mod[2], b)
		z[3], _ = bits.Sub64(z[3], mod[3], b)
	}
}

/*
	Modular Addition
*/
func _addGeneric(out, x, y, mod *Element) {
	var c uint64
	var tmp Element

	tmp[0], c = bits.Add64(x[0], y[0], 0)
	tmp[1], c = bits.Add64(x[1], y[1], c)
	tmp[2], c = bits.Add64(x[2], y[2], c)
	tmp[3], c = bits.Add64(x[3], y[3], c)

	out[0], c = bits.Sub64(tmp[0], mod[0], 0)
	out[1], c = bits.Sub64(tmp[1], mod[1], c)
	out[2], c = bits.Sub64(tmp[2], mod[2], c)
	out[3], c = bits.Sub64(tmp[3], mod[3], c)

	if c != 0 { // unnecessary sub
		*out = tmp
	}
}

/*
	Modular Subtraction
*/
func _subGeneric(out, x, y, mod *Element) {
	var c, c1 uint64
	var tmp Element

	tmp[0], c1 = bits.Sub64(x[0], y[0], 0)
	tmp[1], c1 = bits.Sub64(x[1], y[1], c1)
	tmp[2], c1 = bits.Sub64(x[2], y[2], c1)
	tmp[3], c1 = bits.Sub64(x[3], y[3], c1)

	out[0], c = bits.Add64(tmp[0], mod[0], 0)
	out[1], c = bits.Add64(tmp[1], mod[1], c)
	out[2], c = bits.Add64(tmp[2], mod[2], c)
	out[3], c = bits.Add64(tmp[3], mod[3], c)

	if c1 == 0 { // unnecessary add
		*out = tmp
	}
}

/*
// Submod generated by goff.
func _subGeneric(z, x, y, mod *Element) {
    var b uint64
    z[0], b = bits.Sub64(x[0], y[0], 0)
    z[0], b = bits.Sub64(x[0], y[0], b)
    z[1], b = bits.Sub64(x[1], y[1], b)
    z[2], b = bits.Sub64(x[2], y[2], b)
    z[3], b = bits.Sub64(x[3], y[3], b)
    if b != 0 {
        var c uint64
        z[0], c = bits.Add64(z[0], mod[0], 0)
        z[0], c = bits.Add64(z[0], mod[0], c)
        z[1], c = bits.Add64(z[1], mod[1], c)
        z[2], c = bits.Add64(z[2], mod[2], c)
        z[3], _ = bits.Add64(z[3], mod[3], c)
    }
}
*/
