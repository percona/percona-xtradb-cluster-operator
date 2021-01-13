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
