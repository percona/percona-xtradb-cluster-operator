package pxc

import (
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

func TestUpsertUserQuery(t *testing.T) {
	var tests = []struct {
		name     string
		user     *api.User
		pass     string
		expected []string
	}{
		{
			name: "Hosts set but no DBs",
			user: &api.User{
				Name:   "test",
				Hosts:  []string{"host1", "host2"},
				Grants: []string{"SELECT, INSERT"},
			},
			pass: "pass1",
			expected: []string{
				"CREATE USER IF NOT EXISTS 'test'@'host1' IDENTIFIED BY 'pass1'",
				"GRANT SELECT, INSERT ON *.* TO 'test'@'host1' ",
				"CREATE USER IF NOT EXISTS 'test'@'host2' IDENTIFIED BY 'pass1'",
				"GRANT SELECT, INSERT ON *.* TO 'test'@'host2' ",
			},
		},
		{
			name: "DBs and hosts set",
			user: &api.User{
				Name:   "test",
				Hosts:  []string{"host1", "host2"},
				DBs:    []string{"db1", "db2"},
				Grants: []string{"SELECT, INSERT"},
			},
			pass: "pass1",
			expected: []string{
				"CREATE DATABASE IF NOT EXISTS db1",
				"CREATE DATABASE IF NOT EXISTS db2",
				"CREATE USER IF NOT EXISTS 'test'@'host1' IDENTIFIED BY 'pass1'",
				"GRANT SELECT, INSERT ON db1.* TO 'test'@'host1' ",
				"GRANT SELECT, INSERT ON db2.* TO 'test'@'host1' ",
				"CREATE USER IF NOT EXISTS 'test'@'host2' IDENTIFIED BY 'pass1'",
				"GRANT SELECT, INSERT ON db1.* TO 'test'@'host2' ",
				"GRANT SELECT, INSERT ON db2.* TO 'test'@'host2' ",
			},
		},
		{
			name: "DBs and hosts set with grants and grant option",
			user: &api.User{
				Name:            "test",
				Hosts:           []string{"host1", "host2"},
				DBs:             []string{"db1", "db2"},
				Grants:          []string{"SELECT, INSERT"},
				WithGrantOption: true,
			},
			pass: "pass1",
			expected: []string{
				"CREATE DATABASE IF NOT EXISTS db1",
				"CREATE DATABASE IF NOT EXISTS db2",
				"CREATE USER IF NOT EXISTS 'test'@'host1' IDENTIFIED BY 'pass1'",
				"GRANT SELECT, INSERT ON db1.* TO 'test'@'host1' WITH GRANT OPTION",
				"GRANT SELECT, INSERT ON db2.* TO 'test'@'host1' WITH GRANT OPTION",
				"CREATE USER IF NOT EXISTS 'test'@'host2' IDENTIFIED BY 'pass1'",
				"GRANT SELECT, INSERT ON db1.* TO 'test'@'host2' WITH GRANT OPTION",
				"GRANT SELECT, INSERT ON db2.* TO 'test'@'host2' WITH GRANT OPTION",
			},
		},
		{
			name: "DBs and hosts set with no grants",
			user: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
				DBs:   []string{"db1", "db2"},
			},
			pass: "pass1",
			expected: []string{
				"CREATE DATABASE IF NOT EXISTS db1",
				"CREATE DATABASE IF NOT EXISTS db2",
				"CREATE USER IF NOT EXISTS 'test'@'host1' IDENTIFIED BY 'pass1'",
				"CREATE USER IF NOT EXISTS 'test'@'host2' IDENTIFIED BY 'pass1'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := upsertUserQuery(tt.user, tt.pass)
			if !reflect.DeepEqual(actual, tt.expected) {
				t.Fatalf("expected %s, got %s", tt.expected, actual)
			}
		})
	}
}

