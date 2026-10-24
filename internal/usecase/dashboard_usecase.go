package usecase

import (
	"context"
	"fmt"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	"time"

	"go.uber.org/zap"
)

// maxDashboardDays membatasi rentang supaya satu request tidak memindai
// seluruh riwayat penjualan.
const maxDashboardDays = 366

type dashboardUsecase struct {
	repo domain.DashboardRepository
	log  *zap.Logger
}

func NewDashboardUsecase(repo domain.DashboardRepository, log *zap.Logger) domain.DashboardUsecase {
	return &dashboardUsecase{repo: repo, log: log}
}

// Summary: tanpa tanggal berarti hari ini (WIB). date_end ikut dihitung penuh.
func (d *dashboardUsecase) Summary(ctx context.Context, request *requestdto.DashboardRequest) (*domain.DashboardSummary, error) {
	from, until, err := dashboardRange(request, time.Now())
	if err != nil {
		return nil, err
	}

	summary, err := d.repo.Summary(ctx, from, until)
	if err != nil {
		d.log.Error("failed to build dashboard summary", zap.Error(err))
		return nil, fmt.Errorf("failed to build dashboard: %w", domain.ErrInternal)
	}
	summary.DateStart = from.Format(dateLayout)
	summary.DateEnd = until.AddDate(0, 0, -1).Format(dateLayout)
	for i := range summary.PaymentMethods {
		summary.PaymentMethods[i].Method = enum.MoneyPayment(summary.PaymentMethods[i].PaymentType).String()
	}
	return summary, nil
}

func dashboardRange(request *requestdto.DashboardRequest, now time.Time) (time.Time, time.Time, error) {
	start, err := parseStoreDay(request.DateStart, "date_start")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseStoreDay(request.DateEnd, "date_end")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	today := now.In(domain.StoreLocation)
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, domain.StoreLocation)
	if start == nil {
		start = &todayStart
	}
	if end == nil {
		end = &todayStart
	}
	if end.Before(*start) {
		return time.Time{}, time.Time{}, domain.Validation("date_end must not be before date_start")
	}
	until := end.AddDate(0, 0, 1)
	if until.Sub(*start) > maxDashboardDays*24*time.Hour {
		return time.Time{}, time.Time{}, domain.Validation(fmt.Sprintf("date range must not exceed %d days", maxDashboardDays))
	}
	return *start, until, nil
}
