package collector

import (
	"context"
	"log"
)

// InvalidateCache is kept for compatibility with the wsrep notify handler that
// calls the collector when a node joins after SST. The collector has no GTID
// cache anymore: it resolves the resume point from the binlog headers on every
// run, so there is nothing to invalidate.
func InvalidateCache(_ context.Context, _ *Collector, hostname string) error {
	log.Printf("binlog GTID cache is not used anymore, nothing to invalidate for %s", hostname)
	return nil
}
