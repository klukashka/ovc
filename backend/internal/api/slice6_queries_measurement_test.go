//go:build slice6_measurement

package api_test

import (
	"testing"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/api"
	"ovc/backend/internal/testdb"
	"ovc/backend/internal/teststorage"
)

func newQueryMeasurementHarness(t *testing.T) (*authHarness, *testdb.QueryMeasurements) {
	t.Helper()
	database, tracer := testdb.NewQueryMeasurement(t)
	h := newAuthHarnessWithDatabase(t, database, flowProxies(), api.NewHandler,
		func(local *teststorage.Storage) storage.Store { return local.Store })
	return h, tracer
}

func measurementQueryPlans(t *testing.T, h *authHarness, queries []testdb.MeasuredQuery, label string) {
	t.Helper()
	testdb.LogMeasurementQueryPlans(t, h.database, queries, label)
}
