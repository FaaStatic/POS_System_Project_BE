package enum

type UserRole int

const (
	Superadmin UserRole = iota
	Staff
)

func (typeUser UserRole) String() string {
	switch typeUser {
	case Superadmin:
		return "superadmin"
	case Staff:
		return "staff"
	default:
		return "unknown"
	}
}
