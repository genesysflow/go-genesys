package migrations

import (
	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/queue"
)

// CreateBlogTables creates everything the blog needs: posts, comments,
// tags with their polymorphic pivot, attachments, and the framework
// tables the example uses (queue, notifications, password resets and API
// tokens).
type CreateBlogTables struct{}

// Name returns the migration name.
func (m *CreateBlogTables) Name() string {
	return "2026_08_01_000000_create_blog_tables"
}

// Up runs the migration.
func (m *CreateBlogTables) Up(builder *schema.Builder) error {
	if err := builder.Create("posts", func(table *schema.Blueprint) {
		table.ID()
		table.ForeignID("author_id")
		table.String("title", 255)
		table.String("slug", 255).Unique()
		table.Text("body")
		table.Timestamp("published_at").Nullable()
		table.Timestamps()
		table.SoftDeletes()
		table.Index("author_id")
	}); err != nil {
		return err
	}

	if err := builder.Create("comments", func(table *schema.Blueprint) {
		table.ID()
		table.ForeignID("post_id")
		table.ForeignID("author_id")
		table.Text("body")
		table.Timestamps()
		table.Index("post_id")
	}); err != nil {
		return err
	}

	if err := builder.Create("tags", func(table *schema.Blueprint) {
		table.ID()
		table.String("name", 100)
		table.String("slug", 100).Unique()
		table.Timestamps()
	}); err != nil {
		return err
	}

	// The polymorphic pivot: a tag can label a post today and anything
	// else tomorrow without a second table.
	if err := builder.Create("taggables", func(table *schema.Blueprint) {
		table.BigInteger("tag_id")
		table.BigInteger("taggable_id")
		table.String("taggable_type", 100)
		table.Index("taggable_type", "taggable_id")
	}); err != nil {
		return err
	}

	if err := builder.Create("attachments", func(table *schema.Blueprint) {
		table.ID()
		table.String("attachable_type", 100)
		table.BigInteger("attachable_id")
		table.String("disk", 50)
		table.String("path", 255)
		table.BigInteger("size")
		table.Timestamps()
		table.Index("attachable_type", "attachable_id")
	}); err != nil {
		return err
	}

	if err := builder.Create("notifications", func(table *schema.Blueprint) {
		table.ID()
		table.String("notifiable_id", 100)
		table.String("name", 255)
		table.Text("data")
		table.Timestamp("read_at").Nullable()
		table.Timestamp("created_at").Nullable()
		table.Index("notifiable_id")
	}); err != nil {
		return err
	}

	// One live reset token per address, which is what the broker's
	// delete-then-insert assumes.
	if err := builder.Create("password_reset_tokens", func(table *schema.Blueprint) {
		table.String("email", 255).Unique()
		table.String("token", 64)
		table.Timestamp("created_at").Nullable()
	}); err != nil {
		return err
	}

	if err := auth.CreatePersonalAccessTokensTable(builder); err != nil {
		return err
	}

	return queue.CreateJobsTables(builder)
}

// Down reverses the migration.
func (m *CreateBlogTables) Down(builder *schema.Builder) error {
	if err := queue.DropJobsTables(builder); err != nil {
		return err
	}
	if err := auth.DropPersonalAccessTokensTable(builder); err != nil {
		return err
	}

	for _, table := range []string{
		"password_reset_tokens", "notifications", "attachments",
		"taggables", "tags", "comments", "posts",
	} {
		if err := builder.DropIfExists(table); err != nil {
			return err
		}
	}
	return nil
}
