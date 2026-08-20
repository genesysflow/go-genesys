package auth

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/genesysflow/go-genesys/database"
)

// ORMUserProvider retrieves users through the framework ORM:
//
//	type User struct {
//	    database.Model
//	    Email    string `db:"email" json:"email"`
//	    Password string `db:"password" json:"-"`
//	}
//
//	func (u *User) GetAuthIdentifier() any    { return u.ID }
//	func (u *User) GetAuthPassword() string   { return u.Password }
//
//	provider := auth.NewORMUserProvider[User]()
type ORMUserProvider[T any] struct {
	// TokenField is the column checked by RetrieveByToken
	// (default "api_token").
	TokenField string

	// RememberField is the column holding the remember-me token
	// (default "remember_token").
	RememberField string
}

// NewORMUserProvider creates an ORM-backed user provider. Panics unless *T
// implements Authenticatable.
func NewORMUserProvider[T any]() *ORMUserProvider[T] {
	var probe T
	if _, ok := any(&probe).(Authenticatable); !ok {
		panic(fmt.Sprintf("auth: *%T does not implement auth.Authenticatable", probe))
	}
	return &ORMUserProvider[T]{TokenField: "api_token", RememberField: "remember_token"}
}

// RetrieveByID returns the user with the given identifier.
func (p *ORMUserProvider[T]) RetrieveByID(id any) (Authenticatable, error) {
	// Session-stored identifiers arrive as strings; normalise numeric ids
	// so typed database columns match.
	if s, ok := id.(string); ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			id = n
		}
	}
	user, err := database.Find[T](id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return any(user).(Authenticatable), nil
}

// RetrieveByCredentials returns the user matching all non-password
// credential columns.
func (p *ORMUserProvider[T]) RetrieveByCredentials(credentials map[string]any) (Authenticatable, error) {
	if len(credentials) == 0 {
		return nil, ErrUserNotFound
	}
	q := database.Query[T]()
	for column, value := range credentials {
		q.Where(column, value)
	}
	user, err := q.First()
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return any(user).(Authenticatable), nil
}

// RetrieveByToken returns the user owning the given API token.
func (p *ORMUserProvider[T]) RetrieveByToken(token string) (Authenticatable, error) {
	user, err := database.FirstWhere[T](p.TokenField, token)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return any(user).(Authenticatable), nil
}

// RetrieveByRememberToken returns the user with the given id whose
// stored remember token matches.
func (p *ORMUserProvider[T]) RetrieveByRememberToken(id any, token string) (Authenticatable, error) {
	if s, ok := id.(string); ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			id = n
		}
	}
	user, err := database.Query[T]().Where("id", id).Where(p.RememberField, token).First()
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return any(user).(Authenticatable), nil
}

// UpdateRememberToken stores a new remember token for the user.
func (p *ORMUserProvider[T]) UpdateRememberToken(user Authenticatable, token string) error {
	_, err := database.Query[T]().
		Where("id", user.GetAuthIdentifier()).
		Update(map[string]any{p.RememberField: token})
	return err
}
