package usecase

import (
	"context"
	"errors"
	"fmt"
	"shop_project_be/internal/constant/enum"
	"shop_project_be/internal/domain"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"shop_project_be/pkg/jwt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("timing-equalizer-not-a-real-password"), bcrypt.DefaultCost)

var errBadCredentials = errors.New("username atau password salah")

var errInvalidRefreshToken = errors.New("invalid refresh token")

type userUsecase struct {
	userRepo    domain.UserRepository
	sessionRepo domain.SessionRepository
	deviceRepo  domain.DeviceTokenRepository
	log         *zap.Logger
	jwtService  *jwt.JWTService
}

func NewUserUsecase(userRepo domain.UserRepository, sessionRepo domain.SessionRepository, deviceRepo domain.DeviceTokenRepository, log *zap.Logger, jwtService *jwt.JWTService) domain.UserUsecase {
	return &userUsecase{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		deviceRepo:  deviceRepo,
		log:         log,
		jwtService:  jwtService,
	}
}

func (u *userUsecase) RegisterUser(ctx context.Context, userDto *requestdto.UserRegisterRequest) (*responsedto.UserRegisterResponse, error) {
	existing, err := u.userRepo.GetUserByUsername(ctx, userDto.Username)
	if err != nil {
		u.log.Error("error get user by username", zap.Error(err))
		return nil, fmt.Errorf("failed to check username: %w", domain.ErrInternal)
	}
	if existing != nil {
		return nil, domain.Duplicate("user already exists")
	}

	user := &domain.Users{
		Username: userDto.Username,
		Password: userDto.Password,
		Role:     enum.Staff,
	}
	if err := user.HashPswd(); err != nil {
		u.log.Error("Error Hashing Password", zap.Error(err))
		return nil, fmt.Errorf("failed to hash password: %w", domain.ErrInternal)
	}
	if err := u.userRepo.RegisterUser(ctx, user); err != nil {
		return nil, passOrInternal(u.log, err, "failed to register user", domain.ErrDuplicate)
	}

	return &responsedto.UserRegisterResponse{
		Message: "register success",
		Status:  201,
	}, nil
}

func (u *userUsecase) UserLogin(ctx context.Context, userDto *requestdto.UserLoginRequest) (*responsedto.UserLoginResponse, error) {
	user, err := u.userRepo.GetUserByUsername(ctx, userDto.Username)
	if err != nil {
		u.log.Error("error get user by username", zap.Error(err))
		return nil, fmt.Errorf("failed to get user: %w", domain.ErrInternal)
	}
	if user == nil {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(userDto.Password))
		u.log.Warn("login rejected: user not found")
		return nil, errBadCredentials
	}
	if !user.ComparedPwd(userDto.Password) {
		u.log.Warn("login rejected: wrong password")
		return nil, errBadCredentials
	}
	if userDto.AdminOnly && user.Role != enum.Superadmin {
		return nil, domain.Forbidden("the CMS is for superadmin accounts only; use the cashier app")
	}
	return u.issueSession(ctx, user)
}

func (u *userUsecase) issueSession(ctx context.Context, user *domain.Users) (*responsedto.UserLoginResponse, error) {
	role := user.Role.String()
	pair, err := u.jwtService.GenerateTokenPair(user.ID.String(), role)
	if err != nil {
		u.log.Error("error gen token", zap.Error(err))
		return nil, fmt.Errorf("failed to generate token: %w", domain.ErrInternal)
	}

	accessTTL := time.Duration(pair.ExpiresIn) * time.Second
	accessHash := jwt.HashToken(pair.AccessToken)
	refreshHash := jwt.HashToken(pair.RefreshToken)
	session := &domain.Session{
		AccessToken:  accessHash,
		RefreshToken: refreshHash,
		UserID:       user.ID.String(),
		Role:         role,
		ExpiresAt:    time.Now().Add(accessTTL),
	}
	if err := u.sessionRepo.CreateSession(ctx, session, "session:"+accessHash, accessTTL); err != nil {
		u.log.Error("error save session", zap.Error(err))
		return nil, fmt.Errorf("failed to save session: %w", domain.ErrInternal)
	}
	if err := u.sessionRepo.CreateSession(ctx, session, "refresh:"+refreshHash, u.jwtService.RefreshTokenTTL()); err != nil {
		u.log.Error("error save refresh session", zap.Error(err))
		_ = u.sessionRepo.DeleteSession(ctx, "session:"+accessHash)
		return nil, fmt.Errorf("failed to save session: %w", domain.ErrInternal)
	}

	return &responsedto.UserLoginResponse{
		ID:           user.ID,
		Username:     user.Username,
		Role:         role,
		Token:        pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    int64(pair.ExpiresIn),
	}, nil
}

