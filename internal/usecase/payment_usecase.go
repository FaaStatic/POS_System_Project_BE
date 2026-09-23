package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type paymentUsecase struct {
	paymentRepo domain.PaymentRepository
	gateway     domain.PaymentGateway
	productRepo domain.ProductRepository
	trxUsecase  domain.TransactionUsecase
	trxRepo     domain.TransactionRepository
	notifier    domain.DeviceTokenUsecase
	log         *zap.Logger
}

func NewPaymentUsecase(
	paymentRepo domain.PaymentRepository,
	gateway domain.PaymentGateway,
	productRepo domain.ProductRepository,
	trxUsecase domain.TransactionUsecase,
	trxRepo domain.TransactionRepository,
	notifier domain.DeviceTokenUsecase,
	log *zap.Logger,
) domain.PaymentUsecase {
	return &paymentUsecase{
		paymentRepo: paymentRepo,
		gateway:     gateway,
		productRepo: productRepo,
		trxUsecase:  trxUsecase,
		trxRepo:     trxRepo,
		notifier:    notifier,
		log:         log,
	}
}

type chargeFunc func(ctx context.Context, in domain.GatewayChargeInput) (*domain.GatewayChargeResult, error)

func (u *paymentUsecase) ChargeQris(ctx context.Context, request *requestdto.ChargeQrisRequest) (*responsedto.ChargePaymentResponse, error) {
	return u.charge(ctx, "qris", "", request.UserId, request.CustomerId, request.Items, u.gateway.ChargeQris)
}

func (u *paymentUsecase) ChargeVA(ctx context.Context, request *requestdto.ChargeVARequest) (*responsedto.ChargePaymentResponse, error) {
	return u.charge(ctx, "va", request.Bank, request.UserId, request.CustomerId, request.Items, u.gateway.ChargeVA)
}

func (u *paymentUsecase) charge(ctx context.Context, method, bank, rawUserID string, rawCustomerID *string, reqItems []requestdto.PaymentItemRequest, gatewayCharge chargeFunc) (*responsedto.ChargePaymentResponse, error) {
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		u.log.Error("failed to parse user id", zap.Error(err))
		return nil, domain.InvalidID("invalid user id format")
	}
	customerID, err := parseOptionalUUID(rawCustomerID)
	if err != nil {
		u.log.Error("failed to parse customer id", zap.Error(err))
		return nil, domain.InvalidID("invalid customer id format")
	}

	subtotal, items, err := u.buildOrder(ctx, toItemPairs(reqItems))
	if err != nil {
		return nil, err
	}
	gross := applyFee(method, subtotal)

	orderID := generateInvoice()

	payment := &domain.Payment{
		OrderID:        orderID,
		UserID:         userID,
		CustomerID:     customerID,
		Method:         method,
		SubtotalAmount: subtotal,
		FeeAmount:      gross - subtotal,
		GrossAmount:    gross,
		Status:         domain.PaymentPending,
		VABank:         bank,
		Items:          items,
		StockReserved:  true,
	}
	if err := u.paymentRepo.CreateWithReservation(ctx, payment); err != nil {
		u.log.Warn("failed to create reserved payment", zap.Error(err), zap.String("order_id", orderID))
		return nil, reserveStockError(err)
	}

	result, err := gatewayCharge(ctx, domain.GatewayChargeInput{OrderID: orderID, GrossAmount: gross, Bank: bank})
	if err != nil {
		u.log.Error("failed to create gateway charge", zap.Error(err), zap.String("method", method), zap.String("order_id", orderID))
		u.failPayment(ctx, orderID)
		return nil, fmt.Errorf("failed to create %s payment: %w", method, domain.ErrInternal)
	}

	applyChargeResult(payment, result)
	if err := u.paymentRepo.UpdateWithLock(ctx, orderID, func(p *domain.Payment) (bool, error) {
		applyChargeResult(p, result)
		return true, nil
	}); err != nil {
		u.log.Error("PAYMENT_RECONCILIATION_REQUIRED: charge created but its details were not saved",
			zap.Error(err), zap.String("order_id", orderID))
	}

	status := string(payment.Status)
	if mapInternalStatus(result.TransactionStatus, result.FraudStatus) == domain.PaymentSuccess {
		if err := u.applyAuthoritativeStatus(ctx, orderID, result.TransactionStatus, result.FraudStatus, result.TransactionID); err != nil {
			u.log.Error("charge settled synchronously but could not be finalized",
				zap.Error(err), zap.String("order_id", orderID))
		}
		if refreshed, rerr := u.paymentRepo.GetByOrderID(ctx, orderID); rerr == nil && refreshed != nil {
			status = string(refreshed.Status)
		}
	}

	return &responsedto.ChargePaymentResponse{
		OrderID:        orderID,
		Method:         method,
		Status:         status,
		GrossAmount:    gross,
		MidtransStatus: result.TransactionStatus,
		QrString:       payment.QRString,
		QrUrl:          payment.QRURL,
		RedirectUrl:    payment.RedirectURL,
		VaNumber:       payment.VANumber,
		Bank:           payment.VABank,
		BillKey:        payment.BillKey,
		BillerCode:     payment.BillerCode,
		ExpiryTime:     result.ExpiryTime,
	}, nil
}

