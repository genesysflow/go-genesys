package migrations

import "github.com/genesysflow/go-genesys/database/schema"

// CreateUsersTable migration.
type CreateUsersTable struct{}

// Name returns the migration name.
func (m *CreateUsersTable) Name() string {
	return "2025_12_15_155333_create_users_table"
}

// Up runs the migration.
func (m *CreateUsersTable) Up(builder *schema.Builder) error {
	return builder.Create("users", func(table *schema.Blueprint) {
		table.ID()
		table.String("name", 255)
		table.String("email", 255).Unique()
		table.String("password", 255)
		table.Timestamps()
	})
}

// Down reverses the migration.
func (m *CreateUsersTable) Down(builder *schema.Builder) error {
	return builder.Drop("users")
}
