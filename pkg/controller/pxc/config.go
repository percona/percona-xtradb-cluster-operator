package pxc

import (
	"context"
	_ "embed"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/k8s"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/app/config"
)

//go:embed fluentbit_template.conf
var fluentbitTemplate string

func (r *ReconcilePerconaXtraDBCluster) reconcileConfigMaps(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	result := controllerutil.OperationResultNone

	res, err := r.reconcileAutotuneConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile autotune config")
	}
	result = res

	res, err = r.reconcileCustomConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile custom config")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	res, err = r.reconcileProxySQLConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile proxysql config map")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	res, err = r.reconcileHAProxyConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile haproxy config map")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	res, err = r.reconcileLogcollectorConfigMap(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile logcollector config map")
	}
	if result == controllerutil.OperationResultNone {
		result = res
	}

	_, err = r.reconcileHookScriptConfigMaps(ctx, cr)
	if err != nil {
		return result, errors.Wrap(err, "reconcile hookscript config maps")
	}

	return result, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileAutotuneConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	autotuneCm := config.AutoTuneConfigMapName(cr.Name, "pxc")

	_, ok := cr.Spec.PXC.Resources.Limits[corev1.ResourceMemory]
	if !ok {
		err := deleteConfigMapIfExists(ctx, r.client, cr, autotuneCm)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete configmap")
	}

	configMap, err := config.NewAutoTuneConfigMap(cr, cr.Spec.PXC.Resources.Limits.Memory(), autotuneCm)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "new configmap")
	}

	err = k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update configmap")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileCustomConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	pxcConfigName := config.CustomConfigMapName(cr.Name, "pxc")

	if cr.Spec.PXC.Configuration == "" {
		err := deleteConfigMapIfExists(ctx, r.client, cr, pxcConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	configMap := config.NewConfigMap(cr, pxcConfigName, "init.cnf", cr.Spec.PXC.Configuration)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileHookScriptConfigMaps(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	pxcHookScriptName := config.HookScriptConfigMapName(cr.Name, "pxc")
	if cr.Spec.PXC != nil && cr.Spec.PXC.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.PXC.HookScript, pxcHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create pxc hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, pxcHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete pxc hookscript config map")
		}
	}

	proxysqlHookScriptName := config.HookScriptConfigMapName(cr.Name, "proxysql")
	if cr.Spec.ProxySQL != nil && cr.Spec.ProxySQL.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.ProxySQL.HookScript, proxysqlHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create proxysql hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, proxysqlHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete proxysql hookscript config map")
		}
	}

	haproxyHookScriptName := config.HookScriptConfigMapName(cr.Name, "haproxy")
	if cr.Spec.HAProxy != nil && cr.Spec.HAProxy.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.HAProxy.HookScript, haproxyHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create haproxy hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, haproxyHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete haproxy config map")
		}
	}

	logCollectorHookScriptName := config.HookScriptConfigMapName(cr.Name, "logcollector")
	if cr.Spec.LogCollector != nil && cr.Spec.LogCollector.HookScript != "" {
		err := r.createHookScriptConfigMap(ctx, cr, cr.Spec.LogCollector.HookScript, logCollectorHookScriptName)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "create logcollector hookscript config map")
		}
	} else {
		if err := deleteConfigMapIfExists(ctx, r.client, cr, logCollectorHookScriptName); err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "delete logcollector config map")
		}
	}

	return controllerutil.OperationResultNone, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileProxySQLConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	proxysqlConfigName := config.CustomConfigMapName(cr.Name, "proxysql")

	if !cr.Spec.ProxySQLEnabled() || cr.Spec.ProxySQL.Configuration == "" {
		err := deleteConfigMapIfExists(ctx, r.client, cr, proxysqlConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	configMap := config.NewConfigMap(cr, proxysqlConfigName, "proxysql.cnf", cr.Spec.ProxySQL.Configuration)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileHAProxyConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	haproxyConfigName := config.CustomConfigMapName(cr.Name, "haproxy")

	if !cr.HAProxyEnabled() || cr.Spec.HAProxy.Configuration == "" {
		err := deleteConfigMapIfExists(ctx, r.client, cr, haproxyConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	configMap := config.NewConfigMap(cr, haproxyConfigName, "haproxy-global.cfg", cr.Spec.HAProxy.Configuration)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

func (r *ReconcilePerconaXtraDBCluster) reconcileLogcollectorConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster) (controllerutil.OperationResult, error) {
	logCollectorConfigName := config.CustomConfigMapName(cr.Name, "logcollector")

	if cr.Spec.LogCollector == nil || !cr.Spec.LogCollector.Enabled {
		err := deleteConfigMapIfExists(ctx, r.client, cr, logCollectorConfigName)
		return controllerutil.OperationResultNone, errors.Wrap(err, "delete config map")
	}

	// Check if the ConfigMap already exists and has the correct content
	existingConfigMap := &corev1.ConfigMap{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      logCollectorConfigName,
		Namespace: cr.Namespace,
	}, existingConfigMap)

	if err == nil {
		// ConfigMap exists, check if we need to update it
		// Generate the expected configuration
		fluentBitConfig, err := r.generateFluentBitConfig(cr)
		if err != nil {
			return controllerutil.OperationResultNone, errors.Wrap(err, "generate Fluent-bit configuration")
		}

		expectedConfig := r.generateCompleteFluentBitConfig(fluentBitConfig)

		// Check if the existing ConfigMap has the same content
		if existingConfigMap.Data != nil && existingConfigMap.Data["fluentbit_pxc.conf"] == expectedConfig {
			// ConfigMap already has the correct content, no need to update
			return controllerutil.OperationResultNone, nil
		}
	}

	// Generate Fluent-bit configuration with buffer settings
	fluentBitConfig, err := r.generateFluentBitConfig(cr)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "generate Fluent-bit configuration")
	}

	// Create a complete configuration that overrides the default Docker configuration
	// We create fluentbit_pxc.conf to replace the default Docker configuration
	completeConfig := r.generateCompleteFluentBitConfig(fluentBitConfig)
	configMap := config.NewConfigMap(cr, logCollectorConfigName, "fluentbit_pxc.conf", completeConfig)

	err = k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return controllerutil.OperationResultNone, errors.Wrap(err, "set controller ref")
	}

	res, err := createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return res, errors.Wrap(err, "create or update config map")
	}

	return res, nil
}

