// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

package evmmax

import (
	"unsafe"
)

//go:noescape
func AddMod384Asm(ret, a, b, p *[6]uint64)

//go:noescape
func SubMod384Asm(ret, a, b, p *[6]uint64)

//go:noescape
func MulMod384Asm(ret, a, b, p *[6]uint64, inv uint64)

func MulMont384(m *ModState, out_bytes, x_bytes, y_bytes []byte) {
	x := (*[6]uint64)(unsafe.Pointer(&x_bytes[0]))
	y := (*[6]uint64)(unsafe.Pointer(&y_bytes[0]))
	z := (*[6]uint64)(unsafe.Pointer(&out_bytes[0]))
	mod := (*[6]uint64)(unsafe.Pointer(&m.Mod[0]))

	MulMod384Asm(z, x, y, mod, m.modInv)
}

func AddMod384(m *ModState, out_bytes, x_bytes, y_bytes []byte) {
	x := (*[6]uint64)(unsafe.Pointer(&x_bytes[0]))
	y := (*[6]uint64)(unsafe.Pointer(&y_bytes[0]))
	z := (*[6]uint64)(unsafe.Pointer(&out_bytes[0]))
	mod := (*[6]uint64)(unsafe.Pointer(&m.Mod[0]))

	AddMod384Asm(z, x, y, mod)
}

func SubMod384(m *ModState, out_bytes, x_bytes, y_bytes []byte) {
	x := (*[6]uint64)(unsafe.Pointer(&x_bytes[0]))
	y := (*[6]uint64)(unsafe.Pointer(&y_bytes[0]))
	z := (*[6]uint64)(unsafe.Pointer(&out_bytes[0]))
	mod := (*[6]uint64)(unsafe.Pointer(&m.Mod[0]))

	SubMod384Asm(z, x, y, mod)
}
