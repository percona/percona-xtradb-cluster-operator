package pxc

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	vsc "github.com/percona/percona-xtradb-cluster-operator/version/client"
	"github.com/percona/percona-xtradb-cluster-operator/version/client/models"
	"github.com/percona/percona-xtradb-cluster-operator/version/client/version_service"
)

const productName = "pxc-operator"

func (vs VersionServiceClient) GetExactVersion(cr *api.PerconaXtraDBCluster, endpoint string, vm versionMeta) (DepVersion, error) {
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
		Apply:               vm.Apply,
		BackupVersion:       &vm.BackupVersion,
		CustomResourceUID:   &vm.CRUID,
		DatabaseVersion:     &vm.PXCVersion,
		HaproxyVersion:      &vm.HAProxyVersion,
		KubeVersion:         &vm.KubeVersion,
		LogCollectorVersion: &vm.LogCollectorVersion,
		NamespaceUID:        new(string),
		OperatorVersion:     cr.Spec.CRVersion,
		Platform:            &vm.Platform,
		PmmVersion:          &vm.PMMVersion,
		Product:             productName,
		ProxysqlVersion:     &vm.ProxySQLVersion,
		Context:             nil,
		ClusterWideEnabled:  &vm.ClusterWideEnabled,
		HTTPClient:          &http.Client{Timeout: 10 * time.Second},
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

	haproxyVersion, err := getVersion(resp.Payload.Versions[0].Matrix.Haproxy)
	if err != nil {
		return DepVersion{}, err
	}

	dv := DepVersion{
		PXCImage:        resp.Payload.Versions[0].Matrix.Pxc[pxcVersion].ImagePath,
		PXCVersion:      pxcVersion,
		BackupImage:     resp.Payload.Versions[0].Matrix.Backup[backupVersion].ImagePath,
		BackupVersion:   backupVersion,
		ProxySqlImage:   resp.Payload.Versions[0].Matrix.Proxysql[proxySqlVersion].ImagePath,
		ProxySqlVersion: proxySqlVersion,
		PMMImage:        resp.Payload.Versions[0].Matrix.Pmm[pmmVersion].ImagePath,
		PMMVersion:      pmmVersion,
		HAProxyImage:    resp.Payload.Versions[0].Matrix.Haproxy[haproxyVersion].ImagePath,
		HAProxyVersion:  haproxyVersion,
	}

	if cr.CompareVersionWith("1.7.0") >= 0 {
		logCollectorVersion, err := getVersion(resp.Payload.Versions[0].Matrix.LogCollector)
		if err != nil {
			return DepVersion{}, err
		}

		dv.LogCollectorVersion = logCollectorVersion
		dv.LogCollectorImage = resp.Payload.Versions[0].Matrix.LogCollector[logCollectorVersion].ImagePath

	}

	return dv, nil
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

type VersionService interface {
	GetExactVersion(cr *api.PerconaXtraDBCluster, endpoint string, vm versionMeta) (DepVersion, error)
}

type VersionServiceClient struct {
	OpVersion string
}

type versionMeta struct {
	Apply               string
	PXCVersion          string
	KubeVersion         string
	Platform            string
	PMMVersion          string
	BackupVersion       string
	ProxySQLVersion     string
	HAProxyVersion      string
	LogCollectorVersion string
	CRUID               string
	ClusterWideEnabled  bool
}
