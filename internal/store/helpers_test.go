package store_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
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
