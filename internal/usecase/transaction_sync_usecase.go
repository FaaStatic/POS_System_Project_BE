package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Jam device di luar [now-maxOfflineAge, now+maxClockSkew] dianggap salah dan
// diganti waktu server, supaya transaksi tidak masuk ke laporan hari yang belum
// terjadi atau ke laporan bulan lama yang mungkin sudah dicetak.
const (
	maxClockSkew  = 5 * time.Minute
	maxOfflineAge = 30 * 24 * time.Hour
)

const (
	syncCreated   = "created"
	syncDuplicate = "duplicate"
	syncFailed    = "failed"
)

func (t *transactionUsecase) SyncOfflineTransactions(ctx context.Context, dto *requestdto.SyncTransactionsRequest) (*responsedto.SyncTransactionsResponse, error) {
	results := make([]responsedto.SyncTransactionResult, 0, len(dto.Transactions))
	for _, item := range dto.Transactions {
		results = append(results, t.syncOne(ctx, dto.UserId, item))
	}
	return &responsedto.SyncTransactionsResponse{Results: results}, nil
}

func (t *transactionUsecase) syncOne(ctx context.Context, userID string, item requestdto.OfflineTransaction) responsedto.SyncTransactionResult {
	res := responsedto.SyncTransactionResult{ClientRef: item.ClientRef, Status: syncFailed}

	sale, req, err := parseOfflineTransaction(userID, item, time.Now())
	if err != nil {
		res.Error = err.Error()
		return res
	}

	existing, err := t.trxRepo.GetTransactionByClientRef(ctx, sale.clientRef)
	if err != nil {
		t.log.Error("failed to check client ref", zap.Error(err), zap.String("client_ref", item.ClientRef))
		res.Retryable = true
		res.Error = "failed to check client_ref, try again"
		return res
	}
	if existing != nil {
		return duplicateSyncResult(res, existing)
	}

	created, err := t.addTransaction(ctx, req, true, sale)
	if err != nil {
		// Request lain dengan client_ref sama bisa menang di unique index.
		if errors.Is(err, domain.ErrDuplicate) {
			if existing, lookupErr := t.trxRepo.GetTransactionByClientRef(ctx, sale.clientRef); lookupErr == nil && existing != nil {
				return duplicateSyncResult(res, existing)
			}
		}
		res.Error = err.Error()
		res.Retryable = errors.Is(err, domain.ErrInternal) || errors.Is(err, domain.ErrDuplicate)
		return res
	}

	res.Status = syncCreated
	res.TransactionID = &created.TransactionID
	res.NoInvoice = created.NoInvoice
	res.StockConflict = created.StockConflict
	return res
}

func duplicateSyncResult(res responsedto.SyncTransactionResult, existing *domain.Transactions) responsedto.SyncTransactionResult {
	res.Status = syncDuplicate
	res.TransactionID = &existing.ID
	res.NoInvoice = existing.NoInvoice
	res.StockConflict = existing.StockConflict
	return res
}

// parseOfflineTransaction mengubah item antrean kasir menjadi request transaksi
// biasa. Hanya tunai dan hutang yang bisa dicatat tanpa internet.
func parseOfflineTransaction(userID string, item requestdto.OfflineTransaction, now time.Time) (*offlineSale, *requestdto.AddTransactionRequest, error) {
	ref, err := uuid.Parse(strings.TrimSpace(item.ClientRef))
	if err != nil {
		return nil, nil, domain.Validation("client_ref must be a UUID")
	}
	payment := strings.ToLower(strings.TrimSpace(item.TypePayment))
	if payment != "tunai" && payment != "hutang" {
		return nil, nil, domain.Validation("only tunai and hutang can be recorded offline")
	}
	soldAt, err := time.Parse(time.RFC3339, strings.TrimSpace(item.CreatedAtClient))
	if err != nil {
		return nil, nil, domain.Validation("created_at_client must be RFC3339")
	}
	if soldAt.After(now.Add(maxClockSkew)) || soldAt.Before(now.Add(-maxOfflineAge)) {
		soldAt = now
	}
	if len(item.Details) == 0 {
		return nil, nil, domain.Validation("details must not be empty")
	}

	details := make([]requestdto.AddTransactionDetailRequest, 0, len(item.Details))
	for i, d := range item.Details {
		if d.Qty <= 0 {
			return nil, nil, domain.Validation(fmt.Sprintf("details[%d].qty must be greater than 0", i))
		}
		if d.UnitPrice < 0 {
			return nil, nil, domain.Validation(fmt.Sprintf("details[%d].unit_price must not be negative", i))
		}
		price := d.UnitPrice
		details = append(details, requestdto.AddTransactionDetailRequest{
			ProductId:   d.ProductId,
			Qty:         d.Qty,
			Destination: d.Destination,
			UnitPrice:   &price,
		})
	}

	return &offlineSale{clientRef: ref, soldAt: soldAt}, &requestdto.AddTransactionRequest{
		TypePayment: payment,
		UserId:      userID,
		CustomerId:  item.CustomerId,
		Details:     details,
	}, nil
}

func (t *transactionUsecase) ReviewConflict(ctx context.Context, id string) error {
	trxID, err := uuid.Parse(id)
	if err != nil {
		return domain.InvalidID("invalid transaction id format")
	}
	if err := t.trxRepo.MarkConflictReviewed(ctx, trxID); err != nil {
		return passOrInternal(t.log, err, "failed to mark conflict reviewed", domain.ErrNotFound)
	}
	return nil
}
