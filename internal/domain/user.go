package domain

import (
	"context"
	"shop_project_be/internal/constant/enum"
	requestdto "shop_project_be/internal/dto/request_dto"
	responsedto "shop_project_be/internal/dto/response_dto"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Users struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"username"`
	Password     string         `gorm:"type:varchar(255);not null" json:"-"`
	Role         enum.UserRole  `gorm:"type:smallint;check:role IN (0,1)" json:"role"`
	Transactions []Transactions `gorm:"foreignKey:UserID" json:"transactions,omitempty"`
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (user *Users) HashPswd() error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashed)
	return nil
}

func (user *Users) ComparedPwd(pwdUser string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(pwdUser))
	return err == nil
}

func (u *Users) TableName() string {
	return "users"
}

type UserRepository interface {
	RegisterUser(ctx context.Context, user *Users) error
	GetUserByUsername(ctx context.Context, username string) (*Users, error)
	GetUserById(ctx context.Context, id uuid.UUID) (*Users, error)
}

type UserUsecase interface {
	UserLogin(ctx context.Context, userDto *requestdto.UserLoginRequest) (*responsedto.UserLoginResponse, error)
	RegisterUser(ctx context.Context, userDto *requestdto.UserRegisterRequest) (*responsedto.UserRegisterResponse, error)
	RefreshToken(ctx context.Context, refreshDto *requestdto.UserRefreshTokenRequest) (*responsedto.UserLoginResponse, error)
	Logout(ctx context.Context, accessToken string, logoutDto *requestdto.UserLogoutRequest) error
}
