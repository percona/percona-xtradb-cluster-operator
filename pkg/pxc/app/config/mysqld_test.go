package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMySQLd(t *testing.T) {
	tests := map[string]struct {
		parts      []string
		want       MySQLdConfig
		wantErrMsg string
	}{
		"no parts": {
			want: MySQLdConfig{},
		},
		"blank parts are skipped": {
			parts: []string{"", "  \n"},
			want:  MySQLdConfig{},
		},
		"part without a section header is mysqld": {
			parts: []string{"\n# tuned\nmax_connections=100"},
			want:  MySQLdConfig{"max_connections": "100"},
		},
		"only the mysqld section is read": {
			parts: []string{"[client]\nuser=root\n[mysqld]\nmax_connections = 100"},
			want:  MySQLdConfig{"max_connections": "100"},
		},
		"hyphens are normalized": {
			parts: []string{"[mysqld]\nwsrep-provider-options=gcache.size=1G"},
			want:  MySQLdConfig{"wsrep_provider_options": "gcache.size=1G"},
		},
		"later part wins": {
			parts: []string{"[mysqld]\nmax_connections=100\nsync_binlog=1", "[mysqld]\nmax_connections=200"},
			want:  MySQLdConfig{"max_connections": "200", "sync_binlog": "1"},
		},
		"later loose alias replaces the option": {
			parts: []string{"[mysqld]\nmax_connections=100", "[mysqld]\nloose_max_connections=200"},
			want:  MySQLdConfig{"loose_max_connections": "200"},
		},
		"later option replaces the loose alias": {
			parts: []string{"[mysqld]\nloose-max-connections=100", "[mysqld]\nmax_connections=200"},
			want:  MySQLdConfig{"max_connections": "200"},
		},
		"boolean keys are on": {
			parts: []string{"[mysqld]\nskip-name-resolve"},
			want:  MySQLdConfig{"skip_name_resolve": "true"},
		},
		"include directives are skipped": {
			parts: []string{"[mysqld]\n!includedir /etc/mysql/conf.d/\nmax_connections=100"},
			want:  MySQLdConfig{"max_connections": "100"},
		},
		"malformed section": {
			parts:      []string{"[mysqld\nmax_connections=100"},
			wantErrMsg: "load configuration",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ParseMySQLd(tc.parts...)
			if tc.wantErrMsg != "" {
				require.ErrorContains(t, err, tc.wantErrMsg)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMySQLdConfigJSON(t *testing.T) {
	conf := MySQLdConfig{"max_connections": "100", "wsrep_provider_options": "gcache.size=1G;gcs.fc_limit=16"}

	data, err := conf.JSON()
	require.NoError(t, err)

	got, err := ParseMySQLdJSON(data)
	require.NoError(t, err)
	assert.Equal(t, conf, got)

	_, err = ParseMySQLdJSON("not json")
	require.ErrorContains(t, err, "unmarshal configuration")
}

func TestMySQLdConfigHash(t *testing.T) {
	a, err := MySQLdConfig{"a": "1", "b": "2"}.Hash()
	require.NoError(t, err)
	b, err := MySQLdConfig{"b": "2", "a": "1"}.Hash()
	require.NoError(t, err)
	c, err := MySQLdConfig{"a": "1", "b": "3"}.Hash()
	require.NoError(t, err)

	assert.Equal(t, a, b)
	assert.NotEqual(t, a, c)
}

func TestMySQLdConfigString(t *testing.T) {
	assert.Equal(t, "\na=1\nb=2", MySQLdConfig{"b": "2", "a": "1"}.String())
	assert.Empty(t, MySQLdConfig{}.String())
}

func TestMySQLdConfigHasVariable(t *testing.T) {
	conf := MySQLdConfig{"max_connections": "100", "loose_sync_binlog": "1"}

	tests := map[string]struct {
		name string
		want bool
	}{
		"plain":                   {name: "max_connections", want: true},
		"hyphenated":              {name: "max-connections", want: true},
		"loose spelling of plain": {name: "loose_max_connections", want: true},
		"plain spelling of loose": {name: "sync_binlog", want: true},
		"missing":                 {name: "thread_cache_size"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, conf.HasVariable(tc.name))
		})
	}
}

func TestMySQLdConfigDiff(t *testing.T) {
	tests := map[string]struct {
		current     MySQLdConfig
		prev        MySQLdConfig
		wantChanged []string
		wantRemoved []string
	}{
		"identical": {
			current: MySQLdConfig{"a": "1"},
			prev:    MySQLdConfig{"a": "1"},
		},
		"added, changed and removed": {
			current:     MySQLdConfig{"a": "2", "b": "1", "c": "1"},
			prev:        MySQLdConfig{"a": "1", "c": "1", "d": "1"},
			wantChanged: []string{"a", "b"},
			wantRemoved: []string{"d"},
		},
		"everything is new against an empty record": {
			current:     MySQLdConfig{"b": "1", "a": "1"},
			prev:        MySQLdConfig{},
			wantChanged: []string{"a", "b"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			changed, removed := tc.current.Diff(tc.prev)
			assert.Equal(t, tc.wantChanged, changed)
			assert.Equal(t, tc.wantRemoved, removed)
		})
	}
}

func TestOptionNames(t *testing.T) {
	assert.Equal(t, "wsrep_provider_options", NormalizeOptionName(" wsrep-provider-options "))
	assert.True(t, IsLooseOption("loose_audit_log_file"))
	assert.False(t, IsLooseOption("audit_log_file"))
	assert.Equal(t, "audit_log_file", VariableName("loose_audit_log_file"))
	assert.Equal(t, "audit_log_file", VariableName("audit_log_file"))
}

func TestFormatValue(t *testing.T) {
	tests := map[string]struct {
		value string
		want  string
	}{
		"integer":         {value: "134217728", want: "134217728"},
		"float":           {value: " 0.5 ", want: "0.5"},
		"byte suffix":     {value: "128M", want: "134217728"},
		"lower suffix":    {value: "1g", want: "1073741824"},
		"true":            {value: "true", want: "ON"},
		"on":              {value: "On", want: "ON"},
		"false":           {value: "FALSE", want: "OFF"},
		"off":             {value: "off", want: "OFF"},
		"string":          {value: "O_DIRECT", want: "'O_DIRECT'"},
		"quote escaped":   {value: "it's", want: "'it''s'"},
		"backslash":       {value: `a\b`, want: `'a\\b'`},
		"bare suffix":     {value: "G", want: "'G'"},
		"not a quantity":  {value: "1x", want: "'1x'"},
		"provider string": {value: "gcache.size=1G;gcs.fc_limit=16", want: "'gcache.size=1G;gcs.fc_limit=16'"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, FormatValue(tc.value))
		})
	}
}

