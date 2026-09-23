package enum

type DebtStatus int

const (
	BELUM_LUNAS DebtStatus = iota
	LUNAS
)

func (typeItem DebtStatus) String() string {
	switch typeItem {
	case BELUM_LUNAS:
		return "Belum Lunas"
	case LUNAS:
		return "Lunas"
	default:
		return "unknown"
	}
}
