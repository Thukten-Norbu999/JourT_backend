package auth

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

const CookieName = "jourt_auth"

func SetAuthCookie(c *gin.Context, token string) {
	secure := os.Getenv("GIN_MODE") == "release" // true in prod (https), false in dev

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,                 // ✅ JS cannot read it
		Secure:   secure,               // ✅ true only on HTTPS
		SameSite: http.SameSiteLaxMode, // ✅ works across localhost ports
		MaxAge:   60 * 60 * 24 * 7,     // 7 days
	})
}

func ClearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("GIN_MODE") == "release",
		SameSite: http.SameSiteLaxMode,
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
