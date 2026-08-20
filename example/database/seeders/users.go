// Package seeders contains the example application's database seeders.
package seeders

import (
	"fmt"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
)

// userFactory builds fake users for seeding.
var userFactory = database.NewFactory(func(i int) *models.User {
	return &models.User{
		Name:      fmt.Sprintf("User %d", i),
		Email:     fmt.Sprintf("user%d@example.com", i),
		Birthdate: "1990-01-01",
	}
})

// SeedUsers creates a handful of demo users (idempotent-ish: skips when
// users already exist).
func SeedUsers() error {
	count, err := database.Query[models.User]().Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = userFactory.Create(10)
	return err
}
