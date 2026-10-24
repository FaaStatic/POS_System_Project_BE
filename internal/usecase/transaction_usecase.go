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
	"slices"
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
	return t.addTransaction(ctx, dto, true, nil)
}

func (t *transactionUsecase) AddPrepaidTransaction(ctx context.Context, dto *requestdto.AddTransactionRequest) (*responsedto.AddTransactionResponse, error) {
	return t.addTransaction(ctx, dto, false, nil)
}

// offlineSale membawa identitas transaksi yang dibuat kasir saat offline.
type offlineSale struct {
	clientRef uuid.UUID
	soldAt    time.Time
}

// addTransaction: offline != nil memakai harga snapshot dari kasir (pembeli sudah
// membayar harga itu) dan waktu jual dari device, serta membiarkan stok minus.
func (t *transactionUsecase) addTransaction(ctx context.Context, dto *requestdto.AddTransactionRequest, deductStock bool, offline *offlineSale) (*responsedto.AddTransactionResponse, error) {
	noInvoice := ensureInvoicePrefix(dto.NoInvoice)

	userId, err := uuid.Parse(dto.UserId)
	if err != nil {
		t.log.Error("failed to parse user id", zap.Error(err))
		return nil, domain.InvalidID("invalid user id format")
	}

	if _, err := t.userRepo.GetUserById(ctx, userId); err != nil {
		return nil, passOrInternal(t.log, err, "failed to get user", domain.ErrNotFound)
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
		if isHutang {
			unitPrice = product.SellingPriceDebt
		}
		if (!deductStock || offline != nil) && detail.UnitPrice != nil {
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
	if offline != nil {
		data.ClientRef = &offline.clientRef
		data.CreatedAt = offline.soldAt
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
		StockConflict:    data.StockConflict,
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
	if dto.Role != enum.Superadmin.String() {
		if err := t.assertStaffCanCancel(ctx, trxId, dto.UserID); err != nil {
			return err
		}
	}
	if err := t.trxRepo.DeleteTransaction(ctx, trxId, strings.TrimSpace(dto.Reason)); err != nil {
		t.log.Error("transaction delete fail", zap.Error(err))
		if errors.Is(err, domain.ErrInternal) {
			return fmt.Errorf("failed to delete transaction: %w", domain.ErrInternal)
		}
		return err
	}

	return nil
}

// assertStaffCanCancel: staff hanya boleh membatalkan transaksinya sendiri di
// hari yang sama (WIB). Pemilik dan created_at tidak pernah berubah, jadi cek di
// luar transaksi DB aman.
func (t *transactionUsecase) assertStaffCanCancel(ctx context.Context, trxID uuid.UUID, rawUserID string) error {
	trx, err := t.trxRepo.GetTransactionByID(ctx, trxID)
	if err != nil {
		return passOrInternal(t.log, err, "failed to delete transaction", domain.ErrNotFound)
	}
	if trx.UserID.String() != rawUserID {
		return domain.Forbidden("staff can only cancel their own transactions")
	}
	now := time.Now().In(domain.StoreLocation)
	if trx.CreatedAt.In(domain.StoreLocation).Format(dateLayout) != now.Format(dateLayout) {
		return domain.Forbidden("staff can only cancel transactions created today; ask a superadmin")
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
		NoInvoices:   dto.InvoiceNumber,
		CustomerID:   customerID,
		Cursor:       cursor,
		DateFrom:     dateFrom,
		DateUntil:    dateUntil,
		Limit:        dto.Limit,
		Order:        dto.Order,
		TypeTrx:      dto.TypePayment,
		OpenConflict: dto.OpenConflict,
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
			details = append(details, toProductTrxResponse(d))
		}
		responses = append(responses, &responsedto.TransactionResponse{
			TransactionID:      trx.ID,
			InvoiceNumber:      trx.NoInvoice,
			PaymentType:        int(trx.PaymentType),
			TotalTransaction:   trx.TotalTransaction,
			StockConflict:      trx.StockConflict,
			ConflictReviewed:   trx.ConflictReviewedAt != nil,
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

func toProductTrxResponse(d domain.TransactionsDetail) *responsedto.ProductTransactionResponse {
	return &responsedto.ProductTransactionResponse{
		ProductID:   d.ProductID,
		ProductName: d.ProductName,
		Price:       d.Price,
		Qty:         d.Qty,
		Subtotal:    d.Subtotal,
	}
}

func (t *transactionUsecase) GetTransaction(ctx context.Context, dto *requestdto.GetTransactionRequest) (*responsedto.TransactionResponse, error) {
	trxId, err := uuid.Parse(dto.ID)
	if err != nil {
		t.log.Error("failed to parse transaction id", zap.Error(err))
		return nil, domain.InvalidID("invalid transaction id format")
	}

	result, err := t.trxRepo.GetTransactionByID(ctx, trxId)
	if err != nil {
		return nil, passOrInternal(t.log, err, "failed to get transaction", domain.ErrNotFound)
	}

	var transactionDetail []*responsedto.ProductTransactionResponse
	var totalProfit int64
	for _, d := range result.TransactionDetail {
		transactionDetail = append(transactionDetail, toProductTrxResponse(d))
		totalProfit += int64(math.Round(float64(d.Price-d.PurchasePrice) * d.Qty))
	}

	response := &responsedto.TransactionResponse{
		TransactionID:      result.ID,
		InvoiceNumber:      result.NoInvoice,
		PaymentType:        int(result.PaymentType),
		TotalTransaction:   result.TotalTransaction,
		TotalProfit:        totalProfit,
		StockConflict:      result.StockConflict,
		ConflictReviewed:   result.ConflictReviewedAt != nil,
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
		return nil, passOrInternal(t.log, err, "failed to get user", domain.ErrNotFound)
	}

	dailyReport, err := t.trxRepo.GetDailyReport(ctx, month, year)
	if err != nil {
		t.log.Error("failed to get daily report", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily report: %w", domain.ErrInternal)
	}

	debtFlow, err := t.trxRepo.GetDailyDebtFlow(ctx, month, year)
	if err != nil {
		t.log.Error("failed to get daily debt flow", zap.Error(err))
		return nil, fmt.Errorf("failed to get daily debt flow: %w", domain.ErrInternal)
	}

	daily := mergeDailyReport(dailyReport, debtFlow)

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
		StoreName:     t.storeName,
		Cashier:       user.Username,
		Month:         month,
		Year:          year,
		Daily:         daily,
		ProductsSold:  products,
		DailyProducts: dailyProducts,
		GeneratedAt:   now,
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

// mergeDailyReport menggabungkan penjualan harian dengan arus hutang harian
// (manual + pembayaran); hari yang hanya punya pembayaran tetap muncul.
func mergeDailyReport(sales []domain.DailyReport, flow []domain.DailyDebtFlow) []pdf.MonthReportDailyRow {
	byDate := make(map[string]*pdf.MonthReportDailyRow, len(sales)+len(flow))
	row := func(date time.Time) *pdf.MonthReportDailyRow {
		key := date.Format(dateLayout)
		if r, ok := byDate[key]; ok {
			return r
		}
		r := &pdf.MonthReportDailyRow{Date: date}
		byDate[key] = r
		return r
	}
	for _, s := range sales {
		r := row(s.Date)
		r.CountSale, r.Revenue, r.Cost = s.CountSale, s.Revenue, s.Cost
		r.CountDebt, r.DebtSales, r.DebtCost = s.CountDebt, s.DebtSales, s.DebtCost
	}
	for _, f := range flow {
		r := row(f.Date)
		r.DebtManual, r.DebtPaid = f.Manual, f.Paid
	}

	daily := make([]pdf.MonthReportDailyRow, 0, len(byDate))
	for _, r := range byDate {
		daily = append(daily, *r)
	}
	slices.SortFunc(daily, func(a, b pdf.MonthReportDailyRow) int { return a.Date.Compare(b.Date) })
	return daily
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
		return nil, passOrInternal(t.log, err, "failed to get transaction", domain.ErrNotFound)
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