// generateFluentBitConfig generates Fluent-bit configuration using a hybrid approach:
// 1. Always start with the template as the base configuration
// 2. If custom configuration is provided, merge it with the template (custom takes precedence)
// 3. Apply buffer settings to the merged configuration
// 4. If template cannot be loaded, use minimal configuration with custom config merged
// Based on https://github.com/percona/percona-docker/blob/main/fluentbit/dockerdir/etc/fluentbit/fluentbit_pxc.conf
func (r *ReconcilePerconaXtraDBCluster) generateFluentBitConfig(cr *api.PerconaXtraDBCluster) (string, error) {
	if cr.Spec.LogCollector == nil || !cr.Spec.LogCollector.Enabled {
		return "", nil
	}

	var baseConfig string
	var err error

	// Always start with the template as the base
	baseConfig, err = r.getFluentBitTemplate()
	if err != nil {
		// If we can't load the template, use a minimal configuration with buffer settings
		logf.Log.Info("Could not load Fluent-bit template, using minimal configuration with buffer settings", "error", err)
		baseConfig = r.getMinimalFluentBitConfig(cr)
	}

	// If custom configuration is provided, merge it with the base template
	if cr.Spec.LogCollector.Configuration != "" {
		baseConfig = r.mergeCustomConfigurationWithTemplate(baseConfig, cr.Spec.LogCollector.Configuration)
	}

	// Apply version-based environment variable names and buffer settings
	configWithVersionFix := r.applyVersionBasedEnvironmentVariables(baseConfig, cr)
	return r.applyBufferSettingsToTemplate(configWithVersionFix, cr.Spec.LogCollector.FluentBitBufferSettings), nil
}

