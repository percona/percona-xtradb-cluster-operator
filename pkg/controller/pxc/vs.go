package pxc

import (
	"fmt"
	"net/url"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/versionserviceclient"
	"github.com/percona/percona-xtradb-cluster-operator/versionserviceclient/models"
	"github.com/percona/percona-xtradb-cluster-operator/versionserviceclient/version_service"
)

const productName = "pxc"

func (vs VersionServiceClient) GetExactVersion(desiredVersion, currentVersion string, versionMeta currVersionMeta) (DepVersion, error) {
	requestURL, err := url.Parse(vs.URL)
	if err != nil {
		return DepVersion{}, err
	}

	srvCl := versionserviceclient.NewHTTPClientWithConfig(nil, &versionserviceclient.TransportConfig{
		Host:     requestURL.Host,
		BasePath: requestURL.Path,
		Schemes:  []string{requestURL.Scheme},
	})

	params := version_service.NewVersionServiceApplyParamsWithTimeout(10 * time.Second).WithProduct(productName).
		WithOperatorVersion(vs.OpVersion).WithApply(desiredVersion).WithDatabaseVersion(&currentVersion).
		WithKubeVersion(&versionMeta.KubeVersion).WithPlatform(&versionMeta.Platform).WithCustomResourceOid(&versionMeta.CRUID).
		WithPmmVersion(&versionMeta.PMMVersion).WithBackupVersion(&versionMeta.BackupVersion)

	resp, err := srvCl.VersionService.VersionServiceApply(params)

	if err != nil {
		return DepVersion{}, err
	}

	if len(resp.Payload.Versions) == 0 {
		return DepVersion{}, fmt.Errorf("empty versions response")
	}

	pxcVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Pxc)
	if err != nil {
		return DepVersion{}, err
	}

	backupVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Backup)
	if err != nil {
		return DepVersion{}, err
	}

	pmmVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Pmm)
	if err != nil {
		return DepVersion{}, err
	}

	proxySqlVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Proxysql)
	if err != nil {
		return DepVersion{}, err
	}

	haproxyVersion, err := getVersion(r.Versions[0].Matrix.HAProxy)
	if err != nil {
		return DepVersion{}, err
	}

	return DepVersion{
		PXCImage:        resp.Payload.Versions[0].Matrix.Pxc[pxcVersion].ImagePath,
		PXCVersion:      pxcVersion,
		BackupImage:     resp.Payload.Versions[0].Matrix.Backup[backupVersion].ImagePath,
		BackupVersion:   backupVersion,
		ProxySqlImage:   resp.Payload.Versions[0].Matrix.Proxysql[proxySqlVersion].ImagePath,
		ProxySqlVersion: proxySqlVersion,
		PMMImage:        resp.Payload.Versions[0].Matrix.Pmm[pmmVersion].ImagePath,
		PMMVersion:      pmmVersion,
	}, nil
}

func getVersion(versions map[string]models.VersionVersion) (string, error) {
	if len(versions) != 1 {
		return "", fmt.Errorf("response has multiple or zero versions")
	}

	for k := range versions {
		return k, nil
	}
	return "", nil
}

type DepVersion struct {
	PXCImage        string `json:"pxcImage,omitempty"`
	PXCVersion      string `json:"pxcVersion,omitempty"`
	BackupImage     string `json:"backupImage,omitempty"`
	BackupVersion   string `json:"backupVersion,omitempty"`
	ProxySqlImage   string `json:"proxySqlImage,omitempty"`
	ProxySqlVersion string `json:"proxySqlVersion,omitempty"`
	HAProxyImage    string `json:"haproxyImage,omitempty"`
	HAProxyVersion  string `json:"haproxyVersion,omitempty"`
	PMMImage        string `json:"pmmImage,omitempty"`
	PMMVersion      string `json:"pmmVersion,omitempty"`
}

type VersionService interface {
	GetExactVersion(vm versionMeta) (DepVersion, error)
}

type VersionServiceClient struct {
	URL       string
	OpVersion string
}

type currVersionMeta struct {
	KubeVersion   string
	Platform      string
	PMMVersion    string
	BackupVersion string
	CRUID         string
}
