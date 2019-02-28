package proxysqlcnf

import (
	"github.com/pkg/errors"
)

// LivenesProbe tries to connect to database to check if ProxySQL is alive.
func LivenessProbe(connstr string) error {
	conn, err := NewProxyManager(connstr)
	if err != nil {
		return errors.Wrap(err, "liveness probe has failed")
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

// ReadinessProbe tries to connect to database to check if ProxySQL is alive and ready to serve the requests.
func ReadinessProbe(connstr string) error {
	conn, err := NewProxyManager(connstr)
	if err != nil {
		return errors.Wrap(err, "readiness probe has failed")
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			panic(err)
		}
	}()

	isInit, err := conn.isProxyNodeInitialized()
	if err != nil || !isInit {
		return errors.Wrap(err, "readiness probe has failed")
	}

	isReady, err := conn.isProxyNodeReady()
	if err != nil || !isReady {
		return errors.Wrap(err, "readiness probe has failed")
	}
	return nil
}
