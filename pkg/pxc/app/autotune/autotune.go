package autotune

import (
	"errors"
	"strconv"
	"strings"

	res "k8s.io/apimachinery/pkg/api/resource"
)

var params = []string{
	"innodb_buffer_pool_size",
	"max_connections",
}

func GetAutoTuneParams(config string, memory string) (string, error) {
	autotuneParams := ""
	q, err := res.ParseQuantity(memory)
	if err != nil {
		return "", err
	}
	for _, p := range params {
		if strings.Contains(config, p) {
			continue
		}
		paramValue := ""
		switch p {
		case "innodb_buffer_pool_size":
			poolSize := q.Value() / int64(100) * int64(75)
			poolSizeVal := strconv.FormatInt(poolSize, 10)
			paramValue = "\n" + p + " = " + poolSizeVal
		case "max_connections":
			devider := int64(12582880)
			if q.Value() < devider {
				return "", errors.New("not enough memory")
			}
			maxConnSize := q.Value() / devider
			maxConnSizeVal := strconv.FormatInt(maxConnSize, 10)
			paramValue = "\n" + p + " = " + maxConnSizeVal
		}
		autotuneParams += paramValue
	}

	return autotuneParams, nil
}
