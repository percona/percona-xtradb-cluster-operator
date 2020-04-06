package manager

import (
	"testing"
)

func TestTbaleNameCheck(t *testing.T) {
	cases := []struct {
		name     string
		desiered bool
	}{
		{
			name:     "test_tableName1",
			desiered: true,
		},
		{
			name:     "test;DROP",
			desiered: false,
		},
		{
			name:     "'`",
			desiered: false,
		},
		{
			name:     "table-name",
			desiered: false,
		},
	}
	for _, c := range cases {
		if tableNameCorrect(c.name) != c.desiered {
			t.Errorf("Table name '%s' test failed", c.name)
		}
	}
}

func TestGetPrivilegeString(t *testing.T) {
	cases := []struct {
		privileges string
		desiered   string
		correct    bool
	}{
		{
			privileges: "all privileges",
			desiered:   "ALL PRIVILEGES",
			correct:    true,
		},
		{
			privileges: "Select,delete",
			desiered:   "SELECT, DELETE",
			correct:    true,
		},
		{
			privileges: "Update,  INSERT ",
			desiered:   "UPDATE, INSERT",
			correct:    true,
		},
		{
			privileges: "Updateeee, INSRT",
			correct:    false,
		},
	}
	for _, c := range cases {
		priv, err := getPrivilegesString(c.privileges)
		if err != nil && c.correct {
			t.Error(err)
		}
		if priv != c.desiered {
			t.Errorf("Privileges '%s' test failed", c.privileges)
		}
	}

}
