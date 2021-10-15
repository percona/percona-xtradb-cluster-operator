/*
Copyright 2021 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package json

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestDecode_KnownFields(t *testing.T) {
	// We ignore type meta since inline only works with k8s yaml parsers
	cases := []struct {
		name  string
		input string

		want    fixture
		wantErr bool
	}{{
		name:  "no metadata",
		input: `{}`,
		want:  fixture{},
	}, {
		name:  "known fields",
		input: `{ "metadata":{"name":"some-name", "namespace":"some-namespace"} }`,
		want: fixture{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-name",
				Namespace: "some-namespace",
			},
		},
	}, {
		name: "with spec",
		input: `{
            "metadata":{"name":"some-name", "namespace":"some-namespace"},
            "spec":{"key":"value"}
        }`,
		want: fixture{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-name",
				Namespace: "some-namespace",
			},
			Spec: map[string]string{"key": "value"},
		},
	}, {
		name: "unknown metadata field",
		input: `{
            "metadata":{"name":"some-name", "namespace":"some-namespace", "bomba":"boom"},
            "spec":{"key":"value"}
        }`,
		want: fixture{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-name",
				Namespace: "some-namespace",
			},
			Spec: map[string]string{"key": "value"},
		},
	}, {
		name: "unknown field top level",
		input: `{
            "metadata":{"name":"some-name", "namespace":"some-namespace"},
            "bomba": "boom",
            "spec":{"key":"value"}
        }`,
		wantErr: true,
	}, {
		name: "multiple metadata keys in the JSON",
		input: `{
            "metadata":{"name":"some-name", "namespace":"some-namespace", "bomba":"boom"},
            "spec":{"metadata":"value"}
        }`,
		want: fixture{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-name",
				Namespace: "some-namespace",
			},
			Spec: map[string]string{"metadata": "value"},
		},
	}, {
		name: "nested object in metadata",
		input: `{
            "metadata":{"name":"some-name", "namespace":"some-namespace", "labels":{"key":"value"}}
        }`,
		want: fixture{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-name",
				Namespace: "some-namespace",
				Labels: map[string]string{
					"key": "value",
				},
			},
		},
	}, {
		name: "nested array in metadata",
		input: `{
            "metadata":{"name":"some-name", "namespace":"some-namespace", "finalizers":["first","second"]}
        }`,
		want: fixture{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "some-name",
				Namespace:  "some-namespace",
				Finalizers: []string{"first", "second"},
			},
		},
	}, {
		name: "bad input",
		// note use two characters so our decoder fails on the second token lookup
		input:   "{{",
		wantErr: true,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := &fixture{}
			err := Decode([]byte(tc.input), got, true)

			if tc.wantErr && err == nil {
				t.Fatal("DecodeTo() expected an error")
			} else if !tc.wantErr && err != nil {
				t.Fatal("unexpected error", err)
			}

			// Don't bother checking against the fixture if
			// we expected an error
			if tc.wantErr {
				return
			}

			if diff := cmp.Diff(&tc.want, got); diff != "" {
				t.Error("unexpected diff", diff)
			}
		})
	}
}

func TestDecode_AllowUnknownFields(t *testing.T) {
	input := `{
            "metadata":{"name":"some-name", "namespace":"some-namespace"},
            "bomba": "boom",
            "spec": {"some":"value"}
        }`

	got := &fixture{}
	want := &fixture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-name",
			Namespace: "some-namespace",
		},
		Spec: map[string]string{"some": "value"},
	}

	err := Decode([]byte(input), got, false)
	if err != nil {
		t.Fatal("unexpected error", err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("unexpected diff", diff)
	}
}

// Note: this test is paired with the failingFixture and knows
// that the implementation of Decode parses the json in two passes
// the first being with an empty metadata '{}' - the second being the real
// input
func TestDecode_UnmarshalMetadataFailed(t *testing.T) {
	input := `{ "metadata":{"name":"some-name", "namespace":"some-namespace"} }`
	err := Decode([]byte(input), &failingFixture{}, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

type fixture struct {
	// Our decoder doesn't support `inline` that's a sig.k8s.io/yaml feature
	// So we skip parsing this property
	metav1.TypeMeta   `json:"-"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              map[string]string `json:"spec"`
}

func (f *fixture) DeepCopyObject() runtime.Object {
	panic("not implemented")
}

func (f *fixture) SetDefaults(context.Context) {
	panic("not implemented")
}

type failingFixture struct {
	fixture
	Meta failingMeta `json:"metadata"`
}

type failingMeta struct {
	metav1.ObjectMeta
}

func (f *failingMeta) UnmarshalJSON(bites []byte) error {
	if bytes.Equal([]byte("{}"), bites) {
		return nil
	}
	return errors.New("bomba")
}
