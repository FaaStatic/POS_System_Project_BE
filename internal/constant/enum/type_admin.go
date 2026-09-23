package enum

import (
	"errors"
	"strings"
)

type UserRole int

const (
	superadmin UserRole = iota
	staff
)

func (typeUser UserRole) String() string {
	switch typeUser {
	case superadmin:
		return "superadmin"
	case staff:
		return "staff"
	default:
		return "unknown"
	}
}

func ParseUserRole(roleStr string) (UserRole, error) {
	switch strings.ToLower(roleStr) {
	case "superadmin":
		return superadmin, nil
	case "staff":
		return staff, nil
	default:
		return 0, errors.New("role not valid")
	}
}
