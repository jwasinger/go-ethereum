// Copyright Supranational LLC
// Licensed under the Apache License, Version 2.0, see LICENSE for details.
// SPDX-License-Identifier: Apache-2.0

package arith384

//go:noescape
func add_mod_384(ret, a, b, p *Element)

//go:noescape
func sub_mod_384(ret, a, b, p *Element)

//go:noescape
func mul_mod_384(ret, a, b, p *Element, inv uint64)
