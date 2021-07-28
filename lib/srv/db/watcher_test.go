/*
Copyright 2021 Gravitational, Inc.

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

package db

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/lib/defaults"

	"github.com/stretchr/testify/require"
)

// TestWatcher verifies that database server properly detects and applies
// changes to database resources.
func TestWatcher(t *testing.T) {
	ctx := context.Background()
	testCtx := setupTestContext(ctx, t)

	// Create database server that isn't proxying any databases initially and
	// watches for databases with label group=a.
	resource, err := types.NewDatabaseServerV3(types.Metadata{
		Name: testCtx.hostID,
	}, types.DatabaseServerSpecV3{
		HostID:   testCtx.hostID,
		Hostname: constants.APIDomain,
		Selectors: []types.Selector{
			{MatchLabels: types.LabelsToProto(types.Labels{
				"group": []string{"a"},
			})},
		},
	})
	require.NoError(t, err)

	// This channel will receive new set of databases the server proxies
	// after each reconciliation.
	reconcileCh := make(chan types.Databases)
	databaseServer := testCtx.setupDatabaseServer(ctx, t, resource)
	databaseServer.cfg.OnReconcile = func(d types.Databases) {
		reconcileCh <- d
	}

	// Create database with label group=a.
	db1, err := makeDatabase("db1", map[string]string{"group": "a"})
	require.NoError(t, err)
	err = testCtx.authServer.CreateDatabase(ctx, db1)
	require.NoError(t, err)

	// It should be registered.
	select {
	case d := <-reconcileCh:
		require.Empty(t, cmp.Diff(types.Databases{db1}, d,
			cmpopts.IgnoreFields(types.Metadata{}, "ID"),
		))
	case <-time.After(time.Second):
		t.Fatal("Didn't receive reconcile event after 1s.")
	}

	// Create database with label group=b.
	db2, err := makeDatabase("db2", map[string]string{"group": "b"})
	require.NoError(t, err)
	err = testCtx.authServer.CreateDatabase(ctx, db2)
	require.NoError(t, err)

	// It shouldn't be registered.
	select {
	case d := <-reconcileCh:
		require.Empty(t, cmp.Diff(types.Databases{db1}, d,
			cmpopts.IgnoreFields(types.Metadata{}, "ID"),
		))
	case <-time.After(time.Second):
		t.Fatal("Didn't receive reconcile event after 1s.")
	}

	// Update db2 labels so it matches.
	db2.SetStaticLabels(map[string]string{"group": "a"})
	err = testCtx.authServer.UpdateDatabase(ctx, db2)
	require.NoError(t, err)

	// Both should be registered now.
	select {
	case d := <-reconcileCh:
		sort.Sort(d)
		require.Empty(t, cmp.Diff(types.Databases{db1, db2}, d,
			cmpopts.IgnoreFields(types.Metadata{}, "ID"),
		))
	case <-time.After(time.Second):
		t.Fatal("Didn't receive reconcile event after 1s.")
	}

	// Update db1 labels so it doesn't match.
	db1.SetStaticLabels(map[string]string{"group": "c"})
	err = testCtx.authServer.UpdateDatabase(ctx, db1)
	require.NoError(t, err)

	// Only db2 should remain registered.
	select {
	case d := <-reconcileCh:
		require.Empty(t, cmp.Diff(types.Databases{db2}, d,
			cmpopts.IgnoreFields(types.Metadata{}, "ID"),
		))
	case <-time.After(time.Second):
		t.Fatal("Didn't receive reconcile event after 1s.")
	}

	// Remove db2.
	err = testCtx.authServer.DeleteDatabase(ctx, db2.GetName())
	require.NoError(t, err)

	// Should be no registered databases.
	select {
	case d := <-reconcileCh:
		require.Len(t, d, 0)
	case <-time.After(time.Second):
		t.Fatal("Didn't receive reconcile event after 1s.")
	}
}

func makeDatabase(name string, labels map[string]string) (types.Database, error) {
	return types.NewDatabaseV3(types.Metadata{
		Name:   name,
		Labels: labels,
	}, types.DatabaseSpecV3{
		Protocol: defaults.ProtocolPostgres,
		URI:      "localhost:5432",
	})
}