func applyChargeResult(p *domain.Payment, r *domain.GatewayChargeResult) {
	p.MidtransTrxID = r.TransactionID
	p.QRString = r.QRString
	p.QRURL = r.QRURL
	p.RedirectURL = r.RedirectURL
	p.VANumber = r.VANumber
	p.BillKey = r.BillKey
	p.BillerCode = r.BillerCode
	p.ExpiryTime = parseMidtransTime(r.ExpiryTime)
	if r.Bank != "" {
		p.VABank = r.Bank
	}
	if p.MidtransStatus == "" {
		p.MidtransStatus, p.FraudStatus = r.TransactionStatus, r.FraudStatus
	}
}

func (u *paymentUsecase) failPayment(ctx context.Context, orderID string) {
	err := u.paymentRepo.UpdateWithLock(ctx, orderID, func(p *domain.Payment) (bool, error) {
		p.Status = domain.PaymentFailed
		p.StockReserved = false
		return true, nil
	})
	if err != nil {
		u.log.Error("PAYMENT_RECONCILIATION_REQUIRED: failed to release the stock of an uncharged payment",
			zap.Error(err), zap.String("order_id", orderID))
	}
}

func (u *paymentUsecase) HandleNotification(ctx context.Context, notif *requestdto.MidtransNotificationRequest) error {
	u.log.Info("midtrans notification received",
		zap.String("order_id", notif.OrderID),
		zap.String("transaction_status", notif.TransactionStatus),
		zap.String("fraud_status", notif.FraudStatus),
		zap.String("payment_type", notif.PaymentType),
	)

	if !u.gateway.VerifySignature(notif.OrderID, notif.StatusCode, notif.GrossAmount, notif.SignatureKey) {
		u.log.Warn("invalid midtrans signature", zap.String("order_id", notif.OrderID))
		return domain.ErrInvalidSignature
	}

	payment, err := u.paymentRepo.GetByOrderID(ctx, notif.OrderID)
	if err != nil {
		u.log.Error("failed to get payment", zap.Error(err))
		return fmt.Errorf("failed to get payment: %w", domain.ErrInternal)
	}
	if payment == nil {
		u.log.Warn("notification for unknown order", zap.String("order_id", notif.OrderID))
		return nil
	}
	if isTerminalPaymentStatus(payment.Status) {
		return nil
	}

	trxStatus, fraudStatus, midtransTrxID := notif.TransactionStatus, notif.FraudStatus, notif.TransactionID
	if authoritative, cerr := u.gateway.CheckStatus(ctx, notif.OrderID); cerr == nil {
		trxStatus, fraudStatus, midtransTrxID = authoritative.TransactionStatus, authoritative.FraudStatus, authoritative.TransactionID
	} else {
		u.log.Warn("failed to verify status to midtrans, fallback to payload", zap.Error(cerr))
	}

	if err := u.applyAuthoritativeStatus(ctx, notif.OrderID, trxStatus, fraudStatus, midtransTrxID); err != nil {
		if domain.IsTransient(err) {
			return err
		}
		u.log.Error("PAYMENT_RECONCILIATION_REQUIRED: permanent failure finalizing settled payment",
			zap.Error(err), zap.String("order_id", notif.OrderID))
		return nil
	}
	return nil
}

