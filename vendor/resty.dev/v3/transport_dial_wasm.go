// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (js && wasm) || wasip1
// +build js,wasm wasip1

package resty

import (
	"context"
	"net"
)

func transportDialContext(_ *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return nil
}