// mergeCustomConfigurationWithTemplate merges custom configuration with the template
// Uses smart merging to prevent duplicate sections and handle user overrides
// Sections are grouped by unique identifiers and merged intelligently
func (r *ReconcilePerconaXtraDBCluster) mergeCustomConfigurationWithTemplate(templateConfig, customConfig string) string {
	if customConfig == "" {
		return templateConfig
	}

	// Parse both configurations into structured sections
	templateSections := r.parseStructuredSections(strings.Split(templateConfig, "\n"))
	customSections := r.parseStructuredSections(strings.Split(customConfig, "\n"))

	// Start with template sections
	resultSections := make(map[string]map[string][]string)
	for sectionType, sections := range templateSections {
		resultSections[sectionType] = make(map[string][]string)
		for identifier, sectionLines := range sections {
			resultSections[sectionType][identifier] = sectionLines
		}
	}

	// Merge custom sections intelligently
	for sectionType, sections := range customSections {
		if resultSections[sectionType] == nil {
			resultSections[sectionType] = make(map[string][]string)
		}

		for identifier, customSectionLines := range sections {
			if existingLines, exists := resultSections[sectionType][identifier]; exists {
				// Merge existing section with custom section
				resultSections[sectionType][identifier] = r.mergeSectionSettings(existingLines, customSectionLines)
			} else {
				// Add new section with normalized indentation
				resultSections[sectionType][identifier] = r.normalizeIndentation(customSectionLines)
			}
		}
	}

	// Build the final configuration
	var result []string

	// Add sections in order: SERVICE, INPUT, OUTPUT, others
	sectionOrder := []string{"SERVICE", "INPUT", "OUTPUT"}

	for _, sectionType := range sectionOrder {
		if sections, exists := resultSections[sectionType]; exists {
			// Sort section identifiers to ensure deterministic order
			var identifiers []string
			for identifier := range sections {
				identifiers = append(identifiers, identifier)
			}
			sort.Strings(identifiers)

			for _, identifier := range identifiers {
				result = append(result, sections[identifier]...)
			}
			delete(resultSections, sectionType)
		}
	}

	// Add any remaining sections in deterministic order
	var remainingSectionTypes []string
	for sectionType := range resultSections {
		remainingSectionTypes = append(remainingSectionTypes, sectionType)
	}
	sort.Strings(remainingSectionTypes)

	for _, sectionType := range remainingSectionTypes {
		sections := resultSections[sectionType]
		// Sort section identifiers to ensure deterministic order
		var identifiers []string
		for identifier := range sections {
			identifiers = append(identifiers, identifier)
		}
		sort.Strings(identifiers)

		for _, identifier := range identifiers {
			result = append(result, sections[identifier]...)
		}
	}

	return strings.Join(result, "\n")
}

// normalizeIndentation normalizes indentation in configuration lines to use 4 spaces
func (r *ReconcilePerconaXtraDBCluster) normalizeIndentation(lines []string) []string {
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			result = append(result, "")
			continue
		}

		// If line starts with [ and ends with ], it's a section header - no indentation
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			result = append(result, trimmed)
			continue
		}

		// All other lines should have 4 spaces indentation
		result = append(result, "    "+trimmed)
	}
	return result
}

