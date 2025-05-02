package version_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	"github.com/percona/percona-xtradb-cluster-operator/pkg/naming"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/version"
)

func TestCRDVersionLabel(t *testing.T) {
	crdNames := []string{
		"perconaxtradbclusterbackups.pxc.percona.com",
		"perconaxtradbclusterrestores.pxc.percona.com",
		"perconaxtradbclusters.pxc.percona.com",
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed to get caller information")
	}
	dir := filepath.Dir(filename)
	crdPath := filepath.Join(dir, "..", "..", "deploy", "crd.yaml")

	data, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("Failed to read file: %s", err.Error())
	}
	yamlDocs := bytes.Split(data, []byte("\n---\n"))
	for _, doc := range yamlDocs {
		if len(doc) == 0 {
			continue
		}
		crd := new(v1.CustomResourceDefinition)
		if err := yaml.Unmarshal(doc, crd); err != nil {
			t.Fatalf("Failed to unmarshal crd: %s", err.Error())
		}
		if !slices.Contains(crdNames, crd.Name) {
			continue
		}
		expected := "v" + version.Version()
		if crd.Labels[naming.LabelOperatorVersion] != expected {
			t.Logf("invalid version is specified in %s label of %s CustomResourceDefinition: have: %s, expected: %s", naming.LabelOperatorVersion, crd.Name, crd.Labels[naming.LabelOperatorVersion], expected)
			t.Log([]byte(crd.Labels[naming.LabelOperatorVersion]), []byte(expected))
			t.Fail()
		}
	}
}
