package usecase

import (
	"context"
	"errors"
	"testing"

	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPrintReportTransaction_RefusesACancelledTransaction(t *testing.T) {
	trx := &domain.Transactions{ID: uuid.New(), DeletedAt: gorm.DeletedAt{Valid: true}}
	u := newTestTransactionUsecase(&fakeTrxRepo{getByIDResult: trx}, &fakeTrxProductRepo{}, &fakeTrxUserRepo{})

	_, err := u.PrintReportTransaction(context.Background(), &requestdto.PrintReportTransactionRequest{TrxId: trx.ID.String()})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("a cancelled transaction must not print a receipt, got %v", err)
	}
}
