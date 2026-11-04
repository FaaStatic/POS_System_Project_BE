package usecase

import (
	"context"
	"errors"
	"fmt"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"shop_project_be/pkg/pdf"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const dateLayout = "2006-01-02"

// debtEntriesLimit: satu hutang terbuka menampung penjualan tanpa batas, jadi rincian
// di detail hutang hanya memuat yang terbaru.
const debtEntriesLimit = 30

type debtUsecase struct {
	debtRepo  domain.DebtRepository
	storeName string
	log       *zap.Logger
}

func NewDebtUsecase(debtRepo domain.DebtRepository, storeName string, log *zap.Logger) domain.DebtUseCase {
	return &debtUsecase{
		debtRepo:  debtRepo,
		storeName: storeName,
		log:       log,
	}
}

func toDebtResponse(debt *domain.Debts) responsedto.DebtResponseDto {
	var dateDebt *string
	if !debt.DueDate.IsZero() {
		s := debt.DueDate.Format(dateLayout)
		dateDebt = &s
	}

	return responsedto.DebtResponseDto{
		DebtID:        debt.ID.String(),
		Status:        debt.Status.String(),
		CustomerID:    debt.CustomerID.String(),
		NameCustomer:  debt.Customer.Name,
		TotalDebt:     debt.TotalDebt,
		RemainingDebt: debt.RemainingDebt,
		DateDebt:      dateDebt,
	}
}

func (d *debtUsecase) AddingDebtCustomer(ctx context.Context, request *requestdto.AddDebtRequest) error {
	customerId, err := uuid.Parse(request.CustomerID)
	if err != nil {
		d.log.Error("failed to parse customer id", zap.Error(err))
		return domain.InvalidID("invalid customer id format")
	}

	userId, err := uuid.Parse(request.UserID)
	if err != nil {
		d.log.Error("failed to parse user id", zap.Error(err))
		return domain.InvalidID("invalid user id format")
	}

	var dueDate time.Time
	if request.JatuhTempo != "" {
		dueDate, err = time.Parse(dateLayout, request.JatuhTempo)
		if err != nil {
			d.log.Error("failed to parse jatuh_tempo", zap.Error(err))
			return domain.Validation("invalid jatuh_tempo format (expected YYYY-MM-DD)")
		}
	}

	debt := &domain.Debts{
		CustomerID:    customerId,
		TotalDebt:     request.TotalTransaksi,
		RemainingDebt: request.TotalTransaksi,
		Status:        enum.BELUM_LUNAS,
		DueDate:       dueDate,
	}
	if err := d.debtRepo.AddDebt(ctx, debt, userId, strings.TrimSpace(request.Note)); err != nil {
		d.log.Error("failed to add debt", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to add debt: %w", domain.ErrInternal)
		}
		return err
	}
	return nil
}

func (d *debtUsecase) GetAllDebtCustomerList(ctx context.Context, request *requestdto.FilterDebtRequest) (*responsedto.DebtListResponseDto, error) {
	filter := domain.FilterDebt{
		Limit:  request.Limit,
		Order:  request.Order,
		Search: strings.TrimSpace(request.Search),
	}
	if request.Status != nil {
		status := enum.DebtStatus(*request.Status)
		filter.Status = &status
	}
	if request.CustomerId != "" {
		customerId, err := uuid.Parse(request.CustomerId)
		if err != nil {
			d.log.Error("failed to parse customer id", zap.Error(err))
			return nil, domain.InvalidID("invalid customer id format")
		}
		filter.CustomerID = customerId
	}

	cursor, err := parseCursor(request.AfterID, request.AfterTime)
	if err != nil {
		return nil, err
	}
	filter.Cursor = cursor

	result, err := d.debtRepo.GetAllDebt(ctx, filter)
	if err != nil {
		d.log.Error("failed to get debts", zap.Error(err))
		return nil, fmt.Errorf("failed to get debts: %w", domain.ErrInternal)
	}

	responses := make([]responsedto.DebtResponseDto, 0, len(result.Data))
	for _, debt := range result.Data {
		responses = append(responses, toDebtResponse(debt))
	}

	nextId, nextTime := result.Cursor.Encode()

	return &responsedto.DebtListResponseDto{
		AfterId:         nextId,
		AfterTime:       nextTime,
		HasNext:         result.HasNext,
		TransactionList: responses,
	}, nil
}

func (d *debtUsecase) GetDebtCustomer(ctx context.Context, request *requestdto.GetDebtRequest) (*responsedto.DebtResponseDto, error) {
	id, err := uuid.Parse(request.DebtId)
	if err != nil {
		d.log.Error("failed to parse debt id", zap.Error(err))
		return nil, domain.InvalidID("invalid debt id format")
	}

	debt, err := d.debtRepo.GetDebtByID(ctx, id)
	if err != nil {
		return nil, passOrInternal(d.log, err, "failed to get debt", domain.ErrNotFound)
	}

	response := toDebtResponse(debt)
	entries, err := d.debtRepo.GetDebtEntries(ctx, id, debtEntriesLimit)
	if err != nil {
		return nil, passOrInternal(d.log, err, "failed to get debt entries", domain.ErrNotFound)
	}
	for _, e := range entries {
		response.Entries = append(response.Entries, responsedto.DebtEntryDto{
			Type: e.Type, Amount: e.Amount, Date: e.At.Format(time.RFC3339), Invoice: e.Invoice, Note: e.Note,
		})
	}
	return &response, nil
}

func (d *debtUsecase) PayDebtCash(ctx context.Context, request *requestdto.DebtPayment) (*responsedto.DebtPaymentResponse, error) {
	debtID, err := uuid.Parse(request.DebtID)
	if err != nil {
		d.log.Error("failed to parse debt id", zap.Error(err))
		return nil, domain.InvalidID("invalid debt id format")
	}
	userID, err := uuid.Parse(request.UserID)
	if err != nil {
		d.log.Error("failed to parse user id", zap.Error(err))
		return nil, domain.InvalidID("invalid user id format")
	}
	if request.NominalBayar <= 0 {
		return nil, domain.Validation("nominal_bayar must be greater than 0")
	}

	payment := &domain.DebtPayments{UserID: userID, NominalBayar: request.NominalBayar, IdempotencyKey: request.IdempotencyKey}
	result, err := d.debtRepo.PayDebt(ctx, debtID, payment)
	if err != nil {
		d.log.Error("failed to pay debt", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return nil, fmt.Errorf("failed to record debt payment: %w", domain.ErrInternal)
		}
		return nil, err
	}

	return &responsedto.DebtPaymentResponse{
		DebtId:                result.Debt.ID.String(),
		CustomerName:          result.Debt.Customer.Name,
		NominalBayar:          request.NominalBayar,
		PreviousRemainingDebt: result.PreviousRemainingDebt,
		RemainingDebt:         result.Debt.RemainingDebt,
		TotalDebt:             result.Debt.TotalDebt,
		Status:                result.Debt.Status.String(),
		PaidAt:                result.PaidAt.Format(time.RFC3339),
	}, nil
}

func (d *debtUsecase) PrintReportDebtCustomer(ctx context.Context, request *requestdto.PrintDebtReport) (*responsedto.PrintDebtCustomerResponse, error) {
	id, err := uuid.Parse(request.DebtId)
	if err != nil {
		d.log.Error("failed to parse debt id", zap.Error(err))
		return nil, domain.InvalidID("invalid debt id format")
	}

	debt, err := d.debtRepo.GetDebtWithPayments(ctx, id)
	if err != nil {
		return nil, passOrInternal(d.log, err, "failed to get debt", domain.ErrNotFound)
	}

	payments := make([]pdf.DebtPaymentRow, 0, len(debt.DebtPayments))
	for _, p := range debt.DebtPayments {
		var cashier string
		if p.User != nil {
			cashier = p.User.Username
		}
		payments = append(payments, pdf.DebtPaymentRow{
			Date:    p.TanggalBayar,
			Cashier: cashier,
			Nominal: p.NominalBayar,
		})
	}

	urlPdf, err := pdf.GenerateDebtReport(pdf.DebtReportData{
		DebtID:          debt.ID.String(),
		CustomerName:    debt.Customer.Name,
		CustomerPhone:   debt.Customer.Phone,
		CustomerAddress: debt.Customer.Address,
		TotalDebt:       debt.TotalDebt,
		RemainingDebt:   debt.RemainingDebt,
		Status:          debt.Status.String(),
		DueDate:         debt.DueDate,
		Payments:        payments,
		GeneratedAt:     time.Now(),
	})
	if err != nil {
		d.log.Error("failed to generate debt report pdf", zap.Error(err))
		return nil, fmt.Errorf("failed to generate report pdf: %w", domain.ErrInternal)
	}

	return &responsedto.PrintDebtCustomerResponse{
		CustomerName: debt.Customer.Name,
		DebtId:       debt.ID.String(),
		UrlPdf:       urlPdf,
	}, nil
}

// truncate memotong teks dari klien (nomor struk) supaya tidak meluber di PDF.
func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// paymentRemaining: sisa hutang sesudah cicilan, tidak pernah negatif.
func paymentRemaining(p *domain.DebtPayments) int64 {
	return max(0, p.PreviousRemainingDebt-p.NominalBayar)
}

func paymentParties(p *domain.DebtPayments) (customer, cashier string) {
	if p.Debt != nil {
		customer = p.Debt.Customer.Name
	}
	if p.User != nil {
		cashier = p.User.Username
	}
	return customer, cashier
}

func (d *debtUsecase) PrintDebtPaymentReceipt(ctx context.Context, request *requestdto.PrintDebtPaymentReceipt) (*responsedto.PrintDebtPaymentResponse, error) {
	id, err := uuid.Parse(request.ID)
	if err != nil {
		return nil, domain.InvalidID("invalid debt payment id format")
	}
	payment, err := d.debtRepo.GetDebtPaymentByID(ctx, id)
	if err != nil {
		return nil, passOrInternal(d.log, err, "failed to get debt payment", domain.ErrNotFound)
	}
	customer, cashier := paymentParties(payment)
	number := ""
	if payment.IdempotencyKey != nil {
		number = *payment.IdempotencyKey
	}
	urlPdf, err := pdf.GenerateDebtPaymentReceipt(pdf.DebtPaymentReceiptData{
		StoreName:         d.storeName,
		Number:            truncate(number, 40),
		Customer:          customer,
		Cashier:           cashier,
		PaidAt:            payment.TanggalBayar,
		PreviousRemaining: payment.PreviousRemainingDebt,
		Paid:              payment.NominalBayar,
		Remaining:         paymentRemaining(payment),
		GeneratedAt:       time.Now(),
	})
	if err != nil {
		d.log.Error("failed to generate debt payment receipt pdf", zap.Error(err))
		return nil, fmt.Errorf("failed to generate report pdf: %w", domain.ErrInternal)
	}
	return &responsedto.PrintDebtPaymentResponse{ID: payment.ID.String(), UrlPdf: urlPdf}, nil
}

func (d *debtUsecase) PrintDebtPaymentsMonth(ctx context.Context, request *requestdto.PrintDebtPaymentsMonth) (*responsedto.PrintDebtPaymentsMonthResponse, error) {
	now := time.Now().In(domain.StoreLocation)
	month, year := request.Month, request.Year
	if month == 0 {
		month = int(now.Month())
	}
	if year == 0 {
		year = now.Year()
	}
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, domain.StoreLocation)
	payments, err := d.debtRepo.GetDebtPaymentsInRange(ctx, from, from.AddDate(0, 1, 0))
	if err != nil {
		return nil, passOrInternal(d.log, err, "failed to get debt payments", domain.ErrInternal)
	}
	rows := make([]pdf.DebtPaymentsMonthRow, 0, len(payments))
	for _, p := range payments {
		customer, cashier := paymentParties(p)
		rows = append(rows, pdf.DebtPaymentsMonthRow{
			Customer:          customer,
			Cashier:           cashier,
			PaidAt:            p.TanggalBayar,
			PreviousRemaining: p.PreviousRemainingDebt,
			Paid:              p.NominalBayar,
			Remaining:         paymentRemaining(p),
		})
	}
	urlPdf, err := pdf.GenerateDebtPaymentsMonth(pdf.DebtPaymentsMonthData{
		StoreName:   d.storeName,
		Month:       month,
		Year:        year,
		Rows:        rows,
		GeneratedAt: time.Now(),
	})
	if err != nil {
		d.log.Error("failed to generate debt payments month pdf", zap.Error(err))
		return nil, fmt.Errorf("failed to generate report pdf: %w", domain.ErrInternal)
	}
	return &responsedto.PrintDebtPaymentsMonthResponse{
		Month:  fmt.Sprintf("%02d", month),
		Year:   strconv.Itoa(year),
		Count:  len(rows),
		UrlPdf: urlPdf,
	}, nil
}

