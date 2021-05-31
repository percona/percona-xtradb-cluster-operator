package config

import (
	"errors"
	"strconv"

	res "k8s.io/apimachinery/pkg/api/resource"
)

func getAutoTuneParams(memory string) (string, error) {
	autotuneParams := ""
	q, err := res.ParseQuantity(memory)
	if err != nil {
		return "", err
	}

	poolSize := q.Value() / int64(100) * int64(75)
	poolSizeVal := strconv.FormatInt(poolSize, 10)
	paramValue := "\n" + "innodb_buffer_pool_size" + " = " + poolSizeVal
	autotuneParams += paramValue

	// Adjust innodb_buffer_pool_chunk_size
	// If innodb_buffer_pool_size is bigger than 1G, innodb_buffer_pool_instances is set to 8.
	// By default, innodb_buffer_pool_chunk_size is 128M and innodb_buffer_pool_size needs to be
	// multiple of innodb_buffer_pool_chunk_size * innodb_buffer_pool_instances.
	// More info: https://dev.mysql.com/doc/refman/8.0/en/innodb-buffer-pool-resize.html
	if poolSize > int64(1000000000) {
		chunkSize := q.Value() / int64(8)
		chunkSizeVal := strconv.FormatInt(chunkSize, 10)
		paramValue = "\n" + "innodb_buffer_pool_chunk_size" + " = " + chunkSizeVal
		autotuneParams += paramValue
	}

	divider := int64(12582880)
	if q.Value() < divider {
		return "", errors.New("Not enough memory set in requests. Must be >= 12Mi.")
	}
	maxConnSize := q.Value() / divider
	maxConnSizeVal := strconv.FormatInt(maxConnSize, 10)
	paramValue = "\n" + "max_connections" + " = " + maxConnSizeVal
	autotuneParams += paramValue

	return autotuneParams, nil
}
