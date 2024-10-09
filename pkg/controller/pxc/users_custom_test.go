package pxc

import (
	"reflect"
	"testing"

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
		// {
		// 	name: "no DBs or hosts set",
		// 	user: &api.User{
		// 		Name:   "test",
		// 		Grants: []string{"SELECT"},
		// 	},
		// 	pass: "password",
		// 	expected: []string{
		// 		"CREATE USER IF NOT EXISTS 'test'@'%' IDENTIFIED BY 'password'",
		// 		"GRANT SELECT ON *.* TO 'test'@'%' ",
		// 	},
		// },
		// {
		// 	name: "DBs set but no hosts",
		// 	user: &api.User{
		// 		Name:   "test",
		// 		DBs:    []string{"db1", "db2"},
		// 		Grants: []string{"SELECT, INSERT"},
		// 	},
		// 	pass: "pass1",
		// 	expected: []string{
		// 		"CREATE DATABASE IF NOT EXISTS db1",
		// 		"CREATE DATABASE IF NOT EXISTS db2",
		// 		"CREATE USER IF NOT EXISTS 'test'@'%' IDENTIFIED BY 'pass1'",
		// 		"GRANT SELECT, INSERT ON db1.* TO 'test'@'%' ",
		// 		"GRANT SELECT, INSERT ON db2.* TO 'test'@'%' ",
		// 	},
		// },
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
		name     string
		crUser   *api.User
		dbUser   []users.User
		expected bool
	}{
		{
			name: "no users in DB",
			crUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			dbUser:   []users.User{},
			expected: true,
		},
		{
			name: "host the same",
			crUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			dbUser: []users.User{
				{
					Name: "test",
					Host: "host1",
				},
				{
					Name: "test",
					Host: "host2",
				},
			},
			expected: false,
		},
		{
			name: "host number not the same",
			crUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			dbUser: []users.User{
				{
					Name: "test",
					Host: "host1",
				},
			},
			expected: true,
		},
		// {
		// 	name: "hosts don't match by number",
		// 	crUser: &api.User{
		// 		Name:  "test",
		// 		Hosts: []string{"host1", "host2"},
		// 	},
		// 	dbUser: []users.User{
		// 		{
		// 			Name: "test",
		// 			Host: "host1",
		// 		},
		// 	},
		// 	expected: false,
		// },
		{
			name: "hosts don't match by content",
			crUser: &api.User{
				Name:  "test",
				Hosts: []string{"host1", "host2"},
			},
			dbUser: []users.User{
				{
					Name: "test",
					Host: "host1",
				},
				{
					Name: "test",
					Host: "host222",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := userChanged(tt.dbUser, tt.crUser)
			if actual != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}
