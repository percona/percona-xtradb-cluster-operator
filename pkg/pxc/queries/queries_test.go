package queries

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGlobalVariableRejectsInvalidNames(t *testing.T) {
	var db Database

	for name, variable := range map[string]string{
		"injection":     "max_connections=1; DROP DATABASE mysql",
		"hyphen":        "max-connections",
		"empty":         "",
		"leading digit": "1max",
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, db.SetGlobalVariable(variable, "1"), "invalid global variable name")

			_, err := db.GlobalVariable(variable)
			require.ErrorContains(t, err, "invalid global variable name")
		})
	}
}