// parseStructuredSections parses configuration into structured sections grouped by unique identifiers
// Returns map[sectionType]map[identifier]sectionLines
func (r *ReconcilePerconaXtraDBCluster) parseStructuredSections(lines []string) map[string]map[string][]string {
	sections := make(map[string]map[string][]string)
	var currentSectionType string
	var currentIdentifier string
	var currentSectionLines []string

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			if currentSectionType != "" {
				currentSectionLines = append(currentSectionLines, line)
			}
			continue
		}

		// Check if this line starts a new section
		if strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]") {
			// Save previous section if exists
			if currentSectionType != "" && currentIdentifier != "" {
				if sections[currentSectionType] == nil {
					sections[currentSectionType] = make(map[string][]string)
				}
				sections[currentSectionType][currentIdentifier] = currentSectionLines
			}

			// Start new section
			sectionName := trimmedLine
			currentSectionType = r.getSectionType(sectionName)
			currentIdentifier = r.getSectionIdentifier(sectionName, lines, i)
			currentSectionLines = []string{line}
		} else if currentSectionType != "" {
			// Add line to current section
			currentSectionLines = append(currentSectionLines, line)
		}
	}

	// Save last section
	if currentSectionType != "" && currentIdentifier != "" {
		if sections[currentSectionType] == nil {
			sections[currentSectionType] = make(map[string][]string)
		}
		sections[currentSectionType][currentIdentifier] = currentSectionLines
	}

	return sections
}

// getSectionType extracts the section type from section name
func (r *ReconcilePerconaXtraDBCluster) getSectionType(sectionName string) string {
	switch sectionName {
	case "[SERVICE]":
		return "SERVICE"
	case "[INPUT]":
		return "INPUT"
	case "[OUTPUT]":
		return "OUTPUT"
	case "[FILTER]":
		return "FILTER"
	case "[PARSER]":
		return "PARSER"
	default:
		return "OTHER"
	}
}

// getSectionIdentifier creates a unique identifier for a section based on its key properties
func (r *ReconcilePerconaXtraDBCluster) getSectionIdentifier(sectionName string, allLines []string, startIndex int) string {
	// For SERVICE section, there's only one
	if sectionName == "[SERVICE]" {
		return "service"
	}

	// For INPUT sections, use Path as identifier
	if sectionName == "[INPUT]" {
		for i := startIndex + 1; i < len(allLines); i++ {
			line := strings.TrimSpace(allLines[i])
			if strings.HasPrefix(line, "Path") {
				// Extract the path value
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					return "input_" + parts[1]
				}
			}
			// Stop at next section
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				break
			}
		}
		return "input_unknown"
	}

	// For OUTPUT sections, use Name + Match as identifier
	if sectionName == "[OUTPUT]" {
		var name, match string
		for i := startIndex + 1; i < len(allLines); i++ {
			line := strings.TrimSpace(allLines[i])
			if strings.HasPrefix(line, "Name") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					name = parts[1]
				}
			}
			if strings.HasPrefix(line, "Match") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					match = parts[1]
				}
			}
			// Stop at next section
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				break
			}
		}
		return "output_" + name + "_" + match
	}

	// For other sections, use section name
	return strings.ToLower(strings.Trim(sectionName, "[]"))
}

// mergeSectionSettings merges two sections, with custom settings overriding template settings
func (r *ReconcilePerconaXtraDBCluster) mergeSectionSettings(templateLines, customLines []string) []string {
	// Parse template settings
	templateSettings := r.parseSectionSettings(templateLines)

	// Normalize custom lines indentation before parsing
	normalizedCustomLines := r.normalizeIndentation(customLines)

	// Parse custom settings
	customSettings := r.parseSectionSettings(normalizedCustomLines)

	// Merge settings (custom overrides template)
	mergedSettings := make(map[string]string)
	for key, value := range templateSettings {
		mergedSettings[key] = value
	}
	for key, value := range customSettings {
		mergedSettings[key] = value
	}

	// Build merged section
	var result []string
	result = append(result, templateLines[0]) // Section header

	// Add all settings in a consistent order
	settingOrder := []string{"Name", "Path", "Tag", "Match", "Host", "Port", "Index", "Type", "Format", "Refresh_Interval", "DB", "multiline.parser", "read_from_head", "Path_Key", "Mem_Buf_Limit", "Buffer_Chunk_Size", "Buffer_Max_Size", "json_date_key"}

	for _, key := range settingOrder {
		if value, exists := mergedSettings[key]; exists {
			result = append(result, "    "+key+" "+value)
		}
	}

	// Add any remaining settings not in the predefined order
	// Sort keys to ensure deterministic output
	var remainingKeys []string
	for key := range mergedSettings {
		found := false
		for _, orderedKey := range settingOrder {
			if key == orderedKey {
				found = true
				break
			}
		}
		if !found {
			remainingKeys = append(remainingKeys, key)
		}
	}
	sort.Strings(remainingKeys)
	for _, key := range remainingKeys {
		result = append(result, "    "+key+" "+mergedSettings[key])
	}

	return result
}

