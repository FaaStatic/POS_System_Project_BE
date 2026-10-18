package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
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

type transactionUsecase struct {
	trxRepo     domain.TransactionRepository
	productRepo domain.ProductRepository
	userRepo    domain.UserRepository
	storeName   string
	log         *zap.Logger
}

func NewTransactionUsecase(trxRepo domain.TransactionRepository, productRepo domain.ProductRepository, userRepo domain.UserRepository, storeName string, log *zap.Logger) domain.TransactionUsecase {
	return &transactionUsecase{
		trxRepo:     trxRepo,
		productRepo: productRepo,
		userRepo:    userRepo,
		storeName:   storeName,
		log:         log,
	}
}

const invoicePrefix = "INV-"

func ensureInvoicePrefix(noInvoice string) string {
	trimmed := strings.TrimSpace(noInvoice)
	if trimmed == "" {
		return generateInvoice()
	}
	if strings.HasPrefix(strings.ToUpper(trimmed), invoicePrefix) {
		return trimmed
	}
	return invoicePrefix + trimmed
}

func (t *transactionUsecase) AddTransaction(ctx context.Context, dto *requestdto.AddTransactionRequest) (*responsedto.AddTransactionResponse, error) {
	return t.addTransaction(ctx, dto, true)
}

func (t *transactionUsecase) AddPrepaidTransaction(ctx context.Context, dto *requestdto.AddTransactionRequest) (*responsedto.AddTransactionResponse, error) {
	return t.addTransaction(ctx, dto, false)
}

