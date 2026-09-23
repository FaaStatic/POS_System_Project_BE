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
	log         *zap.Logger
	jwtService  *jwt.JWTService
}

func NewUserUsecase(userRepo domain.UserRepository, sessionRepo domain.SessionRepository, log *zap.Logger, jwtService *jwt.JWTService) domain.UserUsecase {
	return &userUsecase{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
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

	roleEnum, _ := enum.ParseUserRole("staff")

	user := &domain.Users{
		Username: userDto.Username,
		Password: userDto.Password,
		Role:     roleEnum,
	}
	if err := user.HashPswd(); err != nil {
		u.log.Error("Error Hashing Password", zap.Error(err))
		return nil, fmt.Errorf("failed to hash password: %w", domain.ErrInternal)
	}
	if err := u.userRepo.RegisterUser(ctx, user); err != nil {
		u.log.Error("failed to register user", zap.Error(err))
		if errors.Is(err, domain.ErrDuplicate) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to register user: %w", domain.ErrInternal)
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
	return u.issueSession(ctx, user)
}

func (u *userUsecase) issueSession(ctx context.Context, user *domain.Users) (*responsedto.UserLoginResponse, error) {
	role, err := enum.ParseUserRole(user.Role.String())
	if err != nil {
		u.log.Error("error parsing role", zap.Error(err))
		return nil, fmt.Errorf("invalid user role: %w", domain.ErrInternal)
	}
	pair, err := u.jwtService.GenerateTokenPair(user.ID.String(), role.String())
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
		Role:         role.String(),
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
		Role:         role.String(),
		Token:        pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    int64(pair.ExpiresIn),
	}, nil
}

func (u *userUsecase) Logout(ctx context.Context, accessToken string, logoutDto *requestdto.UserLogoutRequest) error {
	var failed bool

	if accessToken != "" {
		if err := u.sessionRepo.DeleteSession(ctx, "session:"+jwt.HashToken(accessToken)); err != nil {
			u.log.Error("failed to delete access session", zap.Error(err))
			failed = true
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

	session, err := u.sessionRepo.PopSessionByRefreshToken(ctx, "refresh:"+jwt.HashToken(refreshDto.RefreshToken))
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