func (u *paymentUsecase) applyAuthoritativeStatus(ctx context.Context, orderID, trxStatus, fraudStatus, midtransTrxID string) error {
	var final *domain.Payment
	var claimed bool
	err := u.paymentRepo.UpdateWithLock(ctx, orderID, func(p *domain.Payment) (bool, error) {
		if isTerminalPaymentStatus(p.Status) {
			return false, nil
		}
		p.MidtransStatus = trxStatus
		p.FraudStatus = fraudStatus
		if midtransTrxID != "" {
			p.MidtransTrxID = midtransTrxID
		}

		switch mapInternalStatus(trxStatus, fraudStatus) {
		case domain.PaymentSuccess:
			p.Status = domain.PaymentSettling
			claimed = true
		case domain.PaymentFailed:
			p.Status = domain.PaymentFailed
		case domain.PaymentExpired:
			p.Status = domain.PaymentExpired
		default:
			if p.Status != domain.PaymentSettling {
				p.Status = domain.PaymentPending
			}
		}

		if p.Status == domain.PaymentFailed || p.Status == domain.PaymentExpired {
			p.StockReserved = false
		}
		final = p
		return true, nil
	})
	if err != nil {
		u.log.Error("failed to process payment status", zap.Error(err), zap.String("order_id", orderID))
		return fmt.Errorf("failed to update payment: %w", domain.ErrInternal)
	}
	if final == nil {
		return nil
	}

	u.log.Info("payment status processed",
		zap.String("order_id", final.OrderID),
		zap.String("midtrans_status", final.MidtransStatus),
		zap.String("internal_status", string(final.Status)),
	)

	if claimed {
		return u.finalizeSettlement(ctx, final.OrderID)
	}

	switch final.Status {
	case domain.PaymentFailed, domain.PaymentExpired:
		u.notifyPaymentResult(ctx, final, false)
	}
	return nil
}

func (u *paymentUsecase) finalizeSettlement(ctx context.Context, orderID string) error {
	payment, err := u.paymentRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		u.log.Error("failed to reload payment for finalization", zap.Error(err), zap.String("order_id", orderID))
		return fmt.Errorf("failed to reload payment: %w", domain.ErrInternal)
	}
	if payment == nil {
		return domain.NotFound(fmt.Sprintf("payment %s not found", orderID))
	}
	if payment.Status == domain.PaymentSuccess && payment.TransactionID != nil {
		return nil
	}

	trxID, err := u.ensureTransaction(ctx, payment)
	if err != nil {
		u.log.Error("PAYMENT_RECONCILIATION_REQUIRED: settled payment has no sales transaction",
			zap.Error(err),
			zap.String("order_id", orderID),
			zap.Bool("transient", domain.IsTransient(err)),
		)
		u.touch(ctx, orderID)
		return err
	}

	var notified *domain.Payment
	err = u.paymentRepo.UpdateWithLock(ctx, orderID, func(p *domain.Payment) (bool, error) {
		if p.Status == domain.PaymentSuccess && p.TransactionID != nil {
			return false, nil
		}
		p.TransactionID = &trxID
		p.StockReserved = false
		if p.PaidAt == nil {
			now := time.Now()
			p.PaidAt = &now
		}
		p.Status = domain.PaymentSuccess
		notified = p
		return true, nil
	})
	if err != nil {
		u.log.Error("failed to mark payment successful", zap.Error(err), zap.String("order_id", orderID))
		return fmt.Errorf("failed to mark payment successful: %w", domain.ErrInternal)
	}
	if notified != nil {
		u.notifyPaymentResult(ctx, notified, true)
	}
	return nil
}

