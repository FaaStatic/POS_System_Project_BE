package usecase

import (
	"errors"
	"testing"
	"time"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
)

func TestDashboardRange(t *testing.T) {
	// 23:30 UTC sudah tanggal 25 di WIB.
	now := time.Date(2026, 10, 24, 23, 30, 0, 0, time.UTC)
	str := func(s string) *string { return &s }

	from, until, err := dashboardRange(&requestdto.DashboardRequest{}, now)
	if err != nil || from.Format(dateLayout) != "2026-10-25" || until.Sub(from) != 24*time.Hour {
		t.Errorf("default range = %v..%v, %v; want the WIB day 2026-10-25", from, until, err)
	}

	from, until, _ = dashboardRange(&requestdto.DashboardRequest{DateStart: str("2026-10-01"), DateEnd: str("2026-10-31")}, now)
	if from.Format(dateLayout) != "2026-10-01" || until.Format(dateLayout) != "2026-11-01" {
		t.Errorf("explicit range = %v..%v; want end day included", from, until)
	}

	for name, req := range map[string]*requestdto.DashboardRequest{
		"end before start": {DateStart: str("2026-10-10"), DateEnd: str("2026-10-01")},
		"over a year":      {DateStart: str("2025-01-01"), DateEnd: str("2026-10-01")},
		"bad format":       {DateStart: str("01-10-2026")},
	} {
		if _, _, err := dashboardRange(req, now); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%s: err = %v; want validation error", name, err)
		}
	}
}
