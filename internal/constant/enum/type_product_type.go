package enum

import (
	"errors"
	"strings"
)

type ProductType int

const (
	Physical ProductType = iota
	Digital
)

func (p ProductType) String() string {
	switch p {
	case Physical:
		return "physical"
	case Digital:
		return "digital"
	default:
		return "unknown"
	}
}

func (p ProductType) IsDigital() bool { return p == Digital }

func ParseProductType(s string) (ProductType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "physical", "0":
		return Physical, nil
	case "digital", "1":
		return Digital, nil
	default:
		return 0, errors.New("invalid product type")
	}
}
