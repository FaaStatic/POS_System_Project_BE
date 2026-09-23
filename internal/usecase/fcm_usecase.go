package usecase

import (
	"context"
	"fmt"

	"shop_project_be/infrastructure/fcm"
	"shop_project_be/internal/domain"
	"shop_project_be/pkg/pdf"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fcmUsecase struct {
	fcm     *fcm.Sender
	fcmRepo domain.DeviceTokenRepository
	log     *zap.Logger
}

func NewFcmUsecase(client *fcm.Sender, repo domain.DeviceTokenRepository, logger *zap.Logger) domain.DeviceTokenUsecase {
	return &fcmUsecase{
		fcm:     client,
		fcmRepo: repo,
		log:     logger,
	}
}

func (f *fcmUsecase) HandleLogout(ctx context.Context, token string) error {
	return f.fcmRepo.DetachDeviceTokenFromUser(ctx, token)
}

func (f *fcmUsecase) NotifyPaymentResult(ctx context.Context, userID string, orderID string, success bool, amount int64) error {
	userid, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid userID: %w", err)
	}
	tokens, err := f.fcmRepo.GetDeviceTokensByUserID(ctx, userid)
	if err != nil {
		return fmt.Errorf("failed to get device tokens: %w", err)
	}

	if len(tokens) == 0 {
		f.log.Info("No device tokens found for user", zap.String("userID", userID))
		return nil
	}
	var payload domain.Payload
	if success {
		payload = domain.Payload{
			Title: "Pembayaran Berhasil",
			Body:  fmt.Sprintf("Pembayaran %s untuk pesanan %s berhasil.", pdf.FormatRupiah(amount), orderID),
			Data: map[string]string{
				"order_id": orderID,
				"status":   "success",
				"amount":   fmt.Sprintf("%d", amount),
			},
		}
	} else {
		payload = domain.Payload{
			Title: "Pembayaran Gagal",
			Body:  fmt.Sprintf("Pembayaran %s untuk pesanan %s gagal. Silakan coba lagi.", pdf.FormatRupiah(amount), orderID),
			Data: map[string]string{
				"order_id": orderID,
				"status":   "failed",
				"amount":   fmt.Sprintf("%d", amount),
			},
		}
	}

	invalidTokens, err := f.fcm.SendToToken(ctx, tokens, payload)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	if len(invalidTokens) > 0 {
		f.log.Info("Removing invalid device tokens", zap.Strings("tokens", invalidTokens))
		if err := f.fcmRepo.DeleteDeviceToken(ctx, invalidTokens); err != nil {
			f.log.Error("failed to delete invalid device tokens", zap.Error(err))
		}
	}

	return nil
}

func (f *fcmUsecase) RegisterDevice(ctx context.Context, userID string, token string, platform string, deviceID string) error {
	dt := &domain.DeviceToken{
		Token:    token,
		DeviceID: deviceID,
		Platform: domain.Platform(platform),
	}

	if userID != "" {
		userUUID, err := uuid.Parse(userID)
		if err != nil {
			return fmt.Errorf("invalid userID: %w", err)
		}
		dt.UserID = &userUUID
	}

	if err := f.fcmRepo.RegisterDeviceToken(ctx, dt); err != nil {
		return fmt.Errorf("failed to register device token: %w", err)
	}

	return nil
}