func (u *paymentUsecase) ensureTransaction(ctx context.Context, payment *domain.Payment) (uuid.UUID, error) {
	if existing, err := u.trxRepo.CheckTransactionByNoInvoice(ctx, payment.OrderID); err != nil {
		return uuid.Nil, fmt.Errorf("failed to check transaction: %w", domain.ErrInternal)
	} else if existing != nil {
		return existing.ID, nil
	}

	details := make([]requestdto.AddTransactionDetailRequest, 0, len(payment.Items))
	for _, it := range payment.Items {
		detail := requestdto.AddTransactionDetailRequest{
			ProductId: it.ProductID.String(),
			Qty:       it.Qty,
		}
		if it.UnitPrice > 0 {
			price := it.UnitPrice
			detail.UnitPrice = &price
		}
		details = append(details, detail)
	}

	var customerID *string
	if payment.CustomerID != nil {
		s := payment.CustomerID.String()
		customerID = &s
	}
	var bank *string
	if payment.Method == "va" && payment.VABank != "" {
		b := payment.VABank
		bank = &b
	}
	addReq := &requestdto.AddTransactionRequest{
		NoInvoice:   payment.OrderID,
		TypePayment: methodToPaymentType(payment.Method),
		Bank:        bank,
		UserId:      payment.UserID.String(),
		CustomerId:  customerID,
		Details:     details,
	}

	addTrx := u.trxUsecase.AddTransaction
	if payment.StockReserved {
		addTrx = u.trxUsecase.AddPrepaidTransaction
	}
	resp, err := addTrx(ctx, addReq)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			if existing, lookupErr := u.trxRepo.CheckTransactionByNoInvoice(ctx, payment.OrderID); lookupErr == nil && existing != nil {
				return existing.ID, nil
			}
		}
		return uuid.Nil, fmt.Errorf("failed to create transaction from payment: %w", err)
	}

	if payment.SubtotalAmount > 0 && resp.TotalTransaction != payment.SubtotalAmount {
		u.log.Error("PAYMENT_AMOUNT_MISMATCH: transaction total differs from charged subtotal",
			zap.String("order_id", payment.OrderID),
			zap.Int64("charged_subtotal", payment.SubtotalAmount),
			zap.Int64("transaction_total", resp.TotalTransaction),
			zap.Int64("gross_amount", payment.GrossAmount),
		)
	}
	return resp.TransactionID, nil
}

func (u *paymentUsecase) ReconcileStalePayments(ctx context.Context) error {
	settling, err := u.paymentRepo.ListPendingFinalization(ctx, 50)
	if err != nil {
		return fmt.Errorf("failed to list payments pending finalization: %w", err)
	}
	for _, p := range settling {
		if err := u.finalizeSettlement(ctx, p.OrderID); err != nil {
			u.log.Error("reconcile: failed to finalize settled payment",
				zap.Error(err), zap.String("order_id", p.OrderID))
		}
	}

	stale, err := u.paymentRepo.ListStalePending(ctx, 50)
	if err != nil {
		return fmt.Errorf("failed to list stale payments: %w", err)
	}
	for _, p := range stale {
		authoritative, err := u.gateway.CheckStatus(ctx, p.OrderID)
		if err != nil {
			u.log.Warn("reconcile: failed to check status", zap.Error(err), zap.String("order_id", p.OrderID))
			u.touch(ctx, p.OrderID)
			continue
		}
		if err := u.applyAuthoritativeStatus(ctx, p.OrderID, authoritative.TransactionStatus, authoritative.FraudStatus, authoritative.TransactionID); err != nil {
			u.log.Error("reconcile: failed to apply status", zap.Error(err), zap.String("order_id", p.OrderID))
			u.touch(ctx, p.OrderID)
		}
	}
	return nil
}

func (u *paymentUsecase) touch(ctx context.Context, orderID string) {
	if err := u.paymentRepo.Touch(ctx, orderID); err != nil {
		u.log.Warn("failed to touch payment", zap.Error(err), zap.String("order_id", orderID))
	}
}

func isTerminalPaymentStatus(s domain.PaymentStatus) bool {
	switch s {
	case domain.PaymentSuccess, domain.PaymentFailed, domain.PaymentExpired:
		return true
	}
	return false
}

func reserveStockError(err error) error {
	if domain.IsTransient(err) {
		return fmt.Errorf("failed to reserve stock: %w", domain.ErrInternal)
	}
	return err
}

func (u *paymentUsecase) GetStatus(ctx context.Context, orderID, requesterID, requesterRole string) (*responsedto.PaymentStatusResponse, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, domain.Validation("order_id is required")
	}
	payment, err := u.paymentRepo.GetByOrderID(ctx, orderID)
	if err != nil {
		u.log.Error("failed to get payment", zap.Error(err))
		return nil, fmt.Errorf("failed to get payment: %w", domain.ErrInternal)
	}
	if payment == nil {
		return nil, domain.NotFound("payment not found")
	}

	allowed := requesterRole == "superadmin" ||
		payment.UserID.String() == requesterID
	u.log.Info("payment status access",
		zap.String("order_id", payment.OrderID),
		zap.String("requester_id", requesterID),
		zap.String("requester_role", requesterRole),
		zap.String("owner_id", payment.UserID.String()),
		zap.Bool("allowed", allowed),
	)
	if !allowed {
		return nil, domain.ErrPaymentAccessDenied
	}

	res := &responsedto.PaymentStatusResponse{
		OrderID:        payment.OrderID,
		Method:         payment.Method,
		Status:         string(payment.Status),
		MidtransStatus: payment.MidtransStatus,
		GrossAmount:    payment.GrossAmount,
	}
	if payment.TransactionID != nil {
		res.TransactionID = payment.TransactionID.String()
	}
	if payment.PaidAt != nil {
		res.PaidAt = payment.PaidAt.Format(time.RFC3339)
	}
	return res, nil
}

