package middleware

import (
	"shop_project_be/internal/domain"
	"shop_project_be/pkg/jwt"
	"shop_project_be/pkg/response"
	"strings"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type JWTMiddleware struct {
	jwtService  *jwt.JWTService
	sessionRepo domain.SessionRepository
}

func NewJwtMiddleware(jwtService *jwt.JWTService, sessionRepo domain.SessionRepository) *JWTMiddleware {
	return &JWTMiddleware{
		jwtService:  jwtService,
		sessionRepo: sessionRepo,
	}
}

func (m *JWTMiddleware) Auth(log *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		token := extractToken(c)
		if token == "" {
			return response.Error(c, fiber.StatusUnauthorized, "token not found", nil)
		}
		claims, err := m.jwtService.ValidateToken(token)
		if err != nil {
			log.Debug("token not valid", zap.Error(err))
			return response.Error(c, fiber.StatusUnauthorized, "token not valid or expired", err)
		}
		if claims.Type != "access" {
			return response.Error(c, fiber.StatusUnauthorized, "have not access token", nil)
		}

		ctx := c.Context()
		sessionKey := "session:" + jwt.HashToken(token)
		exists, err := m.sessionRepo.Exists(ctx, sessionKey)
		if err != nil || !exists {
			return response.Error(c, fiber.StatusUnauthorized, "Session not valid user please login first!", nil)
		}
		c.Locals("user_id", claims.UserID)
		c.Locals("role", claims.Role)
		c.Locals("access_token", token)

		return c.Next()
	}
}

func (m *JWTMiddleware) RequireRole(roles ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		role, ok := c.Locals("role").(string)
		if !ok {
			return response.Error(c, fiber.StatusForbidden, "role not found!", nil)
		}

		for _, r := range roles {
			if r == role {
				return c.Next()
			}
		}

		return response.Error(c, fiber.StatusForbidden, "akses ditolak", nil)
	}
}

func extractToken(c fiber.Ctx) string {
	auth := c.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
func GetUserID(c fiber.Ctx) string {
	id, _ := c.Locals("user_id").(string)
	return id
}
