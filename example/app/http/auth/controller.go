package auth

import (
	"fmt"

	genesysauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/hash"
	"github.com/genesysflow/go-genesys/http"
)

// Controller drives the login, registration and password-reset flows.
// It is scaffolding: edit it freely, it is your code now.
//
//	controller := &auth.Controller{
//	    Guard:  sessionGuard,
//	    Users:  userProvider,
//	    Create: func(email, hashed string) (genesysauth.Authenticatable, error) {
//	        user := &models.User{Email: email, Password: hashed}
//	        return user, database.Create(user)
//	    },
//	}
//	controller.Routes(router)
type Controller struct {
	// Guard logs users in and out.
	Guard genesysauth.StatefulGuard

	// Users looks users up, for the password-reset flow.
	Users genesysauth.UserProvider

	// Create persists a newly registered user. The framework does not
	// know your user model, so registration goes through here.
	Create func(email, hashedPassword string) (genesysauth.Authenticatable, error)

	// Passwords issues and consumes password-reset tokens. Leave it nil
	// to disable the reset flow.
	Passwords *genesysauth.PasswordBroker

	// Home is where a freshly authenticated user lands.
	Home string
}

// home returns the post-login destination.
func (c *Controller) home() string {
	if c.Home == "" {
		return "/"
	}
	return c.Home
}

// ShowLogin renders the login form.
func (c *Controller) ShowLogin(ctx *http.Context) error {
	return ctx.View("auth.login")
}

// Login authenticates a user and starts their session.
func (c *Controller) Login(ctx *http.Context) error {
	req, err := http.ValidateRequest[LoginRequest](ctx)
	if err != nil {
		return err
	}

	user, err := c.Guard.Attempt(ctx, map[string]any{
		"email":    req.Email,
		"password": req.Password,
	})
	if err != nil || user == nil {
		// The same message whatever failed: saying which half was wrong
		// tells an attacker which emails are registered.
		return ctx.Back("/login").
			WithErrors(map[string][]string{"email": {"These credentials do not match our records."}}).
			WithInput().
			Send()
	}

	// A fresh session id after login, so a session fixed before it
	// cannot be used to ride the new session.
	if session := ctx.Session(); session != nil {
		if err := session.Regenerate(); err != nil {
			return err
		}
	}

	return ctx.RedirectTo(c.home()).Send()
}

// Logout ends the session.
func (c *Controller) Logout(ctx *http.Context) error {
	if err := c.Guard.Logout(ctx); err != nil {
		return err
	}
	if session := ctx.Session(); session != nil {
		if err := session.Regenerate(); err != nil {
			return err
		}
	}
	return ctx.RedirectTo("/").Send()
}

// ShowRegister renders the registration form.
func (c *Controller) ShowRegister(ctx *http.Context) error {
	return ctx.View("auth.register")
}

// Register creates a user and logs them in.
func (c *Controller) Register(ctx *http.Context) error {
	req, err := http.ValidateRequest[RegisterRequest](ctx)
	if err != nil {
		return err
	}
	if c.Create == nil {
		return fmt.Errorf("auth: set Controller.Create to persist new users")
	}

	hashed, err := hash.Make(req.Password)
	if err != nil {
		return err
	}

	user, err := c.Create(req.Email, hashed)
	if err != nil {
		return err
	}

	if err := c.Guard.Login(ctx, user); err != nil {
		return err
	}
	if session := ctx.Session(); session != nil {
		if err := session.Regenerate(); err != nil {
			return err
		}
	}

	return ctx.RedirectTo(c.home()).Send()
}

// ShowForgotPassword renders the "email me a reset link" form.
func (c *Controller) ShowForgotPassword(ctx *http.Context) error {
	return ctx.View("auth.forgot_password")
}

// SendResetLink issues a reset token. Wire up the mail yourself: the
// token is what goes into the link.
func (c *Controller) SendResetLink(ctx *http.Context) error {
	req, err := http.ValidateRequest[ForgotPasswordRequest](ctx)
	if err != nil {
		return err
	}
	if c.Passwords == nil {
		return fmt.Errorf("auth: set Controller.Passwords to enable password resets")
	}

	// The token is issued whether or not the address is registered, and
	// the answer is the same either way: a different response would
	// disclose who has an account here.
	if _, err := c.Passwords.CreateToken(req.Email); err != nil {
		// TODO: mail the reset link to req.Email.
		_ = err
	}

	return ctx.Back("/forgot-password").
		With("status", "If that address is registered, a reset link is on its way.").
		Send()
}

// ShowResetPassword renders the new-password form.
func (c *Controller) ShowResetPassword(ctx *http.Context) error {
	return ctx.View("auth.reset_password", map[string]any{
		"token": ctx.Query("token"),
		"email": ctx.Query("email"),
	})
}

// ResetPassword consumes a reset token and stores the new password.
func (c *Controller) ResetPassword(ctx *http.Context) error {
	req, err := http.ValidateRequest[ResetPasswordRequest](ctx)
	if err != nil {
		return err
	}
	if c.Passwords == nil {
		return fmt.Errorf("auth: set Controller.Passwords to enable password resets")
	}

	err = c.Passwords.Consume(req.Email, req.Token, func() error {
		hashed, err := hash.Make(req.Password)
		if err != nil {
			return err
		}
		// TODO: store the hashed password on your user model.
		_ = hashed
		return nil
	})
	if err != nil {
		return ctx.Back("/reset-password").
			WithErrors(map[string][]string{"email": {"This password reset token is invalid."}}).
			Send()
	}

	return ctx.RedirectTo("/login").
		With("status", "Your password has been reset.").
		Send()
}
