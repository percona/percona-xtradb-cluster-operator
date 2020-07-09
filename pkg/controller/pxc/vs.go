package pxc

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/percona/percona-xtradb-cluster-operator/versionserviceclient"
	"github.com/percona/percona-xtradb-cluster-operator/versionserviceclient/models"
	"github.com/percona/percona-xtradb-cluster-operator/versionserviceclient/version_service"
)

const productName = "pxc-operator"

func (vs VersionServiceClient) GetExactVersion(vm versionMeta) (DepVersion, error) {
	requestURL, err := url.Parse(vs.URL)
	if err != nil {
		return DepVersion{}, err
	}

	srvCl := versionserviceclient.NewHTTPClientWithConfig(nil, &versionserviceclient.TransportConfig{
		Host:     requestURL.Host,
		BasePath: requestURL.Path,
		Schemes:  []string{requestURL.Scheme},
	})

	applyParams := &version_service.VersionServiceApplyParams{
		Apply:             vm.Apply,
		BackupVersion:     &vm.BackupVersion,
		CustomResourceOid: &vm.CRUID,
		DatabaseVersion:   &vm.PXCVersion,
		KubeVersion:       &vm.KubeVersion,
		OperatorVersion:   vs.OpVersion,
		Platform:          &vm.Platform,
		PmmVersion:        &vm.PMMVersion,
		Product:           productName,
		HTTPClient:        &http.Client{Timeout: 10 * time.Second},
	}
	applyParams = applyParams.WithTimeout(10 * time.Second)

	resp, err := srvCl.VersionService.VersionServiceApply(applyParams)

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

type versionMeta struct {
	Apply         string
	PXCVersion    string
	KubeVersion   string
	Platform      string
	PMMVersion    string
	BackupVersion string
	CRUID         string
}