func (d *debtUsecase) GetAllDebtPaymentList(ctx context.Context, request *requestdto.FilterDebtPaymentRequest) (*responsedto.DebtPaymentListResponseDto, error) {
	filter := domain.FilterDebtPayment{
		Limit:  request.Limit,
		Order:  request.Order,
		Search: strings.TrimSpace(request.Search),
	}
	if request.CustomerId != "" {
		customerId, err := uuid.Parse(request.CustomerId)
		if err != nil {
			d.log.Error("failed to parse customer id", zap.Error(err))
			return nil, domain.InvalidID("invalid customer id format")
		}
		filter.CustomerID = customerId
	}
	cursor, err := parseCursor(request.AfterID, request.AfterTime)
	if err != nil {
		return nil, err
	}
	filter.Cursor = cursor

	result, err := d.debtRepo.GetAllDebtPayments(ctx, filter)
	if err != nil {
		d.log.Error("failed to get debt payments", zap.Error(err))
		return nil, fmt.Errorf("failed to get debt payments: %w", domain.ErrInternal)
	}

	items := make([]responsedto.DebtPaymentItemDto, 0, len(result.Data))
	for _, p := range result.Data {
		remaining := paymentRemaining(p)
		status := enum.BELUM_LUNAS
		if remaining == 0 {
			status = enum.LUNAS
		}
		item := responsedto.DebtPaymentItemDto{
			ID:                    p.ID.String(),
			DebtID:                p.DebtID.String(),
			NominalBayar:          p.NominalBayar,
			PreviousRemainingDebt: p.PreviousRemainingDebt,
			RemainingDebt:         remaining,
			Status:                status.String(),
			PaidAt:                p.TanggalBayar.Format(time.RFC3339),
		}
		if p.IdempotencyKey != nil {
			item.ClientRef = *p.IdempotencyKey
		}
		if p.User != nil {
			item.Cashier = p.User.Username
		}
		if p.Debt != nil {
			item.CustomerID = p.Debt.CustomerID.String()
			item.CustomerName = p.Debt.Customer.Name
		}
		items = append(items, item)
	}

	nextId, nextTime := result.Cursor.Encode()
	return &responsedto.DebtPaymentListResponseDto{
		AfterId:     nextId,
		AfterTime:   nextTime,
		HasNext:     result.HasNext,
		PaymentList: items,
	}, nil
}
