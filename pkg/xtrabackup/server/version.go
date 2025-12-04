package server

import (
	"os/exec"
	"regexp"
)

func getMySQLVersionFromXtrabackup() (string, error) {
	versionOut, err := runXtrabackupVersion()
	if err != nil {
		return "", err
	}
	return parseMySQLVersionFromVersionStr(versionOut), nil
}

func runXtrabackupVersion() (string, error) {
	cmd := exec.Command("xtrabackup", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func parseMySQLVersionFromVersionStr(versionStr string) string {
	// Regex matches "MySQL server X.Y.Z" and captures X.Y.Z
	re := regexp.MustCompile(`MySQL server\s+([0-9]+\.[0-9]+\.[0-9]+)`)
	matches := re.FindStringSubmatch(versionStr)

	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
