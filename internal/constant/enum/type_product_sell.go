package enum

import (
	"errors"
	"strings"
)

type ProductUnit int

const (
	pcs ProductUnit = iota
	gram
	kg
	liter
	kardus
	ikat
)

func (u ProductUnit) String() string {
	switch u {
	case pcs:
		return "pcs"
	case gram:
		return "gram"
	case kg:
		return "kg"
	case liter:
		return "liter"
	case kardus:
		return "kardus"
	case ikat:
		return "ikat"
	default:
		return "unknown"
	}
}

func ParseProductUnit(s string) (ProductUnit, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "pcs", "0":
		return pcs, nil
	case "gram", "1":
		return gram, nil
	case "kg", "2":
		return kg, nil
	case "liter", "3":
		return liter, nil
	case "kardus", "4":
		return kardus, nil
	case "ikat", "5":
		return ikat, nil
	default:
		return 0, errors.New("invalid unit")
	}
}
