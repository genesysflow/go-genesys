package migrations

import "github.com/genesysflow/go-genesys/database/schema"

// AddRoleToUsers gives users a role, which the post policy reads.
type AddRoleToUsers struct{}

// Name returns the migration name.
func (m *AddRoleToUsers) Name() string {
	return "2026_08_01_000100_add_role_to_users"
}

// Up runs the migration.
func (m *AddRoleToUsers) Up(builder *schema.Builder) error {
	return builder.Table("users", func(table *schema.Blueprint) {
		table.AddString("role", 20).Default("author")
	})
}

// Down reverses the migration.
func (m *AddRoleToUsers) Down(builder *schema.Builder) error {
	return builder.Table("users", func(table *schema.Blueprint) {
		table.DropColumn("role")
	})
}