// parseSectionSettings parses key-value pairs from section lines
func (r *ReconcilePerconaXtraDBCluster) parseSectionSettings(lines []string) map[string]string {
	settings := make(map[string]string)

	for _, line := range lines[1:] { // Skip section header
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		// Parse key-value pairs
		parts := strings.Fields(trimmedLine)
		if len(parts) >= 2 {
			key := parts[0]
			value := strings.Join(parts[1:], " ")
			settings[key] = value
		}
	}

	return settings
}

// parseConfigurationSections parses configuration lines and groups them by section
// For sections that can have multiple instances (like [INPUT] and [OUTPUT]),
// we concatenate all instances into a single section
func (r *ReconcilePerconaXtraDBCluster) parseConfigurationSections(lines []string) map[string][]string {
	sections := make(map[string][]string)
	var currentSection string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			if currentSection != "" {
				sections[currentSection] = append(sections[currentSection], line)
			}
			continue
		}

		// Check if this line starts a new section
		if strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]") {
			currentSection = trimmedLine
			// If this section already exists, append to it (for multiple [INPUT] or [OUTPUT] sections)
			if _, exists := sections[currentSection]; !exists {
				sections[currentSection] = []string{}
			}
			sections[currentSection] = append(sections[currentSection], line)
		} else if currentSection != "" {
			// Add line to current section
			sections[currentSection] = append(sections[currentSection], line)
		}
	}

	return sections
}

// generateCompleteFluentBitConfig generates a complete Fluent-bit configuration
// that overrides the default Docker configuration completely
func (r *ReconcilePerconaXtraDBCluster) generateCompleteFluentBitConfig(customConfig string) string {
	// Create a complete configuration that includes our custom configuration
	// This will completely override the default Docker configuration
	return customConfig
}

// getFluentBitTemplate returns the base Fluent-bit configuration template
// This uses the embedded fluentbit_template.conf file
// The template matches the official Percona Docker fluentbit_pxc.conf configuration
func (r *ReconcilePerconaXtraDBCluster) getFluentBitTemplate() (string, error) {
	// Use the embedded template file
	if fluentbitTemplate != "" {
		return fluentbitTemplate, nil
	}

	// Fallback: try to read from the config directory relative to the current working directory
	configPath := "config/fluentbit_template.conf"
	if _, err := os.Stat(configPath); err == nil {
		if content, err := ioutil.ReadFile(configPath); err == nil {
			return string(content), nil
		}
	}

	// Fallback: try to find the file relative to the executable
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		fallbackPath := filepath.Join(exeDir, "..", "config", "fluentbit_template.conf")
		if content, err := ioutil.ReadFile(fallbackPath); err == nil {
			return string(content), nil
		}
	}

	// Return error if template file cannot be found
	return "", errors.New("fluentbit_template.conf not found in config directory")
}

