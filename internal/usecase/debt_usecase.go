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
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const dateLayout = "2006-01-02"

type debtUsecase struct {
	debtRepo domain.DebtRepository
	log      *zap.Logger
}

func NewDebtUsecase(debtRepo domain.DebtRepository, log *zap.Logger) domain.DebtUseCase {
	return &debtUsecase{
		debtRepo: debtRepo,
		log:      log,
	}
}

func toDebtResponse(debt *domain.Debts) responsedto.DebtResponseDto {
	var dateDebt *string
	if !debt.DueDate.IsZero() {
		s := debt.DueDate.Format(dateLayout)
		dateDebt = &s
	}

	transactions := make([]responsedto.TransactionResponse, 0, len(debt.Transactions))
	for _, trx := range debt.Transactions {
		transactions = append(transactions, responsedto.TransactionResponse{
			InvoiceNumber:    trx.NoInvoice,
			PaymentType:      int(trx.PaymentType),
			TotalTransaction: trx.TotalTransaction,
			CreatedAt:        trx.CreatedAt.Format(time.RFC3339),
		})
	}

	return responsedto.DebtResponseDto{
		CustomerID:      debt.CustomerID.String(),
		NameCustomer:    debt.Customer.Name,
		TotalDebt:       debt.TotalDebt,
		RemainingDebt:   debt.RemainingDebt,
		DateDebt:        dateDebt,
		TransactionList: transactions,
	}
}

func (d *debtUsecase) AddingDebtCustomer(ctx context.Context, request *requestdto.AddDebtRequest) error {
	customerId, err := uuid.Parse(request.CustomerID)
	if err != nil {
		d.log.Error("failed to parse customer id", zap.Error(err))
		return domain.InvalidID("invalid customer id format")
	}

	dueDate, err := time.Parse(dateLayout, request.JatuhTempo)
	if err != nil {
		d.log.Error("failed to parse jatuh_tempo", zap.Error(err))
		return domain.Validation("invalid jatuh_tempo format (expected YYYY-MM-DD)")
	}

	debt := &domain.Debts{
		CustomerID:    customerId,
		TotalDebt:     request.TotalTransaksi,
		RemainingDebt: request.TotalTransaksi,
		Status:        enum.BELUM_LUNAS,
		DueDate:       dueDate,
	}
	if err := d.debtRepo.AddDebt(ctx, debt); err != nil {
		d.log.Error("failed to add debt", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to add debt: %w", domain.ErrInternal)
		}
		return err
	}
	return nil
}

func (d *debtUsecase) DeleteDebtCustomer(ctx context.Context, request *requestdto.DeleteDebtRequest) error {
	id, err := uuid.Parse(request.DebtId)
	if err != nil {
		d.log.Error("failed to parse debt id", zap.Error(err))
		return domain.InvalidID("invalid debt id format")
	}
	if err := d.debtRepo.DeleteDebt(ctx, id); err != nil {
		d.log.Error("failed to delete debt", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to delete debt: %w", domain.ErrInternal)
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
		d.log.Error("failed to get debt", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get debt: %w", domain.ErrInternal)
	}

	response := toDebtResponse(debt)
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
		d.log.Error("failed to get debt", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get debt: %w", domain.ErrInternal)
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
