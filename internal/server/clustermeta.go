package server

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/gqlo/alertbox/internal/client"
)

const clusterMetaTTL = 5 * time.Minute

func (s *Server) cachedClusterMeta() client.ClusterMeta {
	s.clusterMetaMu.RLock()
	if !s.clusterMetaAt.IsZero() && time.Since(s.clusterMetaAt) < clusterMetaTTL {
		meta := s.clusterMeta
		s.clusterMetaMu.RUnlock()
		return meta
	}
	stale := s.clusterMeta
	s.clusterMetaMu.RUnlock()

	s.triggerClusterMetaRefresh()
	return stale
}

func (s *Server) refreshClusterMetaLoop() {
	s.refreshClusterMeta()

	ticker := time.NewTicker(clusterMetaTTL)
	defer ticker.Stop()
	for range ticker.C {
		s.refreshClusterMeta()
	}
}

func (s *Server) triggerClusterMetaRefresh() {
	go s.refreshClusterMeta()
}

func (s *Server) refreshClusterMeta() {
	if !atomic.CompareAndSwapInt32(&s.clusterMetaRefreshing, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.clusterMetaRefreshing, 0)

	metaCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.EnsureClusterAuthToken(); err != nil {
		log.Printf("cluster meta: token: %v", err)
	}

	meta := client.FetchClusterMeta(metaCtx)
	if meta.Empty() {
		if client.OcBinaryAvailable() {
			log.Print("cluster meta: oc queries returned no data (check oc login and permissions)")
		}
		return
	}

	s.clusterMetaMu.Lock()
	s.clusterMeta = meta
	s.clusterMetaAt = time.Now()
	s.clusterMetaMu.Unlock()
}
