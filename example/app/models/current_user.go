package models

import (
	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/http"
)

// CurrentUser returns the signed-in user for the request, or nil for a
// guest. It is this application's `$request->user()`: the framework
// stores the guard-resolved user as an interface, and this is the one
// place the example narrows it to *User.
//
//	if user := models.CurrentUser(ctx); user != nil {
//	    // ...
//	}
func CurrentUser(ctx *http.Context) *User {
	user, _ := http.UserAs[User](ctx)
	return user
}

// AsUser narrows an auth.Authenticatable - what gates, policies and the
// password broker are handed - to *User. ok is false for a guest and for
// a user model this application does not own, so a rule that only makes
// sense for our own users can say so in one line:
//
//	author, ok := models.AsUser(user)
//	if !ok {
//	    return false
//	}
func AsUser(user auth.Authenticatable) (*User, bool) {
	account, ok := user.(*User)
	if !ok || account == nil {
		return nil, false
	}
	return account, true
}
