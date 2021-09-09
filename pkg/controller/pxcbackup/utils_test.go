package pxcbackup

import (
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

type finalizer string

func (finalizer) Generate(r *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(finalizer(strconv.FormatInt(r.Int63(), 2)))
}

func TestRemoveStringsFromSlice(t *testing.T) {
	t.Run("empty cases", func(t *testing.T) {
		t.Parallel()

		checkCases(t, "", []string{"", "0"})
	})

	t.Run("simple cases", func(t *testing.T) {
		t.Parallel()

		checkCases(t, "a1", []string{"a1", "00a00100"})
	})

	t.Run("random values", func(t *testing.T) {
		t.Parallel()

		test := func(f finalizer) bool {
			got := removeZeroFromString(string(f))
			return !strings.Contains(got, "0")
		}

		if err := quick.Check(test, nil); err != nil {
			t.Error(err)
		}
	})
}

func checkCases(t *testing.T, want string, cases []string) {
	t.Helper()

	for _, str := range cases {
		got := removeZeroFromString(str)
		if got != want {
			t.Errorf("want %v, got %v", want, got)
		}
	}
}

func removeZeroFromString(str string) string {
	seq := strings.Split(str, "")
	seq = removeStringsFromSlice(seq, "0")
	return strings.Join(seq, "")
}
