package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/pkg/errors"
)

const (
	CacheKey = "gtid-binlog-cache.json"
)

type BinlogCacheEntry struct {
	Binlogs map[string]string `json:"binlogs"` // binlog name -> gtid set
}

type HostBinlogCache struct {
	Entries       map[string]*BinlogCacheEntry `json:"entries"` // host -> binlogs
	Version       int                          `json:"version"`
	LastUpdatedAt time.Time                    `json:"last_updated_at"`
}

func loadCache(ctx context.Context, storage storage.Storage) (*HostBinlogCache, error) {
	cache := &HostBinlogCache{
		Entries: make(map[string]*BinlogCacheEntry),
		Version: 1,
	}

	objReader, err := storage.GetObject(ctx, CacheKey)
	if err != nil {
		if strings.Contains(err.Error(), "object not found") {
			log.Printf("WARNING: cache file %s not found", CacheKey)
			return cache, nil
		}
		return nil, errors.Wrap(err, "get cache from storage")
	}
	defer objReader.Close()

	if err := json.NewDecoder(objReader).Decode(cache); err != nil {
		return nil, errors.Wrap(err, "decode cache")
	}

	return cache, nil
}

func saveCache(ctx context.Context, storage storage.Storage, cache *HostBinlogCache) error {
	log.Printf("updating binlog cache")
	cache.LastUpdatedAt = time.Now()

	data, err := json.Marshal(cache)
	if err != nil {
		return errors.Wrap(err, "marshal cache")
	}

	err = storage.PutObject(ctx, CacheKey, bytes.NewReader(data), int64(len(data)))
	return errors.Wrap(err, "put cache to s3")
}
