package osdetect

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type OSRelease struct {
	ID        string
	VersionID string
	Name      string
}

type SupportStatus struct {
	Supported bool
	Reason    string
}

func ParseOSRelease(r io.Reader) (OSRelease, error) {
	scanner := bufio.NewScanner(r)
	values := make(map[string]string)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[key] = cleanValue(value)
	}
	if err := scanner.Err(); err != nil {
		return OSRelease{}, err
	}

	return OSRelease{
		ID:        values["ID"],
		VersionID: values["VERSION_ID"],
		Name:      values["PRETTY_NAME"],
	}, nil
}

func CheckSupport(release OSRelease) SupportStatus {
	if release.ID == "ubuntu" && release.VersionID == "24.04" {
		return SupportStatus{Supported: true, Reason: "Ubuntu 24.04 LTS is supported"}
	}

	if release.ID == "debian" {
		return SupportStatus{Supported: false, Reason: "Debian support is planned after Ubuntu 24.04 LTS"}
	}
	if release.ID == "rocky" || release.ID == "almalinux" || release.ID == "rhel" {
		return SupportStatus{Supported: false, Reason: "RHEL-compatible support is planned after the OS adapter layer is stable"}
	}

	if release.ID == "" || release.VersionID == "" {
		return SupportStatus{Supported: false, Reason: "could not determine OS ID and VERSION_ID"}
	}

	return SupportStatus{
		Supported: false,
		Reason:    fmt.Sprintf("%s %s is not supported by the first installer release", release.ID, release.VersionID),
	}
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}
