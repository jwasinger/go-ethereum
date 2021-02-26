package arith384

import (
	"math/bits"
)

const NUM_LIMBS = 6
type Element [NUM_LIMBS]uint64

// out <- x + y
func add(out *Element, x *Element, y *Element) {
	var c uint64
	c = 0

	out[0], c = bits.Add64(x[0], y[0], c)
	out[1], c = bits.Add64(x[1], y[1], c)
	out[2], c = bits.Add64(x[2], y[2], c)
	out[3], c = bits.Add64(x[3], y[3], c)
	out[4], c = bits.Add64(x[4], y[4], c)
	out[5], c = bits.Add64(x[5], y[5], c)
}

// out <- x - y
func sub(out *Element, x *Element, y *Element) (uint64){
	var c uint64
	c = 0

	out[0], c = bits.Sub64(x[0], y[0], c)
	out[1], c = bits.Sub64(x[1], y[1], c)
	out[2], c = bits.Sub64(x[2], y[2], c)
	out[3], c = bits.Sub64(x[3], y[3], c)
	out[4], c = bits.Sub64(x[4], y[4], c)
	out[5], c = bits.Sub64(x[5], y[5], c)

	return c
}

func Eq(x *Element, y *Element) bool {
    for i := 0; i < NUM_LIMBS; i++ {
        if x[i] != y[i] {
            return false
        }
    }

    return true
}

func MulMod(z, x, y, mod *Element, modinv uint64) {

	var t [6]uint64
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
		c[2], t[2] = madd2(m, mod[3], c[2], c[0])
		c[1], c[0] = madd1(v, y[4], c[1])
		c[2], t[3] = madd2(m, mod[4], c[2], c[0])
		c[1], c[0] = madd1(v, y[5], c[1])
		t[5], t[4] = madd3(m, mod[5], c[0], c[2], c[1])
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
		c[2], t[2] = madd2(m, mod[3], c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, mod[4], c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, mod[5], c[0], c[2], c[1])
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
		c[2], t[2] = madd2(m, mod[3], c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, mod[4], c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, mod[5], c[0], c[2], c[1])
	}
	{
		// round 3
		v := x[3]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * modinv
		c[2] = madd0(m, mod[0], c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, mod[1], c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, mod[2], c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], t[2] = madd2(m, mod[3], c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, mod[4], c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, mod[5], c[0], c[2], c[1])
	}
	{
		// round 4
		v := x[4]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * modinv
		c[2] = madd0(m, mod[0], c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, mod[1], c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, mod[2], c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], t[2] = madd2(m, mod[3], c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, mod[4], c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, mod[5], c[0], c[2], c[1])
	}
	{
		// round 5
		v := x[5]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * modinv
		c[2] = madd0(m, mod[0], c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], z[0] = madd2(m, mod[1], c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], z[1] = madd2(m, mod[2], c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], z[2] = madd2(m, mod[3], c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], z[3] = madd2(m, mod[4], c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		z[5], z[4] = madd3(m, mod[5], c[0], c[2], c[1])
	}

	// TODO can make the following faster and constant time

	// if z > q --> z -= q
	// note: this is NOT constant time
	if !(z[5] < mod[5] || (z[5] == mod[5] && (z[4] < mod[4] || (z[4] == mod[4] && (z[3] < mod[3] || (z[3] == mod[3] && (z[2] < mod[2] || (z[2] == mod[2] && (z[1] < mod[1] || (z[1] == mod[1] && (z[0] < mod[0] || (z[0] == mod[0] && (z[0] < mod[0]))))))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], mod[0], 0)
		z[0], b = bits.Sub64(z[0], mod[0], b)
		z[1], b = bits.Sub64(z[1], mod[1], b)
		z[2], b = bits.Sub64(z[2], mod[2], b)
		z[3], b = bits.Sub64(z[3], mod[3], b)
		z[4], b = bits.Sub64(z[4], mod[4], b)
		z[5], _ = bits.Sub64(z[5], mod[5], b)
	}
}

/*
	Modular Addition
*/
func AddMod(out *Element, x *Element, y *Element, mod *Element) {
	var c uint64
	var tmp Element

	tmp[0], c = bits.Add64(x[0], y[0], 0)
	tmp[1], c = bits.Add64(x[1], y[1], c)
	tmp[2], c = bits.Add64(x[2], y[2], c)
	tmp[3], c = bits.Add64(x[3], y[3], c)
	tmp[4], c = bits.Add64(x[4], y[4], c)
	tmp[5], c = bits.Add64(x[5], y[5], c)

	out[0], c = bits.Sub64(tmp[0], mod[0], 0)
	out[1], c = bits.Sub64(tmp[1], mod[1], c)
	out[2], c = bits.Sub64(tmp[2], mod[2], c)
	out[3], c = bits.Sub64(tmp[3], mod[3], c)
	out[4], c = bits.Sub64(tmp[4], mod[4], c)
	out[5], c = bits.Sub64(tmp[5], mod[5], c)

	// TODO shouldn't check for carry here use carry from before Sub64s ?
	if c != 0 { // unnecessary sub
		*out = tmp
	}
}

/*
	Modular Subtraction
*/
func SubMod(out *Element, x *Element, y *Element, mod *Element) {
	var c, c1 uint64
	var tmp Element

	tmp[0], c1 = bits.Sub64(x[0], y[0], 0)
	tmp[1], c1 = bits.Sub64(x[1], y[1], c1)
	tmp[2], c1 = bits.Sub64(x[2], y[2], c1)
	tmp[3], c1 = bits.Sub64(x[3], y[3], c1)
	tmp[4], c1 = bits.Sub64(x[4], y[4], c1)
	tmp[5], c1 = bits.Sub64(x[5], y[5], c1)

	out[0], c = bits.Add64(tmp[0], mod[0], 0)
	out[1], c = bits.Add64(tmp[1], mod[1], c)
	out[2], c = bits.Add64(tmp[2], mod[2], c)
	out[3], c = bits.Add64(tmp[3], mod[3], c)
	out[4], c = bits.Add64(tmp[4], mod[4], c)
	out[5], c = bits.Add64(tmp[5], mod[5], c)

	if c1 == 0 { // unnecessary add
		*out = tmp
	}
}

func lte(x *Element, y *Element) bool {
	for i := 0; i < NUM_LIMBS; i++ {
		if x[i] > y[i] {
			return false
		}
	}

	return true
}

// returns True if x < y
func lt384(x *Element, y *Element) bool {
	_, carry := bits.Sub64(x[0], y[0], 0)
	_, carry = bits.Sub64(x[1], y[1], carry)
	_, carry = bits.Sub64(x[2], y[2], carry)
	_, carry = bits.Sub64(x[3], y[3], carry)
	_, carry = bits.Sub64(x[4], y[4], carry)
	_, carry = bits.Sub64(x[5], y[5], carry)
	return carry != 0
}

// return True if a < b, else False
func lt(a_hi, a_lo, b_hi, b_lo uint64) bool {
	return a_hi < b_hi || (a_hi == b_hi && a_lo < b_lo)

	/*
	// the below should be faster... but it slows down the MulModMont benchmark by ~15%
	_, carry := bits.Sub64(a_lo, b_lo, 0)
	_, carry = bits.Sub64(a_hi, b_hi, carry)

	return carry != 0
	*/
}