func (u *paymentUsecase) notifyPaymentResult(ctx context.Context, payment *domain.Payment, success bool) {
	userID, orderID := payment.UserID.String(), payment.OrderID
	amount := payment.GrossAmount
	notifCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	go func() {
		defer cancel()
		if err := u.notifier.NotifyPaymentResult(notifCtx, userID, orderID, success, amount); err != nil {
			u.log.Error("failed to send payment push notification",
				zap.Error(err), zap.String("order_id", orderID))
		}
	}()
}

func (u *paymentUsecase) buildOrder(ctx context.Context, items []itemPair) (int64, []domain.PaymentItem, error) {
	if len(items) == 0 {
		return 0, nil, domain.Validation("items is required")
	}
	productIDs := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		productID, err := uuid.Parse(it.productID)
		if err != nil {
			return 0, nil, domain.InvalidID("invalid product id format")
		}
		productIDs = append(productIDs, productID)
	}
	products, err := u.productRepo.GetProductsByIDs(ctx, productIDs, false)
	if err != nil {
		u.log.Error("failed to get products", zap.Error(err))
		return 0, nil, fmt.Errorf("failed to get product: %w", domain.ErrInternal)
	}

	var subtotal int64
	paymentItems := make([]domain.PaymentItem, 0, len(items))
	for i, it := range items {
		productID := productIDs[i]
		product, ok := products[productID]
		if !ok {
			return 0, nil, domain.NotFound(fmt.Sprintf("product %s not found", it.productID))
		}
		if product.ProductType.IsDigital() {
			return 0, nil, domain.Validation("digital products cannot be purchased via online payment")
		}
		if product.Stock < it.qty {
			return 0, nil, domain.Conflict(fmt.Sprintf("insufficient stock for product %s", it.productID))
		}
		subtotal += int64(math.Round(float64(product.SellingPrice) * it.qty))
		paymentItems = append(paymentItems, domain.PaymentItem{
			ProductID: productID,
			Qty:       it.qty,
			UnitPrice: product.SellingPrice,
		})
	}
	return subtotal, paymentItems, nil
}

type itemPair struct {
	productID string
	qty       float64
}

func toItemPairs(items []requestdto.PaymentItemRequest) []itemPair {
	out := make([]itemPair, 0, len(items))
	for _, it := range items {
		out = append(out, itemPair{productID: it.ProductId, qty: it.Qty})
	}
	return out
}

func parseOptionalUUID(s *string) (*uuid.UUID, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(strings.TrimSpace(*s))
	if err != nil {
		return nil, err
	}
	return &id, nil
}

const (
	feeQrisRate = 0.007
	feeVAFlat   = 4440
)

func applyFee(method string, subtotal int64) int64 {
	switch method {
	case "qris":
		return int64(math.Ceil(float64(subtotal) / (1 - feeQrisRate)))
	case "va":
		return subtotal + feeVAFlat
	default:
		return subtotal
	}
}

func methodToPaymentType(method string) string {
	if method == "va" {
		return "transfer"
	}
	return "qris"
}

func mapInternalStatus(trxStatus, fraudStatus string) domain.PaymentStatus {
	switch strings.ToLower(trxStatus) {
	case "capture":
		if strings.EqualFold(fraudStatus, "accept") {
			return domain.PaymentSuccess
		}
		if strings.EqualFold(fraudStatus, "challenge") {
			return domain.PaymentPending
		}
		return domain.PaymentFailed
	case "settlement":
		return domain.PaymentSuccess
	case "pending":
		return domain.PaymentPending
	case "deny", "cancel":
		return domain.PaymentFailed
	case "expire":
		return domain.PaymentExpired
	default:
		return domain.PaymentPending
	}
}

func generateInvoice() string {
	return invoicePrefix + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:16])
}

func parseMidtransTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, domain.StoreLocation)
	if err != nil {
		return nil
	}
	return &t
}