func (t *transactionUsecase) addTransaction(ctx context.Context, dto *requestdto.AddTransactionRequest, deductStock bool) (*responsedto.AddTransactionResponse, error) {
	noInvoice := ensureInvoicePrefix(dto.NoInvoice)

	userId, err := uuid.Parse(dto.UserId)
	if err != nil {
		t.log.Error("failed to parse user id", zap.Error(err))
		return nil, domain.InvalidID("invalid user id format")
	}

	if _, err := t.userRepo.GetUserById(ctx, userId); err != nil {
		t.log.Error("failed to get user", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get user: %w", domain.ErrInternal)
	}

	var customerId *uuid.UUID
	if dto.CustomerId != nil {
		parsedID, err := uuid.Parse(*dto.CustomerId)
		if err != nil {
			t.log.Error("failed to parse customer ID", zap.Error(err))
			return nil, domain.InvalidID("failed to parse customer ID")
		}
		customerId = &parsedID
	}

	paymentType, err := enum.ParseMoneyPayment(dto.TypePayment)
	if err != nil {
		t.log.Error("failed to parse payment type", zap.Error(err))
		return nil, domain.Validation("payment type is not valid")
	}
	isHutang := paymentType == enum.Hutang
	if isHutang && customerId == nil {
		t.log.Error("customer id is required for hutang")
		return nil, domain.Validation("customer id is required for hutang")
	}

	var bank *string
	if paymentType.String() == "transfer" {
		if dto.Bank == nil || (*dto.Bank != "bca" && *dto.Bank != "mandiri") {
			t.log.Error("bank is required for transfer", zap.String("no_invoice", noInvoice))
			return nil, domain.Validation("bank (bca/mandiri) is required for transfer payment")
		}
		bank = dto.Bank
	}

	productIDs := make([]uuid.UUID, 0, len(dto.Details))
	for _, detail := range dto.Details {
		productId, err := uuid.Parse(detail.ProductId)
		if err != nil {
			t.log.Error("failed to parse product id", zap.Error(err))
			return nil, domain.InvalidID("invalid product id format")
		}
		productIDs = append(productIDs, productId)
	}
	products, err := t.productRepo.GetProductsByIDs(ctx, productIDs, !deductStock)
	if err != nil {
		t.log.Error("failed to get products", zap.Error(err))
		return nil, fmt.Errorf("failed to get product: %w", domain.ErrInternal)
	}

	detailTrx := make([]domain.TransactionsDetail, 0, len(dto.Details))
	var total int64
	for i, detail := range dto.Details {
		productId := productIDs[i]
		product, ok := products[productId]
		if !ok {
			t.log.Error("product not found", zap.String("product_id", detail.ProductId))
			return nil, domain.NotFound(fmt.Sprintf("product with id %s not found", productId))
		}

		unitPrice := product.SellingPrice
		if isHutang && !product.ProductType.IsDigital() {
			unitPrice = product.SellingPriceDebt
		}
		if !deductStock && detail.UnitPrice != nil {
			unitPrice = *detail.UnitPrice
		}

		var destination *string
		if product.ProductType.IsDigital() {
			if detail.Destination == nil || strings.TrimSpace(*detail.Destination) == "" {
				t.log.Error("destination required for digital product", zap.String("product_id", detail.ProductId))
				return nil, domain.Validation(fmt.Sprintf("destination is required for digital product %s", detail.ProductId))
			}
			d := strings.TrimSpace(*detail.Destination)
			destination = &d
		}

		subtotal := int64(math.Round(float64(unitPrice) * detail.Qty))
		total += subtotal

		detailTrx = append(detailTrx, domain.TransactionsDetail{
			ProductID:     productId,
			ProductName:   product.ProductName,
			Price:         unitPrice,
			PriceDebt:     product.SellingPriceDebt,
			PurchasePrice: product.PurchasePrice,
			Qty:           detail.Qty,
			Subtotal:      subtotal,
			Destination:   destination,
		})
	}

	data := &domain.Transactions{
		NoInvoice:         noInvoice,
		UserID:            userId,
		CustomerID:        customerId,
		PaymentType:       paymentType,
		Bank:              bank,
		TotalTransaction:  total,
		TransactionDetail: detailTrx,
	}

	debtSnapshot, err := t.trxRepo.CreateTransaction(ctx, data, isHutang, deductStock)
	if err != nil {
		t.log.Error("failed to create transaction", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return nil, fmt.Errorf("failed to create transaction: %w", domain.ErrInternal)
		}
		return nil, err
	}

	resp := &responsedto.AddTransactionResponse{
		TransactionID:    data.ID,
		NoInvoice:        data.NoInvoice,
		TotalTransaction: data.TotalTransaction,
		PaymentType:      paymentType.String(),
	}
	if debtSnapshot != nil {
		resp.DebtInfo = &responsedto.DebtTransactionInfo{
			DebtID:                debtSnapshot.DebtID.String(),
			PreviousRemainingDebt: debtSnapshot.PreviousRemainingDebt,
			AmountAdded:           debtSnapshot.AmountAdded,
			TotalDebt:             debtSnapshot.TotalDebt,
			RemainingDebt:         debtSnapshot.RemainingDebt,
			Status:                debtSnapshot.Status.String(),
		}
	}

	return resp, nil
}

func (t *transactionUsecase) DeleteTransaction(ctx context.Context, dto *requestdto.DeleteTransactionRequest) error {
	trxId, err := uuid.Parse(dto.ID)
	if err != nil {
		t.log.Error("transaction id parse fail", zap.Error(err))
		return domain.InvalidID("invalid transaction id format")
	}
	if err := t.trxRepo.DeleteTransaction(ctx, trxId); err != nil {
		t.log.Error("transaction delete fail", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to delete transaction: %w", domain.ErrInternal)
		}
		return err
	}

	return nil
}

func (t *transactionUsecase) GetAllTransaction(ctx context.Context, dto *requestdto.FilterTransactionRequest) (*responsedto.GetAllTransactionResponse, error) {
	cursor, err := parseCursor(dto.AfterID, dto.AfterTime)
	if err != nil {
		return nil, err
	}
	dateFrom, err := parseStoreDay(dto.DateStart, "date_start")
	if err != nil {
		return nil, err
	}
	dateUntil, err := parseStoreDay(dto.DateEnd, "date_end")
	if err != nil {
		return nil, err
	}
	if dateUntil != nil {
		next := dateUntil.AddDate(0, 0, 1)
		dateUntil = &next
	}

	var customerID *uuid.UUID
	if dto.CustomerID != "" {
		id, err := uuid.Parse(dto.CustomerID)
		if err != nil {
			return nil, domain.InvalidID("invalid customer id format")
		}
		customerID = &id
	}

	filter := &domain.FilterTransaction{
		NoInvoices: dto.InvoiceNumber,
		CustomerID: customerID,
		Cursor:     cursor,
		DateFrom:   dateFrom,
		DateUntil:  dateUntil,
		Limit:      dto.Limit,
		Order:      dto.Order,
		TypeTrx:    dto.TypePayment,
	}

	result, err := t.trxRepo.GetAllTransaction(ctx, *filter)
	if err != nil {
		t.log.Error("failed to get all transactions", zap.Error(err))
		return nil, fmt.Errorf("failed to get all transactions: %w", domain.ErrInternal)
	}

	responses := make([]*responsedto.TransactionResponse, 0, len(result.DataItem))
	for _, trx := range result.DataItem {
		details := make([]*responsedto.ProductTransactionResponse, 0, len(trx.TransactionDetail))
		for _, d := range trx.TransactionDetail {
			details = append(details, &responsedto.ProductTransactionResponse{
				ProductID:   d.ProductID,
				ProductName: d.ProductName,
				Price:       d.Price,
				Qty:         d.Qty,
				Subtotal:    d.Subtotal,
			})
		}
		responses = append(responses, &responsedto.TransactionResponse{
			TransactionID:      trx.ID,
			InvoiceNumber:      trx.NoInvoice,
			PaymentType:        int(trx.PaymentType),
			TotalTransaction:   trx.TotalTransaction,
			CreatedAt:          trx.CreatedAt.Format(time.RFC3339),
			TransactionDetails: details,
		})
	}

	nextId, nextTime := result.Cursor.Encode()

	return &responsedto.GetAllTransactionResponse{
		UserID:          dto.UserId,
		AfterId:         nextId,
		AfterTime:       nextTime,
		HasNext:         result.HasNext,
		TransactionList: responses,
	}, nil
}

func (t *transactionUsecase) GetTransaction(ctx context.Context, dto *requestdto.GetTransactionRequest) (*responsedto.TransactionResponse, error) {
	trxId, err := uuid.Parse(dto.ID)
	if err != nil {
		t.log.Error("failed to parse transaction id", zap.Error(err))
		return nil, domain.InvalidID("invalid transaction id format")
	}

	result, err := t.trxRepo.GetTransactionByID(ctx, trxId)
	if err != nil {
		t.log.Error("failed to get transaction", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get transaction: %w", domain.ErrInternal)
	}

	var transactionDetail []*responsedto.ProductTransactionResponse
	var totalProfit int64
	for _, d := range result.TransactionDetail {
		transactionDetail = append(transactionDetail, &responsedto.ProductTransactionResponse{
			ProductID:   d.ProductID,
			ProductName: d.ProductName,
			Price:       d.Price,
			Qty:         d.Qty,
			Subtotal:    d.Subtotal,
		})
		totalProfit += int64(math.Round(float64(d.Price-d.PurchasePrice) * d.Qty))
	}

	response := &responsedto.TransactionResponse{
		TransactionID:      result.ID,
		InvoiceNumber:      result.NoInvoice,
		PaymentType:        int(result.PaymentType),
		TotalTransaction:   result.TotalTransaction,
		TotalProfit:        totalProfit,
		CreatedAt:          result.CreatedAt.Format(time.RFC3339),
		TransactionDetails: transactionDetail,
	}
	return response, nil
}

func (t *transactionUsecase) PrintReportMonth(ctx context.Context, dto *requestdto.PrintReportMonthRequest) (*responsedto.PrintReportMonthTransactionResponse, error) {
	month := dto.Month
	year := dto.Year
	now := time.Now()
	if month == 0 {
		month = int(now.Month())
	}
	if year == 0 {
		year = now.Year()
	}
	if month < 1 || month > 12 {
		t.log.Error("invalid month", zap.Int("month", month))
		return nil, domain.Validation(fmt.Sprintf("invalid month: %d", month))
	}

	userId, err := uuid.Parse(dto.UserId)
	if err != nil {
		t.log.Error("failed to parse user id", zap.Error(err))
		return nil, domain.InvalidID("invalid user id format")
	}
	user, err := t.userRepo.GetUserById(ctx, userId)
	if err != nil {
		t.log.Error("failed to get user", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get user: %w", domain.ErrInternal)
	}

	report, err := t.trxRepo.GetMonthlyReport(ctx, month, year)
	if err != nil {
		t.log.Error("failed to get monthly report", zap.Error(err))
		return nil, fmt.Errorf("failed to get monthly report: %w", domain.ErrInternal)
	}

	dailyReport, err := t.trxRepo.GetDailyReport(ctx, month, year)
	if err != nil {
		t.log.Error("failed to get daily report", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily report: %w", domain.ErrInternal)
	}

	daily := make([]pdf.MonthReportDailyRow, 0, len(dailyReport))
	for _, d := range dailyReport {
		daily = append(daily, pdf.MonthReportDailyRow{
			Date:             d.Date,
			TotalTransaction: d.TotalTransaction,
			Revenue:          d.TotalRevenue,
			Debt:             d.TotalDebt,
			Total:            d.GrandTotal,
		})
	}

	productSold, err := t.trxRepo.GetMonthlyProductSold(ctx, month, year)
	if err != nil {
		t.log.Error("failed to get monthly product sold", zap.Error(err))
		return nil, fmt.Errorf("failed to get monthly product sold: %w", domain.ErrInternal)
	}

	products := make([]pdf.MonthReportProductRow, 0, len(productSold))
	for _, p := range productSold {
		products = append(products, pdf.MonthReportProductRow{
			ProductName: p.ProductName,
			Qty:         p.Qty,
			Total:       p.Total,
		})
	}

	dailyProductSold, err := t.trxRepo.GetDailyProductSold(ctx, month, year)
	if err != nil {
		t.log.Error("failed to get daily product sold", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily product sold: %w", domain.ErrInternal)
	}

	var dailyProducts []pdf.MonthReportDailyProducts
	for _, p := range dailyProductSold {
		n := len(dailyProducts)
		if n == 0 || !dailyProducts[n-1].Date.Equal(p.Date) {
			dailyProducts = append(dailyProducts, pdf.MonthReportDailyProducts{Date: p.Date})
			n++
		}
		dailyProducts[n-1].Products = append(dailyProducts[n-1].Products, pdf.MonthReportProductRow{
			ProductName: p.ProductName,
			Qty:         p.Qty,
			Total:       p.Total,
		})
	}

	urlPdf, err := pdf.GenerateMonthReport(pdf.MonthReportData{
		StoreName:        t.storeName,
		Cashier:          user.Username,
		Month:            month,
		Year:             year,
		TotalTransaction: report.TotalTransaction,
		TotalRevenue:     report.TotalRevenue,
		TotalDebt:        report.TotalDebt,
		GrandTotal:       report.GrandTotal,
		Daily:            daily,
		ProductsSold:     products,
		DailyProducts:    dailyProducts,
		GeneratedAt:      now,
	})
	if err != nil {
		t.log.Error("failed to generate month report pdf", zap.Error(err))
		return nil, fmt.Errorf("failed to generate report pdf: %w", domain.ErrInternal)
	}

	return &responsedto.PrintReportMonthTransactionResponse{
		ID:     uuid.New(),
		Month:  fmt.Sprintf("%02d", month),
		Year:   strconv.Itoa(year),
		UrlPdf: urlPdf,
	}, nil
}

func (t *transactionUsecase) PrintReportTransaction(ctx context.Context, dto *requestdto.PrintReportTransactionRequest) (*responsedto.PrintReportTransactionResponse, error) {
	var trxId uuid.UUID

	switch {
	case dto.TrxId != "":
		parsed, err := uuid.Parse(dto.TrxId)
		if err != nil {
			t.log.Error("failed to parse trx id", zap.Error(err))
			return nil, domain.InvalidID("invalid trx id format")
		}
		trxId = parsed
	case dto.NoInvoice != "":
		existing, err := t.trxRepo.CheckTransactionByNoInvoice(ctx, dto.NoInvoice)
		if err != nil {
			t.log.Error("failed to check transaction", zap.Error(err))
			return nil, fmt.Errorf("failed to get transaction: %w", domain.ErrInternal)
		}
		if existing == nil {
			t.log.Error("transaction not found", zap.String("no_invoice", dto.NoInvoice))
			return nil, domain.NotFound(fmt.Sprintf("transaction with no invoice %s not found", dto.NoInvoice))
		}
		trxId = existing.ID
	default:
		return nil, domain.Validation("trx_id or number_invoice is required")
	}

	trx, err := t.trxRepo.GetTransactionByID(ctx, trxId)
	if err != nil {
		t.log.Error("failed to get transaction", zap.Error(err))
		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get transaction: %w", domain.ErrInternal)
	}

	items := make([]pdf.TransactionReportItem, 0, len(trx.TransactionDetail))
	for _, d := range trx.TransactionDetail {
		items = append(items, pdf.TransactionReportItem{
			ProductName: d.ProductName,
			Qty:         d.Qty,
			Price:       d.Price,
			Subtotal:    d.Subtotal,
		})
	}

	var customerName string
	if trx.CustomerID != nil {
		customerName = trx.Customer.Name
	}
	urlPdf, err := pdf.GenerateTransactionReport(pdf.TransactionReportData{
		StoreName:   t.storeName,
		NoInvoice:   trx.NoInvoice,
		Cashier:     trx.User.Username,
		Customer:    customerName,
		PaymentType: trx.PaymentType.String(),
		CreatedAt:   trx.CreatedAt,
		Items:       items,
		Total:       trx.TotalTransaction,
		GeneratedAt: time.Now(),
	})
	if err != nil {
		t.log.Error("failed to generate transaction report pdf", zap.Error(err))
		return nil, fmt.Errorf("failed to generate report pdf: %w", domain.ErrInternal)
	}

	return &responsedto.PrintReportTransactionResponse{
		ID:        trx.ID,
		NoInvoice: trx.NoInvoice,
		UrlPdf:    urlPdf,
	}, nil
}