// getMinimalFluentBitConfig returns a minimal Fluent-bit configuration
// This is used as a fallback when the template file cannot be loaded
// It includes the essential configuration for MySQL log processing with buffer settings
func (r *ReconcilePerconaXtraDBCluster) getMinimalFluentBitConfig(cr *api.PerconaXtraDBCluster) string {
	// Use correct POD_NAMESPACE for CR version >= 1.19.0, otherwise use POD_NAMESPASE for backward compatibility
	podNamespaceVar := "POD_NAMESPASE"
	if cr.CompareVersionWith("1.19.0") >= 0 {
		podNamespaceVar = "POD_NAMESPACE"
	}

	return fmt.Sprintf(`[SERVICE]
    Flush        1
    Log_Level    error
    Daemon       off
    parsers_file parsers_multiline.conf

[INPUT]
    Name        tail
    Path        ${LOG_DATA_DIR}/mysqld-error.log
    Tag         ${%s}.${POD_NAME}.mysqld-error.log
    Mem_Buf_Limit 5MB
    Refresh_Interval 5
    DB          /tmp/flb_kube.db
    multiline.parser multiline-regex-test
    read_from_head true
    Path_Key    file

[INPUT]
    Name        tail
    Path        ${LOG_DATA_DIR}/wsrep_recovery_verbose.log
    Tag         ${%s}.${POD_NAME}.wsrep_recovery_verbose.log
    Mem_Buf_Limit 5MB
    Refresh_Interval 5
    DB          /tmp/flb_kube.db
    multiline.parser multiline-regex-test
    read_from_head true
    Path_Key    file

[INPUT]
    Name        tail
    Path        ${LOG_DATA_DIR}/innobackup.prepare.log
    Tag         ${%s}.${POD_NAME}.innobackup.prepare.log
    Refresh_Interval 5
    DB          /tmp/flb_kube.db
    multiline.parser multiline-regex-test
    read_from_head true
    Path_Key    file

[INPUT]
    Name        tail
    Path        ${LOG_DATA_DIR}/innobackup.move.log
    Tag         ${%s}.${POD_NAME}.innobackup.move.log
    Refresh_Interval 5
    DB          /tmp/flb_kube.db
    multiline.parser multiline-regex-test
    read_from_head true
    Path_Key    file

[INPUT]
    Name        tail
    Path        ${LOG_DATA_DIR}/innobackup.backup.log
    Tag         ${%s}.${POD_NAME}.innobackup.backup.log
    Refresh_Interval 5
    DB          /tmp/flb_kube.db
    multiline.parser multiline-regex-test
    read_from_head true
    Path_Key    file

[INPUT]
    Name        tail
    Path        ${LOG_DATA_DIR}/mysqld.post.processing.log
    Tag         ${%s}.${POD_NAME}.mysqld.post.processing.log
    Refresh_Interval 5
    DB          /tmp/flb_kube.db
    multiline.parser multiline-regex-test
    read_from_head true
    Path_Key    file

[OUTPUT]
    Name             stdout
    Match            *
    Format           json_lines
    json_date_key    false

[OUTPUT]
    Name file
    Match ${%s}.${POD_NAME}.innobackup.prepare.log
    File innobackup.prepare.full.log
    Path ${LOG_DATA_DIR}/

[OUTPUT]
    Name file
    Match ${%s}.${POD_NAME}.innobackup.move.log
    File innobackup.move.full.log
    Path ${LOG_DATA_DIR}/

[OUTPUT]
    Name file
    Match ${%s}.${POD_NAME}.innobackup.backup.log
    File innobackup.backup.full.log
    Path ${LOG_DATA_DIR}/

[OUTPUT]
    Name file
    Match ${%s}.${POD_NAME}.mysqld.post.processing.log
    File mysqld.post.processing.full.log
    Path ${LOG_DATA_DIR}/`, podNamespaceVar, podNamespaceVar, podNamespaceVar, podNamespaceVar, podNamespaceVar, podNamespaceVar, podNamespaceVar, podNamespaceVar, podNamespaceVar, podNamespaceVar)
}

