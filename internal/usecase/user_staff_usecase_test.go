package usecase

import (
	"context"
	"errors"
	"testing"

	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type fakeStaffUserRepo struct {
	domain.UserRepository
	user        *domain.Users
	deactivated bool
	purgeFields map[string]any
}

func (f *fakeStaffUserRepo) GetUserAnyState(ctx context.Context, id uuid.UUID) (*domain.Users, error) {
	return f.user, nil
}

func (f *fakeStaffUserRepo) DeactivateUser(ctx context.Context, id uuid.UUID) error {
	f.deactivated = true
	return nil
}

func (f *fakeStaffUserRepo) PurgeUser(ctx context.Context, id uuid.UUID, anon map[string]any) (bool, error) {
	f.purgeFields = anon
	return true, nil
}

type fakeStaffSessionRepo struct {
	domain.SessionRepository
	revoked []string
}

func (f *fakeStaffSessionRepo) RevokeUserSessions(ctx context.Context, userID string) error {
	f.revoked = append(f.revoked, userID)
	return nil
}

type fakeStaffDeviceRepo struct {
	domain.DeviceTokenRepository
	cleared []uuid.UUID
}

func (f *fakeStaffDeviceRepo) DeleteDeviceTokensByUser(ctx context.Context, userID uuid.UUID) error {
	f.cleared = append(f.cleared, userID)
	return nil
}

func newStaffUsecase(user *domain.Users) (*userUsecase, *fakeStaffUserRepo, *fakeStaffSessionRepo, *fakeStaffDeviceRepo) {
	users := &fakeStaffUserRepo{user: user}
	sessions := &fakeStaffSessionRepo{}
	devices := &fakeStaffDeviceRepo{}
	return &userUsecase{userRepo: users, sessionRepo: sessions, deviceRepo: devices, log: zap.NewNop()}, users, sessions, devices
}

func TestDeactivateStaff_RevokesSessionsAndPushTokens(t *testing.T) {
	staff := &domain.Users{ID: uuid.New(), Username: "kasir1", Role: enum.Staff}
	u, users, sessions, devices := newStaffUsecase(staff)

	if err := u.DeactivateStaff(context.Background(), staff.ID.String()); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if !users.deactivated || len(sessions.revoked) != 1 || len(devices.cleared) != 1 {
		t.Errorf("deactivated=%v revoked=%v cleared=%v; want all three done", users.deactivated, sessions.revoked, devices.cleared)
	}
}

func TestStaffManagement_RefusesSuperadmin(t *testing.T) {
	admin := &domain.Users{ID: uuid.New(), Username: "owner", Role: enum.Superadmin}
	u, users, _, _ := newStaffUsecase(admin)

	if err := u.DeactivateStaff(context.Background(), admin.ID.String()); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("deactivate superadmin: err = %v; want forbidden", err)
	}
	if users.deactivated {
		t.Error("superadmin was deactivated")
	}
}

func TestPurgeStaff_RequiresMatchingConfirmation(t *testing.T) {
	staff := &domain.Users{ID: uuid.New(), Username: "kasir1", Role: enum.Staff}
	u, users, _, _ := newStaffUsecase(staff)

	_, err := u.PurgeStaff(context.Background(), &requestdto.PurgeStaffRequest{ID: staff.ID.String(), Confirm: "kasir2"})
	if !errors.Is(err, domain.ErrValidation) || users.purgeFields != nil {
		t.Fatalf("wrong confirmation: err = %v, purged = %v; want validation error and no purge", err, users.purgeFields != nil)
	}

	resp, err := u.PurgeStaff(context.Background(), &requestdto.PurgeStaffRequest{ID: staff.ID.String(), Confirm: "kasir1"})
	if err != nil || resp.Mode != "anonymized" {
		t.Fatalf("purge: resp=%+v err=%v; want anonymized", resp, err)
	}
	if users.purgeFields["username"] == "kasir1" || users.purgeFields["password"] == "" {
		t.Errorf("anon fields keep identity or leave password empty: %v", users.purgeFields)
	}
}
