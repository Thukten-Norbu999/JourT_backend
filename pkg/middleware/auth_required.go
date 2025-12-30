package middleware

import (
	"net/http"

	"jourt_backend/internal/auth"

	"github.com/gin-gonic/gin"
)

const CtxUserIDKey = "user_id"

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := auth.ReadAuthCookie(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Not authenticated",
				"type":    "auth_error",
			})
			return
		}

		claims, err := auth.VerifyToken(token) // ✅ you must have VerifyToken in internal/auth
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Invalid or expired session",
				"type":    "auth_error",
			})
			return
		}

		c.Set(CtxUserIDKey, claims.UserID)
		c.Next()
	}
}

func GetUserID(c *gin.Context) uint {
	v, ok := c.Get(CtxUserIDKey)
	if !ok {
		return 0
	}
	id, _ := v.(uint)
	return id
}