func TestValueMatches(t *testing.T) {
	tests := map[string]struct {
		name       string
		current    string
		configured string
		want       bool
	}{
		"same number": {
			name: "innodb_purge_threads", current: "4", configured: "4", want: true,
		},
		"different number": {
			name: "innodb_purge_threads", current: "4", configured: "8",
		},
		"byte suffix against bytes": {
			name: "innodb_buffer_pool_chunk_size", current: "134217728", configured: "128M", want: true,
		},
		"numeric bool": {
			name: "innodb_numa_interleave", current: "OFF", configured: "0", want: true,
		},
		"word bool": {
			name: "innodb_adaptive_hash_index", current: "OFF", configured: "False", want: true,
		},
		"bool mismatch": {
			name: "innodb_numa_interleave", current: "ON", configured: "0",
		},
		"string": {
			name: "innodb_flush_method", current: "O_DIRECT", configured: "O_DIRECT", want: true,
		},
		"loose name": {
			name: "loose_innodb_flush_method", current: "fsync", configured: "O_DIRECT",
		},
		"provider options held among others": {
			name:       "wsrep_provider_options",
			current:    "base_port = 4567; gcache.recover = yes; gcache.size = 171533074; gcs.fc_limit = 73; pc.recovery = true",
			configured: "gcache.recover=yes;gcache.size=171533074;gcs.fc_limit=73;pc.recovery=1;",
			want:       true,
		},
		"provider options with suffix": {
			name:       "wsrep-provider-options",
			current:    "gcache.size = 134217728",
			configured: `"gcache.size=128M"`,
			want:       true,
		},
		"provider option differs": {
			name:       "wsrep_provider_options",
			current:    "gcache.size = 128M; gcs.fc_limit = 16",
			configured: "gcache.size=128M;gcs.fc_limit=73",
		},
		"provider option missing": {
			name:       "wsrep_provider_options",
			current:    "gcache.size = 128M",
			configured: "gcache.size=128M;evs.send_window=589",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, ValueMatches(tc.name, tc.current, tc.configured))
		})
	}
}
