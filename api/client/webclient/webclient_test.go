/*
Copyright 2020-2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package webclient provides a client for the Teleport Proxy API endpoints.
package webclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func newPingHandler(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.RequestURI != path {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(PingResponse{ServerVersion: "test"})
	})
}

func TestPingHttpFallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handler := newPingHandler("/webapi/ping")
	httpSvr := httptest.NewServer(handler)
	defer httpSvr.Close()

	t.Run("Allowed on insecure & loopback", func(t *testing.T) {
		_, err := Ping(ctx, httpSvr.Listener.Addr().String(), true, nil, "")
		require.NoError(t, err)
	})

	t.Run("Denied on secure", func(t *testing.T) {
		_, err := Ping(ctx, httpSvr.Listener.Addr().String(), false, nil, "")
		require.Error(t, err)
	})

	t.Run("Denied on non-loopback", func(t *testing.T) {
		nonLoopbackSvr := httptest.NewUnstartedServer(handler)

		// replace the test-supplied loopback listener with the first available
		// non-loopback address
		nonLoopbackSvr.Listener.Close()
		l, err := net.Listen("tcp", "0.0.0.0:0")
		require.NoError(t, err)
		nonLoopbackSvr.Listener = l
		nonLoopbackSvr.Start()
		defer nonLoopbackSvr.Close()

		_, err = Ping(ctx, nonLoopbackSvr.Listener.Addr().String(), true, nil, "")
		require.Error(t, err)
	})
}

func TestFindHttpFallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handler := newPingHandler("/webapi/find")
	httpSvr := httptest.NewServer(handler)
	defer httpSvr.Close()

	t.Run("Allowed on insecure & loopback", func(t *testing.T) {
		_, err := Find(ctx, httpSvr.Listener.Addr().String(), true, nil)
		require.NoError(t, err)
	})

	t.Run("Denied on secure", func(t *testing.T) {
		_, err := Find(ctx, httpSvr.Listener.Addr().String(), false, nil)
		require.Error(t, err)
	})

	t.Run("Denied on non-loopback", func(t *testing.T) {
		svr := httptest.NewUnstartedServer(handler)

		// replace the loopback listener with the first available non-loopback address
		svr.Listener.Close()
		l, err := net.Listen("tcp", "0.0.0.0:0")
		require.NoError(t, err)
		svr.Listener = l
		svr.Start()
		defer svr.Close()

		_, err = Find(ctx, svr.Listener.Addr().String(), true, nil)
		require.Error(t, err)
	})
}
