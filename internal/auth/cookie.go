package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const CookieName = "jourt_auth"

func SetAuthCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,                  // ✅ MUST be true for SameSite=None
		SameSite: http.SameSiteNoneMode, // ✅ CHANGED from Lax to None
		MaxAge:   60 * 60 * 24 * 7,      // 7 days
	})
}

func ClearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,                  // ✅ MUST be true
		SameSite: http.SameSiteNoneMode, // ✅ CHANGED from Lax to None
		MaxAge:   -1,
	})
}

func ReadAuthCookie(c *gin.Context) (string, bool) {
	v, err := c.Cookie(CookieName)
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}
