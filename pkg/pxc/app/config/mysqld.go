package config

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-ini/ini"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/resource"
)

const ProviderOptionsVariable = "wsrep_provider_options"

var loosePrefix = regexp.MustCompile(`^loose_`)

type MySQLdConfig map[string]string

// ParseMySQLd merges the [mysqld] options of parts. A later part wins, also
// over the loose_ alias of the same option.
func ParseMySQLd(parts ...string) (MySQLdConfig, error) {
	result := make(MySQLdConfig)
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}

		f, err := ini.LoadSources(ini.LoadOptions{AllowBooleanKeys: true}, []byte(withMySQLdSection(part)))
		if err != nil {
			return nil, errors.Wrap(err, "load configuration")
		}

		for _, k := range f.Section("mysqld").Keys() {
			if strings.HasPrefix(k.Name(), "!") {
				continue
			}
			name := NormalizeOptionName(k.Name())
			delete(result, "loose_"+VariableName(name))
			delete(result, VariableName(name))
			result[name] = k.Value()
		}
	}
	return result, nil
}

func ParseMySQLdJSON(data string) (MySQLdConfig, error) {
	result := make(MySQLdConfig)
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, errors.Wrap(err, "unmarshal configuration")
	}
	return result, nil
}

func (c MySQLdConfig) JSON() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", errors.Wrap(err, "marshal configuration")
	}
	return string(data), nil
}

func (c MySQLdConfig) Hash() (string, error) {
	data, err := c.JSON()
	if err != nil {
		return "", err
	}
	sum := md5.Sum([]byte(data))
	return hex.EncodeToString(sum[:]), nil
}

// HasVariable reports whether the option is set, with or without loose_.
func (c MySQLdConfig) HasVariable(name string) bool {
	name = VariableName(NormalizeOptionName(name))
	_, ok := c[name]
	_, looseOK := c["loose_"+name]
	return ok || looseOK
}

// Diff returns the options of c that are new or changed since prev and the
// options of prev that c no longer has.
func (c MySQLdConfig) Diff(prev MySQLdConfig) (changed, removed []string) {
	for name, value := range c {
		if old, ok := prev[name]; !ok || old != value {
			changed = append(changed, name)
		}
	}
	for name := range prev {
		if _, ok := c[name]; !ok {
			removed = append(removed, name)
		}
	}
	slices.Sort(changed)
	slices.Sort(removed)
	return changed, removed
}

func (c MySQLdConfig) String() string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(c)) {
		b.WriteString("\n")
		b.WriteString(name)
		b.WriteString("=")
		b.WriteString(c[name])
	}
	return b.String()
}

func NormalizeOptionName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "-", "_")
}

func IsLooseOption(name string) bool {
	return loosePrefix.MatchString(name)
}

// VariableName is the system variable an option sets.
func VariableName(name string) string {
	return loosePrefix.ReplaceAllString(name, "")
}

func withMySQLdSection(part string) string {
	for line := range strings.SplitSeq(part, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			return part
		}
		break
	}
	return "[mysqld]\n" + part
}

// FormatValue renders an option value as the right-hand side of SET GLOBAL.
func FormatValue(value string) string {
	value = strings.TrimSpace(value)

	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}

	if expanded, ok := expandByteSuffix(value); ok {
		return expanded
	}

	switch strings.ToLower(value) {
	case "true", "on":
		return "ON"
	case "false", "off":
		return "OFF"
	}

	return QuoteLiteral(value)
}

// QuoteLiteral renders value as a single SQL string literal. Quotes are doubled
// rather than backslash-escaped so it holds under NO_BACKSLASH_ESCAPES too.
func QuoteLiteral(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `''`)
	return fmt.Sprintf("'%s'", escaped)
}

// ValueMatches reports whether current, as mysqld reports it, is what the
// configured option value asks for.
func ValueMatches(name, current, configured string) bool {
	if VariableName(NormalizeOptionName(name)) == ProviderOptionsVariable {
		return providerOptionsMatch(current, configured)
	}
	return scalarMatches(current, configured)
}

func scalarMatches(current, configured string) bool {
	if FormatValue(current) == FormatValue(configured) {
		return true
	}

	c, cok := canonicalBool(current)
	f, fok := canonicalBool(configured)
	return cok && fok && c == f
}

func providerOptionsMatch(current, configured string) bool {
	have := parseProviderOptions(current)
	for name, want := range parseProviderOptions(configured) {
		got, ok := have[name]
		if !ok || !scalarMatches(got, want) {
			return false
		}
	}
	return true
}

func parseProviderOptions(value string) map[string]string {
	result := make(map[string]string)
	for opt := range strings.SplitSeq(strings.Trim(strings.TrimSpace(value), `"'`), ";") {
		name, val, ok := strings.Cut(opt, "=")
		if !ok {
			continue
		}
		result[strings.TrimSpace(name)] = strings.TrimSpace(val)
	}
	return result
}

func canonicalBool(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "yes", "1":
		return "ON", true
	case "off", "false", "no", "0":
		return "OFF", true
	}
	return "", false
}

func expandByteSuffix(value string) (string, bool) {
	// Quantity parses a bare "Gi" as zero, so require a leading digit rather
	// than turning a mangled value into a silent 0.
	if value == "" || value[0] < '0' || value[0] > '9' {
		return "", false
	}

	q, err := resource.ParseQuantity(strings.ToUpper(value) + "i")
	if err != nil {
		return "", false
	}

	return strconv.FormatInt(q.Value(), 10), true
}
