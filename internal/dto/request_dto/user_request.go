package requestdto

type UserLoginRequest struct {
	Username string `json:"username" validate:"required,min=3,max=100"`
	Password string `json:"password" validate:"required,min=6"`
}

type UserRegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=100"`
	Password string `json:"password" validate:"required,min=6"`
}

type UserRefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type UserLogoutRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
	FcmToken     string `json:"fcm_token,omitempty"`
}