func TestAlterUserQuery(t *testing.T) {
	var tests = []struct {
		name     string
		user     *api.User
		pass     string
		expected []string
	}{
		{
			name: "no hosts set",
			user: &api.User{
				Name: "test",
			},
			pass:     "password",
			expected: []string{"ALTER USER 'test'@'%' IDENTIFIED BY 'password'"},
		},
		{
			name: "hosts set",
			user: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			pass: "pass1",
			expected: []string{
				"ALTER USER 'test'@'host1' IDENTIFIED BY 'pass1'",
				"ALTER USER 'test'@'host2' IDENTIFIED BY 'pass1'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := alterUserQuery(tt.user, tt.pass)
			if !reflect.DeepEqual(actual, tt.expected) {
				t.Fatalf("expected %s, got %s", tt.expected, actual)
			}
		})
	}
}

func TestUserChanged(t *testing.T) {
	var tests = []struct {
		name        string
		desiredUser *api.User
		currentUser *users.User
		expected    bool
	}{
		{
			name: "no users in DB",
			desiredUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			currentUser: nil,
			expected:    true,
		},
		{
			name: "host the same",
			desiredUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			currentUser: &users.User{
				Name:  "test",
				Hosts: sets.New("host1", "host2"),
			},
			expected: false,
		},
		{
			name: "host number not the same",
			desiredUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			currentUser: &users.User{
				Name:  "test",
				Hosts: sets.New("host1"),
			},
			expected: true,
		},
		{
			name: "hosts don't match by content",
			desiredUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			currentUser: &users.User{
				Name:  "test",
				Hosts: sets.New("host1", "host2222"),
			},
			expected: true,
		},
		{
			name: "dbs the same",
			desiredUser: &api.User{
				Name: "test",
				DBs:  []string{"db1", "db2"},
			},
			currentUser: &users.User{
				Name: "test",
				DBs:  sets.New("db1", "db2"),
			},
			expected: false,
		},
		{
			name: "db number not the same",
			desiredUser: &api.User{
				Name: "test",
				DBs:  []string{"db1", "db2"},
			},
			currentUser: &users.User{
				Name: "test",
				DBs:  sets.New("db1"),
			},
			expected: true,
		},
		{
			name: "dbs don't match by content",
			desiredUser: &api.User{
				Name: "test",
				DBs:  []string{"db1", "db2"},
			},
			currentUser: &users.User{
				Name: "test",
				DBs:  sets.New("db1", "db2222"),
			},
			expected: true,
		},
		{
			name: "grants the same with same number of hosts and DBs specified",
			desiredUser: &api.User{
				Name:            "test",
				Hosts:           []string{"host1", "host2"},
				DBs:             []string{"db1", "db2"},
				Grants:          []string{"SELECT", "INSERT"},
				WithGrantOption: true,
			},
			currentUser: &users.User{
				Name:  "test",
				DBs:   sets.New("db1", "db2"),
				Hosts: sets.New("host1", "host2"),
				Grants: map[string][]string{
					"host1": {
						"GRANT USAGE ON *.* TO `test`@`host1`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host1` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host1` WITH GRANT OPTION",
					},
					"host2": {
						"GRANT USAGE ON *.* TO `test`@`host2`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host2` WITH GRANT OPTION",
					},
				},
			},
			expected: false,
		},
		{
			name: "grants the same with more DBs then hosts specified",
			desiredUser: &api.User{
				Name:            "test",
				Hosts:           []string{"host1", "host2"},
				DBs:             []string{"db1", "db2", "db3"},
				Grants:          []string{"SELECT", "INSERT"},
				WithGrantOption: true,
			},
			currentUser: &users.User{
				Name:  "test",
				DBs:   sets.New("db1", "db2", "db3"),
				Hosts: sets.New("host1", "host2"),
				Grants: map[string][]string{
					"host1": {
						"GRANT USAGE ON *.* TO `test`@`host1`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host1` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host1` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db3`.* TO `test`@`host1` WITH GRANT OPTION",
					},
					"host2": {
						"GRANT USAGE ON *.* TO `test`@`host2`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db3`.* TO `test`@`host2` WITH GRANT OPTION",
					},
				},
			},
			expected: false,
		},
		{
			name: "grants the same with more hosts then DBs specified",
			desiredUser: &api.User{
				Name:            "test",
				Hosts:           []string{"host1", "host2", "host3"},
				DBs:             []string{"db1", "db2"},
				Grants:          []string{"SELECT", "INSERT"},
				WithGrantOption: true,
			},
			currentUser: &users.User{
				Name:  "test",
				DBs:   sets.New("db1", "db2"),
				Hosts: sets.New("host1", "host2", "host3"),
				Grants: map[string][]string{
					"host1": {
						"GRANT USAGE ON *.* TO `test`@`host1`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host1` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host1` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db3`.* TO `test`@`host1` WITH GRANT OPTION",
					},
					"host2": {
						"GRANT USAGE ON *.* TO `test`@`host2`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db3`.* TO `test`@`host2` WITH GRANT OPTION",
					},
					"host3": {
						"GRANT USAGE ON *.* TO `test`@`host3`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host3` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host3` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db3`.* TO `test`@`host3` WITH GRANT OPTION",
					},
				},
			},
			expected: false,
		},
		{
			name: "grants the same with more privileges then specified",
			desiredUser: &api.User{
				Name:            "test",
				Hosts:           []string{"host1"},
				DBs:             []string{"db1"},
				Grants:          []string{"SELECT", "INSERT"},
				WithGrantOption: true,
			},
			currentUser: &users.User{
				Name:  "test",
				DBs:   sets.New("db1"),
				Hosts: sets.New("host1"),
				Grants: map[string][]string{
					"host1": {
						"GRANT USAGE ON *.* TO `test`@`host1`",
						"GRANT SELECT, INSERT, UPDATE ON `db1`.* TO `test`@`host1` WITH GRANT OPTION",
					},
				},
			},
			expected: false,
		},
		{
			name: "grants for user host missing",
			desiredUser: &api.User{
				Name:            "test",
				Hosts:           []string{"host1", "host2"},
				DBs:             []string{"db1", "db2"},
				Grants:          []string{"SELECT", "INSERT"},
				WithGrantOption: true,
			},
			currentUser: &users.User{
				Name:  "test",
				DBs:   sets.New("db1", "db2", "db3"),
				Hosts: sets.New("host1", "host2"),
				Grants: map[string][]string{
					"host2": {
						"GRANT USAGE ON *.* TO `test`@`host2`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db2`.* TO `test`@`host2` WITH GRANT OPTION",
					},
				},
			},
			expected: true,
		},
		{
			name: "grants for DB missing",
			desiredUser: &api.User{
				Name:            "test",
				Hosts:           []string{"host1", "host2"},
				DBs:             []string{"db1", "db2"},
				Grants:          []string{"SELECT", "INSERT"},
				WithGrantOption: true,
			},
			currentUser: &users.User{
				Name:  "test",
				DBs:   sets.New("db1", "db2", "db3"),
				Hosts: sets.New("host1", "host2"),
				Grants: map[string][]string{
					"host2": {
						"GRANT USAGE ON *.* TO `test`@`host2`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT, INSERT ON `db88`.* TO `test`@`host2` WITH GRANT OPTION",
					},
				},
			},
			expected: true,
		},
		{
			name: "grants for privileges missing",
			desiredUser: &api.User{
				Name:            "test",
				Hosts:           []string{"host1", "host2"},
				DBs:             []string{"db1", "db2"},
				Grants:          []string{"SELECT", "INSERT"},
				WithGrantOption: true,
			},
			currentUser: &users.User{
				Name:  "test",
				DBs:   sets.New("db1", "db2", "db3"),
				Hosts: sets.New("host1", "host2"),
				Grants: map[string][]string{
					"host2": {
						"GRANT USAGE ON *.* TO `test`@`host2`",
						"GRANT SELECT, INSERT ON `db1`.* TO `test`@`host2` WITH GRANT OPTION",
						"GRANT SELECT ON `db2`.* TO `test`@`host2` WITH GRANT OPTION",
					},
				},
			},
			expected: true,
		},
	}

	log := logr.Discard()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := userChanged(tt.currentUser, tt.desiredUser, log)
			if actual != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}