// applyVersionBasedEnvironmentVariables applies version-based environment variable names to the configuration
// For CR version >= 1.19.0, uses POD_NAMESPACE (correct spelling)
// For older versions, uses POD_NAMESPASE (backward compatibility)
func (r *ReconcilePerconaXtraDBCluster) applyVersionBasedEnvironmentVariables(config string, cr *api.PerconaXtraDBCluster) string {
	// Use correct POD_NAMESPACE for CR version >= 1.19.0, otherwise use POD_NAMESPASE for backward compatibility
	oldVar := "POD_NAMESPASE"
	newVar := "POD_NAMESPACE"

	if cr.CompareVersionWith("1.19.0") >= 0 {
		// Replace POD_NAMESPASE with POD_NAMESPACE for newer versions
		return strings.ReplaceAll(config, "${"+oldVar+"}", "${"+newVar+"}")
	}

	// For older versions, keep POD_NAMESPASE as is
	return config
}

// applyBufferSettingsToTemplate applies buffer settings to tail input plugins only
// This function injects Buffer_Chunk_Size, Buffer_Max_Size, and updates Mem_Buf_Limit in [INPUT] sections that use the tail plugin
// It works with both template-based and custom configurations
func (r *ReconcilePerconaXtraDBCluster) applyBufferSettingsToTemplate(template string, bufferSettings *api.FluentBitBufferSettings) string {
	if bufferSettings == nil {
		return template
	}

	// Split the template into lines for processing
	lines := strings.Split(template, "\n")
	var result []string
	inInputSection := false
	currentInputStart := 0
	isTailPlugin := false

	for _, line := range lines {
		result = append(result, line)

		// Check if we're entering an [INPUT] section
		if strings.TrimSpace(line) == "[INPUT]" {
			// If we were in a previous INPUT section, add buffer settings to it if it was a tail plugin
			if inInputSection && isTailPlugin {
				r.addBufferSettingsToInputSection(&result, currentInputStart, bufferSettings)
			}
			inInputSection = true
			isTailPlugin = false // Reset for new section
			currentInputStart = len(result) - 1
			continue
		}

		// Check if this is a tail plugin
		if inInputSection && strings.Contains(line, "Name") && strings.Contains(line, "tail") {
			isTailPlugin = true
		}

		// Check if we're leaving an [INPUT] section (next section)
		if inInputSection && strings.HasPrefix(strings.TrimSpace(line), "[") {
			// We're at the end of an [INPUT] section, add buffer settings if it was a tail plugin
			if isTailPlugin {
				r.addBufferSettingsToInputSection(&result, currentInputStart, bufferSettings)
			}
			inInputSection = false
			isTailPlugin = false
		}

		// Update Mem_Buf_Limit if present and this is a tail plugin
		if inInputSection && isTailPlugin && strings.Contains(line, "Mem_Buf_Limit") && bufferSettings.MemBufLimit != "" {
			// Replace the existing Mem_Buf_Limit with the configured value
			result[len(result)-1] = "    Mem_Buf_Limit " + bufferSettings.MemBufLimit
		}
	}

	// Handle the last INPUT section if we're still in one at the end
	if inInputSection && isTailPlugin {
		r.addBufferSettingsToInputSection(&result, currentInputStart, bufferSettings)
	}

	return strings.Join(result, "\n")
}

