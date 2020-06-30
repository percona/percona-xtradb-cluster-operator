package pxc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func (vs VersionServiceClient) GetExactVersion(desiredVersion, currentVersion string, versionMeta currVersionMeta) (DepVersion, error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}

	requestURL, err := url.Parse(fmt.Sprintf("%s/api/versions/v1/pxc/%s/%s", vs.URL, vs.OpVersion, desiredVersion))
	if err != nil {
		return DepVersion{}, err
	}

	q := requestURL.Query()
	q.Add("databaseVersion", currentVersion)
	q.Add("kubeVersion", versionMeta.KubeVersion)
	q.Add("platform", versionMeta.Platform)
	q.Add("customResourceUID", versionMeta.CRUID)

	if versionMeta.PMMVersion != "" {
		q.Add("pmmVersion", versionMeta.PMMVersion)
	}

	if versionMeta.BackupVersion != "" {
		q.Add("backupVersion", versionMeta.BackupVersion)
	}

	requestURL.RawQuery = q.Encode()
	req, err := http.NewRequest("GET", requestURL.String(), nil)
	if err != nil {
		return DepVersion{}, err
	}

	log.Info("FINAL URL", "URL", requestURL.String)

	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return DepVersion{}, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DepVersion{}, fmt.Errorf("received bad status code %s", resp.Status)
	}

	r := VersionResponse{}
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return DepVersion{}, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if len(r.Versions) == 0 {
		return DepVersion{}, fmt.Errorf("empty versions response")
	}

	pxcVersion, err := getVersion(r.Versions[0].Matrix.PXC)
	if err != nil {
		return DepVersion{}, err
	}

	backupVersion, err := getVersion(r.Versions[0].Matrix.Backup)
	if err != nil {
		return DepVersion{}, err
	}

	pmmVersion, err := getVersion(r.Versions[0].Matrix.PMM)
	if err != nil {
		return DepVersion{}, err
	}

	proxySqlVersion, err := getVersion(r.Versions[0].Matrix.ProxySQL)
	if err != nil {
		return DepVersion{}, err
	}

	return DepVersion{
		PXCImage:        r.Versions[0].Matrix.PXC[pxcVersion].ImagePath,
		PXCVersion:      pxcVersion,
		BackupImage:     r.Versions[0].Matrix.Backup[backupVersion].ImagePath,
		BackupVersion:   backupVersion,
		ProxySqlImage:   r.Versions[0].Matrix.ProxySQL[proxySqlVersion].ImagePath,
		ProxySqlVersion: proxySqlVersion,
		PMMImage:        r.Versions[0].Matrix.PMM[pmmVersion].ImagePath,
		PMMVersion:      pmmVersion,
	}, nil
}

func getVersion(versions map[string]Version) (string, error) {
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
	PMMImage        string `json:"pmmImage,omitempty"`
	PMMVersion      string `json:"pmmVersion,omitempty"`
}

type VersionService interface {
	GetExactVersion(desiredVersion, currentVersion string, versionMeta currVersionMeta) (DepVersion, error)
}

type VersionServiceClient struct {
	URL       string
	OpVersion string
}

type Version struct {
	Version   string `json:"version"`
	ImagePath string `json:"imagePath"`
	Imagehash string `json:"imageHash"`
	Status    string `json:"status"`
	Critilal  bool   `json:"critilal"`
}

type VersionMatrix struct {
	PXC      map[string]Version `json:"pxc"`
	PMM      map[string]Version `json:"pmm"`
	ProxySQL map[string]Version `json:"proxysql"`
	Backup   map[string]Version `json:"backup"`
}

type OperatorVersion struct {
	Operator string        `json:"operator"`
	Database string        `json:"database"`
	Matrix   VersionMatrix `json:"matrix"`
}

type VersionResponse struct {
	Versions []OperatorVersion `json:"versions"`
}

type currVersionMeta struct {
	KubeVersion   string
	Platform      string
	PMMVersion    string
	BackupVersion string
	CRUID         string
}
