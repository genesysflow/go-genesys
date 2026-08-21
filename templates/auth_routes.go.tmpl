package auth

import (
	genesysauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/http"
)

// Routes registers the authentication routes. Guests reach the forms;
// logout needs a session.
func (c *Controller) Routes(router *http.Router) {
	guest := genesysauth.GuestMiddleware(c.Guard, c.home())

	router.GET("/login", c.ShowLogin, guest).Name("login")
	router.POST("/login", c.Login, guest)

	router.GET("/register", c.ShowRegister, guest).Name("register")
	router.POST("/register", c.Register, guest)

	router.POST("/logout", c.Logout).Name("logout")

	router.GET("/forgot-password", c.ShowForgotPassword, guest).Name("password.request")
	router.POST("/forgot-password", c.SendResetLink, guest)

	router.GET("/reset-password", c.ShowResetPassword, guest).Name("password.reset")
	router.POST("/reset-password", c.ResetPassword, guest)
}
