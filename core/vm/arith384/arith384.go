// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

package arith384

type Element [6]uint64

func AddMod(ret, a, b, p *Element) {
	add_mod_384(ret, a, b, p)
}

func SubMod(ret, a, b, p *Element) {
	sub_mod_384(ret, a, b, p)
}

func MulMod(ret, a, b, p *Element, inv uint64) {
	mul_mod_384(ret, a, b, p, inv)
}
