package usecase

import (
	"context"
	"fmt"
	"math"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// buildChange memvalidasi jumlah bayar dan rincian kembalian lalu menyusun baris
// yang disimpan; mengembalikan kembalian yang harus diberikan. Nilai barang
// dihitung dari harga jual produk, bukan dari klien, dan jumlah semua bagian harus
// pas dengan kembalian. Tanpa amount_paid tidak ada yang dicatat (transaksi lama,
// atau kasir yang belum mengirim jumlah bayar).
func (t *transactionUsecase) buildChange(ctx context.Context, dto *requestdto.AddTransactionRequest, paymentType enum.MoneyPayment, total int64) (int64, []domain.TransactionChange, error) {
	if dto.AmountPaid == nil {
		if len(dto.Change) > 0 {
			return 0, nil, domain.Validation("amount_paid is required when change is given")
		}
		return 0, nil, nil
	}
	if paymentType != enum.Tunai {
		return 0, nil, domain.Validation("amount_paid and change only apply to tunai payments")
	}
	due := *dto.AmountPaid - total
	if due < 0 {
		return 0, nil, domain.Validation(fmt.Sprintf("amount_paid %d is less than the total %d", *dto.AmountPaid, total))
	}

	// Kind divalidasi di sini, bukan hanya oleh tag DTO: jalur sync offline tidak
	// memakai `dive`, dan kind asing akan jatuh ke CHECK di DB sebagai error internal
	// yang membuat kasir mengulang item yang sama terus.
	productIDs := make([]uuid.UUID, len(dto.Change))
	var toFetch []uuid.UUID
	for i, line := range dto.Change {
		switch line.Kind {
		case domain.ChangeCash:
		case domain.ChangeCandy, domain.ChangeProduct:
			if line.ProductId == nil {
				return 0, nil, domain.Validation(fmt.Sprintf("change[%d].product_id is required for %s", i, line.Kind))
			}
			id, err := uuid.Parse(*line.ProductId)
			if err != nil {
				return 0, nil, domain.InvalidID(fmt.Sprintf("change[%d].product_id is not a valid id", i))
			}
			productIDs[i] = id
			toFetch = append(toFetch, id)
		default:
			return 0, nil, domain.Validation(fmt.Sprintf("change[%d].kind %q is not valid; use tunai, permen or produk_lain", i, line.Kind))
		}
	}
	products, err := t.productRepo.GetProductsByIDs(ctx, toFetch, false)
	if err != nil {
		t.log.Error("failed to get change products", zap.Error(err))
		return 0, nil, fmt.Errorf("failed to get product: %w", domain.ErrInternal)
	}

	lines := make([]domain.TransactionChange, 0, len(dto.Change))
	var given int64
	for i, line := range dto.Change {
		if line.Kind == domain.ChangeCash {
			if line.Amount <= 0 || line.ProductId != nil {
				return 0, nil, domain.Validation(fmt.Sprintf("change[%d]: tunai needs amount and no product", i))
			}
			lines = append(lines, domain.TransactionChange{Kind: line.Kind, Amount: line.Amount})
			given += line.Amount
			continue
		}

		product, ok := products[productIDs[i]]
		if !ok {
			return 0, nil, domain.NotFound(fmt.Sprintf("change[%d]: product %s not found", i, *line.ProductId))
		}
		if product.ProductType.IsDigital() {
			return 0, nil, domain.Validation(fmt.Sprintf("change[%d]: digital products cannot be given as change", i))
		}
		if line.Qty <= 0 {
			return 0, nil, domain.Validation(fmt.Sprintf("change[%d].qty must be greater than 0", i))
		}
		if err := domain.CheckQty(product.Unit, line.Qty); err != nil {
			return 0, nil, err
		}
		value := int64(math.Round(float64(product.SellingPrice) * line.Qty))
		if value <= 0 {
			return 0, nil, domain.Validation(fmt.Sprintf("change[%d]: quantity is too small to have a value", i))
		}
		if line.Amount != 0 && line.Amount != value {
			return 0, nil, domain.Validation(fmt.Sprintf("change[%d].amount %d does not match the product price (%d)", i, line.Amount, value))
		}
		productID := product.ID
		lines = append(lines, domain.TransactionChange{
			Kind:          line.Kind,
			Amount:        value,
			ProductID:     &productID,
			ProductName:   product.ProductName,
			Qty:           line.Qty,
			UnitPrice:     product.SellingPrice,
			PurchasePrice: product.PurchasePrice,
		})
		given += value
	}
	if given != due {
		return 0, nil, domain.Validation(fmt.Sprintf("change lines add up to %d but the change due is %d", given, due))
	}
	return due, lines, nil
}

func toChangeResponse(lines []domain.TransactionChange) []responsedto.ChangeLineResponse {
	out := make([]responsedto.ChangeLineResponse, 0, len(lines))
	for _, l := range lines {
		out = append(out, responsedto.ChangeLineResponse{
			Kind:        l.Kind,
			Amount:      l.Amount,
			ProductID:   l.ProductID,
			ProductName: l.ProductName,
			Qty:         l.Qty,
		})
	}
	return out
}
