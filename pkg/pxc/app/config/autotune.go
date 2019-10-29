package config

import (
	"errors"
	"strconv"

	res "k8s.io/apimachinery/pkg/api/resource"
)

var params = []string{
	"innodb_buffer_pool_size",
	"max_connections",
}

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

	devider := int64(12582880)
	if q.Value() < devider {
		return "", errors.New("not enough memory")
	}
	maxConnSize := q.Value() / devider
	maxConnSizeVal := strconv.FormatInt(maxConnSize, 10)
	paramValue = "\n" + "max_connections" + " = " + maxConnSizeVal
	autotuneParams += paramValue

	return autotuneParams, nil
}
