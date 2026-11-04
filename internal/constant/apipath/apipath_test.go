package apipath

import "testing"

func TestValidateVersion(t *testing.T) {
	for _, v := range []string{"v1", "v2", "v10"} {
		if err := ValidateVersion(v); err != nil {
			t.Errorf("ValidateVersion(%q) = %v, want nil", v, err)
		}
	}
	for _, v := range []string{"", "V1", "1", "v", "v1.2", "v1/", " v1", "api"} {
		if err := ValidateVersion(v); err == nil {
			t.Errorf("ValidateVersion(%q) = nil, want an error", v)
		}
	}
}

func TestPaths(t *testing.T) {
	tests := []struct{ name, got, want string }{
		{"base", Base("v2"), "/api/v2"},
		{"group", Group("v1", GroupCatalog), "/api/v1/catalog"},
		{"webhook", NotificationPath("v1"), "/api/v1/sales/payments/notification"},
		{"reports", ReportsPrefix("v3"), "/api/v3/insight/reports"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
