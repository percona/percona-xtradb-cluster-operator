package config

import (
	"strconv"

	"github.com/pkg/errors"
	res "k8s.io/apimachinery/pkg/api/resource"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
)

const (
	chunkSizeMin     int64 = 1048576
	chunkSizeDefault int64 = 134217728
)

func getAutoTuneParams(cr *api.PerconaXtraDBCluster, q *res.Quantity) (string, error) {
	autotuneParams := ""

	bufferConfigured, err := cr.ConfigHasKey("mysqld", "innodb_buffer_pool_size")
	if err != nil {
		return autotuneParams, errors.Wrap(err, "check if innodb_buffer_pool_size configured")
	}

	maxConnConfigured, err := cr.ConfigHasKey("mysqld", "max_connections")
	if err != nil {
		return autotuneParams, errors.Wrap(err, "check if max_connections configured")
	}

	if !bufferConfigured {
		poolSize := q.Value() / int64(100) * int64(75)
		if q.Value()-poolSize < int64(1000000000) {
			poolSize = q.Value() / int64(100) * int64(50)
		}
		if poolSize%chunkSizeDefault != 0 {
			poolSize += chunkSizeDefault - (poolSize % chunkSizeDefault)
		}

		// Adjust innodb_buffer_pool_chunk_size
		// If innodb_buffer_pool_size is bigger than 1Gi, innodb_buffer_pool_instances is set to 8.
		// By default, innodb_buffer_pool_chunk_size is 128M and innodb_buffer_pool_size needs to be
		// multiple of innodb_buffer_pool_chunk_size * innodb_buffer_pool_instances.
		// More info: https://dev.mysql.com/doc/refman/8.0/en/innodb-buffer-pool-resize.html
		if poolSize > int64(1073741824) {
			chunkSize := poolSize / 8
			// round to multiple of chunkSizeMin
			chunkSize = chunkSize + chunkSizeMin - (chunkSize % chunkSizeMin)

			poolSize = chunkSize * 8

			chunkSizeVal := strconv.FormatInt(chunkSize, 10)
			paramValue := "\n" + "innodb_buffer_pool_chunk_size" + " = " + chunkSizeVal
			autotuneParams += paramValue
		}

		poolSizeVal := strconv.FormatInt(poolSize, 10)
		paramValue := "\n" + "innodb_buffer_pool_size" + " = " + poolSizeVal
		autotuneParams += paramValue
	}

	if !maxConnConfigured {
		divider := int64(12582880)
		if q.Value() < divider {
			return "", errors.New("Not enough memory set in requests. Must be >= 12Mi.")
		}
		maxConnSize := q.Value() / divider
		maxConnSizeVal := strconv.FormatInt(maxConnSize, 10)
		paramValue := "\n" + "max_connections" + " = " + maxConnSizeVal
		autotuneParams += paramValue
	}

	return autotuneParams, nil
}
