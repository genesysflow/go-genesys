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
}
