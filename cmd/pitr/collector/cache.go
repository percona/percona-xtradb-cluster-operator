package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/backup/storage"
	"github.com/pkg/errors"
)

type BinlogCacheEntry struct {
	Binlogs map[string]string `json:"binlogs"` // binlog name -> gtid set
}

type HostBinlogCache struct {
	// host -> binlogs. we use pointer here to not copy BinlogCacheEntry
	// in case if it grows big and it'll grow big eventually.
	Entries       map[string]*BinlogCacheEntry `json:"entries"`
	Version       int                          `json:"version"`
	LastUpdatedAt time.Time                    `json:"last_updated_at"`
}

func (e *BinlogCacheEntry) Get(key string) (string, bool) {
	value, ok := e.Binlogs[key]
	return value, ok
}

func (e *BinlogCacheEntry) Set(key, value string) {
	e.Binlogs[key] = value
}

func loadCache(ctx context.Context, s storage.Storage, key string) (*HostBinlogCache, error) {
	cache := &HostBinlogCache{
		Entries: make(map[string]*BinlogCacheEntry),
		Version: 1,
	}

	objReader, err := s.GetObject(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			log.Printf("WARNING: cache file %s not found", key)
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

func saveCache(ctx context.Context, s storage.Storage, cache *HostBinlogCache, key string) error {
	log.Printf("updating binlog cache")
	cache.LastUpdatedAt = time.Now()

	data, err := json.Marshal(cache)
	if err != nil {
		return errors.Wrap(err, "marshal cache")
	}

	err = s.PutObject(ctx, key, bytes.NewReader(data), int64(len(data)))
	return errors.Wrap(err, "put cache to s3")
}

func InvalidateCache(ctx context.Context, c *Collector, hostname string) error {
	cache, err := loadCache(ctx, c.GetStorage(), c.GetGTIDCacheKey())
	if err != nil {
		return errors.Wrap(err, "load cache")
	}

	_, ok := cache.Entries[hostname]
	if !ok {
		return errors.Errorf("failed to find cache for %s", hostname)
	}

	log.Printf("invalidating cache for %s", hostname)
	delete(cache.Entries, hostname)

	err = saveCache(ctx, c.GetStorage(), cache, c.GetGTIDCacheKey())
	if err != nil {
		return errors.Wrap(err, "save cache")
	}

	return nil
}
