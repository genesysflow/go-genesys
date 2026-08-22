// Package models contains the example application's database models.
package models

import (
	"github.com/genesysflow/go-genesys/database"
)

// User is the example user model, managed by the framework ORM.
type User struct {
	database.Model
	Name      string `db:"name" json:"name"`
	Email     string `db:"email" json:"email"`
	Password  string `db:"password" json:"-"`
	Birthdate string `db:"birthdate" json:"birthdate"`
	Role      string `db:"role" json:"role"`

	Posts []*Post `rel:"hasMany,fk:author_id" json:"posts,omitempty"`
}

// GetAuthIdentifier satisfies auth.Authenticatable.
func (u *User) GetAuthIdentifier() any { return u.ID }

// GetAuthPassword satisfies auth.Authenticatable.
func (u *User) GetAuthPassword() string { return u.Password }

// IsEditor reports whether the user may act on anyone's post.
func (u *User) IsEditor() bool { return u.Role == "editor" }

// Hidden keeps the password hash out of anything serialized from the
// model, whatever the caller does with it.
func (u *User) Hidden() []string { return []string{"password"} }
