package auth

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

func jwtSecret() []byte {
	sec := os.Getenv("JWT_SECRET")
	if sec == "" {
		sec = "dev_secret_change_me" // ✅ ok for local dev ONLY
	}
	return []byte(sec)
}

// GenerateToken creates JWT valid for 7 days
func GenerateToken(userID uint, email string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			// Optional but nice:
			NotBefore: jwt.NewNumericDate(time.Now().Add(-10 * time.Second)),
		},
	}

	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(jwtSecret())
}

// VerifyToken validates token and returns claims
func VerifyToken(tokenString string) (*Claims, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("missing token")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		// ✅ Security: ensure HS256/HMAC only
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	// ✅ extra safety (rare edge)
	if claims.ExpiresAt == nil || time.Now().After(claims.ExpiresAt.Time) {
		return nil, errors.New("token expired")
	}

	return claims, nil
}

// ParseToken alias (optional convenience if any code still calls ParseToken)
func ParseToken(tokenString string) (*Claims, error) {
	return VerifyToken(tokenString)
}

// ✅ Postman-friendly: Authorization: Bearer <token>
func ReadBearerToken(authHeader string) (string, bool) {
	if authHeader == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		tok := strings.TrimSpace(authHeader[7:])
		if tok == "" {
			return "", false
		}
		return tok, true
	}
	return "", false
}