func (u *userUsecase) Logout(ctx context.Context, accessToken string, logoutDto *requestdto.UserLogoutRequest) error {
	var failed bool

	if accessToken != "" {
		session, err := u.sessionRepo.PopSession(ctx, "session:"+jwt.HashToken(accessToken))
		if err != nil {
			u.log.Error("failed to delete access session", zap.Error(err))
			failed = true
		}
		if session != nil && session.RefreshToken != "" {
			if err := u.sessionRepo.DeleteSession(ctx, "refresh:"+session.RefreshToken); err != nil {
				u.log.Error("failed to delete refresh session", zap.Error(err))
				failed = true
			}
		}
	}

	if logoutDto != nil && logoutDto.RefreshToken != "" {
		if err := u.sessionRepo.DeleteSession(ctx, "refresh:"+jwt.HashToken(logoutDto.RefreshToken)); err != nil {
			u.log.Error("failed to delete refresh session", zap.Error(err))
			failed = true
		}
	}

	if failed {
		return fmt.Errorf("failed to revoke session: %w", domain.ErrInternal)
	}
	return nil
}

func (u *userUsecase) RefreshToken(ctx context.Context, refreshDto *requestdto.UserRefreshTokenRequest) (*responsedto.UserLoginResponse, error) {
	claims, err := u.jwtService.ValidateToken(refreshDto.RefreshToken)
	if err != nil || claims.Type != "refresh" {
		return nil, errInvalidRefreshToken
	}

	session, err := u.sessionRepo.PopSession(ctx, "refresh:"+jwt.HashToken(refreshDto.RefreshToken))
	if err != nil {
		u.log.Error("error pop refresh session", zap.Error(err))
		return nil, fmt.Errorf("failed to read refresh session: %w", domain.ErrInternal)
	}
	if session == nil {
		return nil, errInvalidRefreshToken
	}

	userID, err := uuid.Parse(session.UserID)
	if err != nil {
		return nil, errInvalidRefreshToken
	}
	user, err := u.userRepo.GetUserById(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errInvalidRefreshToken
		}
		u.log.Error("error get user", zap.Error(err))
		return nil, fmt.Errorf("failed to get user: %w", domain.ErrInternal)
	}

	if session.AccessToken != "" {
		if err := u.sessionRepo.DeleteSession(ctx, "session:"+session.AccessToken); err != nil {
			u.log.Error("error delete old session", zap.Error(err))
		}
	}

	return u.issueSession(ctx, user)
}

func toStaffResponse(user *domain.Users) responsedto.StaffResponse {
	return responsedto.StaffResponse{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role.String(),
		Active:    !user.DeletedAt.Valid,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}
}

func (u *userUsecase) ListStaff(ctx context.Context, request *requestdto.ListStaffRequest) (*responsedto.ListStaffResponse, error) {
	cursor, err := parseCursor(request.AfterID, request.AfterTime)
	if err != nil {
		return nil, err
	}
	result, err := u.userRepo.ListUsers(ctx, domain.FilterUser{
		Search:   request.Search,
		Inactive: request.Status == "inactive",
		Cursor:   cursor,
		Limit:    request.Limit,
		Order:    request.Order,
	})
	if err != nil {
		u.log.Error("failed to list users", zap.Error(err))
		return nil, fmt.Errorf("failed to list staff: %w", domain.ErrInternal)
	}

	staff := make([]responsedto.StaffResponse, 0, len(result.DataItem))
	for _, item := range result.DataItem {
		staff = append(staff, toStaffResponse(item))
	}
	nextID, nextTime := result.Cursor.Encode()
	return &responsedto.ListStaffResponse{AfterId: nextID, AfterTime: nextTime, HasNext: result.HasNext, Staff: staff}, nil
}

// staffTarget memuat akun yang akan dikelola dari CMS. Akun superadmin dikelola
// lewat perintah create-admin, bukan dari CMS, supaya admin tidak bisa mengunci
// dirinya sendiri keluar.
func (u *userUsecase) staffTarget(ctx context.Context, rawID string) (*domain.Users, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return nil, domain.InvalidID("invalid user id format")
	}
	user, err := u.userRepo.GetUserAnyState(ctx, id)
	if err != nil {
		return nil, passOrInternal(u.log, err, "failed to get user", domain.ErrNotFound)
	}
	if user.Role != enum.Staff {
		return nil, domain.Forbidden("superadmin accounts cannot be managed from the CMS")
	}
	return user, nil
}

