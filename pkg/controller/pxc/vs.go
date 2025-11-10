package pxc

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	vsc "github.com/percona/percona-xtradb-cluster-operator/pkg/version/client"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version/client/models"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version/client/version_service"
)

const productName = "pxc-operator"

func (vs VersionServiceClient) GetExactVersion(cr *api.PerconaXtraDBCluster, endpoint string, vm versionMeta, opts versionOptions) (DepVersion, error) {
	if strings.Contains(endpoint, "https://check.percona.com/versions") {
		endpoint = api.GetDefaultVersionServiceEndpoint()
	}
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return DepVersion{}, err
	}

	srvCl := vsc.NewHTTPClientWithConfig(nil, &vsc.TransportConfig{
		Host:     requestURL.Host,
		BasePath: requestURL.Path,
		Schemes:  []string{requestURL.Scheme},
	})

	applyParams := &version_service.VersionServiceApplyParams{
		Apply:                 vm.Apply,
		BackupVersion:         &vm.BackupVersion,
		CustomResourceUID:     &vm.CRUID,
		DatabaseVersion:       &vm.PXCVersion,
		HaproxyVersion:        &vm.HAProxyVersion,
		KubeVersion:           &vm.KubeVersion,
		LogCollectorVersion:   &vm.LogCollectorVersion,
		NamespaceUID:          new(string),
		OperatorVersion:       cr.Spec.CRVersion,
		Platform:              &vm.Platform,
		PmmVersion:            &vm.PMMVersion,
		Product:               productName,
		ProxysqlVersion:       &vm.ProxySQLVersion,
		Context:               nil,
		ClusterWideEnabled:    &vm.ClusterWideEnabled,
		HTTPClient:            &http.Client{Timeout: 10 * time.Second},
		UserManagementEnabled: &vm.UserManagementEnabled,
	}
	applyParams = applyParams.WithTimeout(10 * time.Second)

	resp, err := srvCl.VersionService.VersionServiceApply(applyParams)
	if err != nil {
		return DepVersion{}, err
	}

	if !versionUpgradeEnabled(cr) {
		return DepVersion{}, nil
	}

	if len(resp.Payload.Versions) == 0 {
		return DepVersion{}, fmt.Errorf("empty versions response")
	}

	pxcVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Pxc)
	if err != nil {
		return DepVersion{}, errors.Wrapf(err, "get pxc version")
	}

	backupVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Backup)
	if err != nil {
		return DepVersion{}, errors.Wrapf(err, "get backup version")
	}

	pmmVersion, err := getPMMVersion(resp.Payload.Versions[0].Matrix.Pmm, opts.PMM3Enabled)
	if err != nil {
		return DepVersion{}, errors.Wrapf(err, "get pmm version")
	}

	proxySqlVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Proxysql)
	if err != nil {
		return DepVersion{}, errors.Wrapf(err, "get proxysql version")
	}

	haproxyVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Haproxy)
	if err != nil {
		return DepVersion{}, errors.Wrap(err, "haproxy version")
	}

	logCollectorVersion, err := getVersion(resp.Payload.Versions[0].Matrix.LogCollector)
	if err != nil {
		return DepVersion{}, errors.Wrap(err, "get logcollector version")
	}

	dv := DepVersion{
		PXCImage:            resp.Payload.Versions[0].Matrix.Pxc[pxcVersion].ImagePath,
		PXCVersion:          pxcVersion,
		BackupImage:         resp.Payload.Versions[0].Matrix.Backup[backupVersion].ImagePath,
		BackupVersion:       backupVersion,
		ProxySqlImage:       resp.Payload.Versions[0].Matrix.Proxysql[proxySqlVersion].ImagePath,
		ProxySqlVersion:     proxySqlVersion,
		PMMImage:            resp.Payload.Versions[0].Matrix.Pmm[pmmVersion].ImagePath,
		PMMVersion:          pmmVersion,
		HAProxyImage:        resp.Payload.Versions[0].Matrix.Haproxy[haproxyVersion].ImagePath,
		HAProxyVersion:      haproxyVersion,
		LogCollectorVersion: logCollectorVersion,
		LogCollectorImage:   resp.Payload.Versions[0].Matrix.LogCollector[logCollectorVersion].ImagePath,
	}

	return dv, nil
}

func getPMMVersion(versions map[string]models.VersionVersion, isPMM3 bool) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("response has zero versions")
	}
	// One version for PMM3 and one version for PMM2 should only exist.
	if len(versions) > 2 {
		return "", fmt.Errorf("response has more than 2 versions")
	}

	var pmm2Version, pmm3Version string
	for version := range versions {
		if strings.HasPrefix(version, "3.") {
			pmm3Version = version
		}
		if strings.HasPrefix(version, "2.") {
			pmm2Version = version
		}
	}

	if isPMM3 && pmm3Version == "" {
		return "", fmt.Errorf("pmm3 is configured, but no pmm3 version exists")
	}
	if isPMM3 && pmm3Version != "" {
		return pmm3Version, nil
	}
	if pmm2Version != "" {
		return pmm2Version, nil
	}

	return "", fmt.Errorf("no recognizable PMM version found")
}

func getVersion(versions map[string]models.VersionVersion) (string, error) {
	if len(versions) != 1 {
		return "", fmt.Errorf("response has multiple or zero versions %v", versions)
	}

	for k := range versions {
		return k, nil
	}
	return "", nil
}

type DepVersion struct {
	PXCImage            string `json:"pxcImage,omitempty"`
	PXCVersion          string `json:"pxcVersion,omitempty"`
	BackupImage         string `json:"backupImage,omitempty"`
	BackupVersion       string `json:"backupVersion,omitempty"`
	ProxySqlImage       string `json:"proxySqlImage,omitempty"`
	ProxySqlVersion     string `json:"proxySqlVersion,omitempty"`
	HAProxyImage        string `json:"haproxyImage,omitempty"`
	HAProxyVersion      string `json:"haproxyVersion,omitempty"`
	PMMImage            string `json:"pmmImage,omitempty"`
	PMMVersion          string `json:"pmmVersion,omitempty"`
	LogCollectorVersion string `json:"logCollectorVersion,omitempty"`
	LogCollectorImage   string `json:"LogCollectorImage,omitempty"`
}

type versionOptions struct {
	PMM3Enabled bool
}

type VersionService interface {
	GetExactVersion(cr *api.PerconaXtraDBCluster, endpoint string, vm versionMeta, opts versionOptions) (DepVersion, error)
}

type VersionServiceClient struct {
	OpVersion string
}

type versionMeta struct {
	Apply                 string
	PXCVersion            string
	KubeVersion           string
	Platform              string
	PMMVersion            string
	BackupVersion         string
	ProxySQLVersion       string
	HAProxyVersion        string
	LogCollectorVersion   string
	CRUID                 string
	ClusterWideEnabled    bool
	UserManagementEnabled bool
}
