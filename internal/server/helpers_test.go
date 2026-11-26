package server_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gqlo/alertbox/internal/server"
	"github.com/gqlo/alertbox/internal/store"
	"github.com/redis/go-redis/v9"
)

func testStore(t *testing.T) *store.AlertStore {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return store.NewAlertStoreWithRedis(rdb)
}

func testServer(t *testing.T) *server.Server {
	t.Helper()
	return server.NewServer(testStore(t), "", "")
}

func sampleAlert(id, severity, status, source, ns, alertname string, received time.Time) store.StoredAlert {
	return store.StoredAlert{
		ID:         id,
		ReceivedAt: received,
		Source:     source,
		Status:     status,
		Labels: map[string]string{
			"severity":  severity,
			"alertname": alertname,
			"namespace": ns,
		},
		Annotations: map[string]string{"summary": "test"},
	}
}