// revokeAccess mencabut semua sesi dan token push milik user. Dipanggil setelah
// perubahan DB berhasil; kegagalan di sini dilaporkan supaya admin bisa mengulang.
func (u *userUsecase) revokeAccess(ctx context.Context, userID uuid.UUID) error {
	if err := u.sessionRepo.RevokeUserSessions(ctx, userID.String()); err != nil {
		u.log.Error("failed to revoke user sessions", zap.Error(err), zap.String("user_id", userID.String()))
		return fmt.Errorf("failed to revoke sessions: %w", domain.ErrInternal)
	}
	if err := u.deviceRepo.DeleteDeviceTokensByUser(ctx, userID); err != nil {
		u.log.Error("failed to delete device tokens", zap.Error(err), zap.String("user_id", userID.String()))
		return fmt.Errorf("failed to delete device tokens: %w", domain.ErrInternal)
	}
	return nil
}

func (u *userUsecase) UpdateStaff(ctx context.Context, request *requestdto.UpdateStaffRequest) error {
	user, err := u.staffTarget(ctx, request.ID)
	if err != nil {
		return err
	}

	fields := map[string]any{}
	if request.Username != nil {
		fields["username"] = strings.TrimSpace(*request.Username)
	}
	if request.Password != nil {
		hashed := &domain.Users{Password: *request.Password}
		if err := hashed.HashPswd(); err != nil {
			u.log.Error("failed to hash password", zap.Error(err))
			return fmt.Errorf("failed to hash password: %w", domain.ErrInternal)
		}
		fields["password"] = hashed.Password
	}
	if len(fields) == 0 {
		return domain.Validation("no fields to update")
	}

	if err := u.userRepo.UpdateUser(ctx, user.ID, fields); err != nil {
		return passOrInternal(u.log, err, "failed to update staff", domain.ErrNotFound, domain.ErrDuplicate)
	}
	// Password baru harus mengeluarkan device yang masih memakai password lama.
	if request.Password != nil {
		if err := u.sessionRepo.RevokeUserSessions(ctx, user.ID.String()); err != nil {
			u.log.Error("failed to revoke sessions after password change", zap.Error(err))
			return fmt.Errorf("password changed but sessions were not revoked: %w", domain.ErrInternal)
		}
	}
	return nil
}

func (u *userUsecase) DeactivateStaff(ctx context.Context, id string) error {
	user, err := u.staffTarget(ctx, id)
	if err != nil {
		return err
	}
	if err := u.userRepo.DeactivateUser(ctx, user.ID); err != nil {
		return passOrInternal(u.log, err, "failed to deactivate staff", domain.ErrNotFound)
	}
	return u.revokeAccess(ctx, user.ID)
}

func (u *userUsecase) RestoreStaff(ctx context.Context, id string) error {
	user, err := u.staffTarget(ctx, id)
	if err != nil {
		return err
	}
	if err := u.userRepo.RestoreUser(ctx, user.ID); err != nil {
		return passOrInternal(u.log, err, "failed to restore staff", domain.ErrNotFound)
	}
	return nil
}

func (u *userUsecase) PurgeStaff(ctx context.Context, request *requestdto.PurgeStaffRequest) (*responsedto.PurgeResponse, error) {
	user, err := u.staffTarget(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Confirm) != user.Username {
		return nil, domain.Validation("confirm must match the staff username")
	}

	// Password acak yang tidak pernah diberikan ke siapa pun: akun anonim tidak
	// bisa login lagi. Username tetap unik karena memakai potongan id.
	locked := &domain.Users{Password: uuid.NewString() + uuid.NewString()}
	if err := locked.HashPswd(); err != nil {
		u.log.Error("failed to hash placeholder password", zap.Error(err))
		return nil, fmt.Errorf("failed to delete staff: %w", domain.ErrInternal)
	}
	anonymized, err := u.userRepo.PurgeUser(ctx, user.ID, map[string]any{
		"username": "staff-terhapus-" + user.ID.String()[:8],
		"password": locked.Password,
	})
	if err != nil {
		return nil, passOrInternal(u.log, err, "failed to delete staff", domain.ErrNotFound, domain.ErrConflict)
	}
	if err := u.sessionRepo.RevokeUserSessions(ctx, user.ID.String()); err != nil {
		u.log.Error("failed to revoke sessions after purge", zap.Error(err))
	}

	return &responsedto.PurgeResponse{Mode: purgeMode(anonymized)}, nil
}