// addBufferSettingsToInputSection adds buffer settings to a specific INPUT section
func (r *ReconcilePerconaXtraDBCluster) addBufferSettingsToInputSection(result *[]string, inputStart int, bufferSettings *api.FluentBitBufferSettings) {
	// Find the end of this INPUT section (next [SECTION] or end of array)
	inputEnd := len(*result)
	for i := inputStart + 1; i < len(*result); i++ {
		line := strings.TrimSpace((*result)[i])
		// Stop at next section or empty line that might indicate end of section
		if strings.HasPrefix(line, "[") || line == "" {
			inputEnd = i
			break
		}
	}

	// Get the lines for this INPUT section
	currentInputLines := (*result)[inputStart:inputEnd]

	// Find the last non-empty line in the INPUT section to insert buffer settings after it
	lastInputLine := inputStart
	for i := inputEnd - 1; i > inputStart; i-- {
		if strings.TrimSpace((*result)[i]) != "" {
			lastInputLine = i
			break
		}
	}

	// Insert buffer settings after the last line of the INPUT section
	insertIndex := lastInputLine + 1

	// Add buffer settings if not already present
	if bufferSettings.BufferChunkSize != "" && !r.hasBufferSetting(currentInputLines, "Buffer_Chunk_Size") {
		// Insert Buffer_Chunk_Size after the last line of the INPUT section
		newLine := "    Buffer_Chunk_Size " + bufferSettings.BufferChunkSize
		*result = append((*result)[:insertIndex], append([]string{newLine}, (*result)[insertIndex:]...)...)
		insertIndex++ // Adjust for the inserted line
	}
	if bufferSettings.BufferMaxSize != "" && !r.hasBufferSetting(currentInputLines, "Buffer_Max_Size") {
		// Insert Buffer_Max_Size after the last line of the INPUT section
		newLine := "    Buffer_Max_Size " + bufferSettings.BufferMaxSize
		*result = append((*result)[:insertIndex], append([]string{newLine}, (*result)[insertIndex:]...)...)
	}
}

// hasBufferSetting checks if a buffer setting is already present in the configuration
func (r *ReconcilePerconaXtraDBCluster) hasBufferSetting(lines []string, setting string) bool {
	for _, line := range lines {
		if strings.Contains(line, setting) {
			return true
		}
	}
	return false
}

func (r *ReconcilePerconaXtraDBCluster) createHookScriptConfigMap(ctx context.Context, cr *api.PerconaXtraDBCluster, hookScript string, configMapName string) error {
	configMap := config.NewConfigMap(cr, configMapName, "hook.sh", hookScript)

	err := k8s.SetControllerReference(cr, configMap, r.scheme)
	if err != nil {
		return errors.Wrap(err, "set controller ref")
	}

	_, err = createOrUpdateConfigmap(ctx, r.client, configMap)
	if err != nil {
		return errors.Wrap(err, "create or update configmap")
	}

	return nil
}

func createOrUpdateConfigmap(ctx context.Context, cl client.Client, configMap *corev1.ConfigMap) (controllerutil.OperationResult, error) {
	log := logf.FromContext(ctx)

	nn := types.NamespacedName{Namespace: configMap.Namespace, Name: configMap.Name}

	currMap := &corev1.ConfigMap{}
	err := cl.Get(ctx, nn, currMap)
	if err != nil && !k8serrors.IsNotFound(err) {
		return controllerutil.OperationResultNone, errors.Wrap(err, "get current configmap")
	}

	if k8serrors.IsNotFound(err) {
		log.V(1).Info("Creating object", "object", configMap.Name, "kind", configMap.GetObjectKind())
		return controllerutil.OperationResultCreated, cl.Create(ctx, configMap)
	}

	if !reflect.DeepEqual(currMap.Data, configMap.Data) {
		log.V(1).Info("Updating object", "object", configMap.Name, "kind", configMap.GetObjectKind())
		err = k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			cm := &corev1.ConfigMap{}

			err := cl.Get(ctx, nn, cm)
			if err != nil {
				return err
			}

			cm.Data = configMap.Data

			return cl.Update(ctx, cm)
		})
		return controllerutil.OperationResultUpdated, err
	}

	return controllerutil.OperationResultNone, nil
}

func deleteConfigMapIfExists(ctx context.Context, cl client.Client, cr *api.PerconaXtraDBCluster, cmName string) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{}

	err := cl.Get(ctx, types.NamespacedName{Namespace: cr.Namespace, Name: cmName}, configMap)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err, "get config map")
	}

	if k8serrors.IsNotFound(err) {
		return nil
	}

	if !metav1.IsControlledBy(configMap, cr) {
		return nil
	}

	log.V(1).Info("Deleting object", "object", configMap.Name, "kind", configMap.GetObjectKind())
	return cl.Delete(ctx, configMap)
}
