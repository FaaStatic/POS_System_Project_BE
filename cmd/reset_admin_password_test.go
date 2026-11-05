package cmd

import "testing"

func TestValidateNewPassword(t *testing.T) {
	tests := []struct {
		name              string
		password, confirm string
		wantErr           bool
	}{
		{"matching and long enough", "secret1", "secret1", false},
		{"exactly six", "abcdef", "abcdef", false},
		{"mismatch", "secret1", "secret2", true},
		{"too short", "abc", "abc", true},
		{"empty", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateNewPassword(tc.password, tc.confirm); (err != nil) != tc.wantErr {
				t.Errorf("err = %v; wantErr %v", err, tc.wantErr)
			}
		})
	}
}
