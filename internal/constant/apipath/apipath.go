// Package apipath menyusun URL API: /api/{versi}/{group}/{service}. Versi datang dari
// konfigurasi (server.api_version), jadi semua pembentuk path memakai paket ini.
package apipath

import (
	"fmt"
	"regexp"
)

const (
	Root           = "/api"
	DefaultVersion = "v1"

	GroupAccount = "account"
	GroupCatalog = "catalog"
	GroupSales   = "sales"
	GroupInsight = "insight"
)

var versionPattern = regexp.MustCompile(`^v[0-9]+$`)

func ValidateVersion(version string) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("api version must look like v1 or v2, got %q", version)
	}
	return nil
}

// Base: "/api/v1".
func Base(version string) string {
	return Root + "/" + version
}

// Group: "/api/v1/sales".
func Group(version, group string) string {
	return Base(version) + "/" + group
}

// NotificationPath: path webhook Midtrans.
func NotificationPath(version string) string {
	return Group(version, GroupSales) + "/payments/notification"
}

// ReportsPrefix: awalan URL unduhan PDF yang dikembalikan ke klien.
func ReportsPrefix(version string) string {
	return Group(version, GroupInsight) + "/reports"
}
