package responsedto

import "github.com/google/uuid"

type UserLoginResponse struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	Token        string    `json:"token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresIn    int64     `json:"expires_in,omitempty"`
}

type UserRegisterResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}
