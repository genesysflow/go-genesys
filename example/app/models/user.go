package models

import (
	"github.com/genesysflow/go-genesys/database"
)

// User represents a user model.
type User struct {
	database.BaseModel
	Name      string `json:"name" db:"name"`
	Email     string `json:"email" db:"email"`
	Password  string `json:"password,omitempty" db:"password"`
	Birthdate string `json:"birthdate" db:"birthdate"`
}

// TableName returns the table name for the model.
func (u *User) TableName() string {
	return "users"
}
