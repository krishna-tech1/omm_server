package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"one-more-mile/server/internal/config"
)

const ClaimsKey = "authClaims"

const claimsKey = ClaimsKey

type Claims struct {
	UserID uuid.UUID `json:"uid"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

func GenerateToken(cfg config.Config, userID uuid.UUID, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(cfg.TokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JWTSecret))
}

func BearerToken(header string) string {
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

func ParseToken(cfg config.Config, tokenValue string) (Claims, error) {
	token, err := jwt.ParseWithClaims(tokenValue, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil || token == nil || !token.Valid {
		return Claims{}, jwt.ErrTokenInvalidClaims
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return Claims{}, jwt.ErrTokenInvalidClaims
	}

	return *claims, nil
}

func Auth(cfg config.Config, redisClient *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenValue := BearerToken(c.Get("Authorization"))
		if tokenValue == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authorization header"})
		}

		claims, err := ParseToken(cfg, tokenValue)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
		}

		if redisClient != nil {
			isBanned, _ := redisClient.Exists(c.Context(), "banned:"+claims.UserID.String()).Result()
			if isBanned > 0 {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "user is banned"})
			}
		}

		c.Locals(claimsKey, claims)
		return c.Next()
	}
}

func GetClaims(c *fiber.Ctx) (Claims, bool) {
	value := c.Locals(claimsKey)
	claims, ok := value.(Claims)
	return claims, ok
}
