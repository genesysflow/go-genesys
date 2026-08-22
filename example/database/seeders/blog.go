package seeders

import (
	"errors"
	"fmt"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/example/database/factories"
)

// editorEmail is the fixed account the seeded blog is edited by.
const editorEmail = "editor@example.com"

// SeedBlog fills the blog with a small, realistic dataset: an editor,
// a few authors, published and draft posts, comments and tags.
func SeedBlog() error {
	// The seeder is expected to be run more than once, so the fixed
	// editor is looked up before it is created.
	editor, err := database.FirstWhere[models.User]("email", editorEmail)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("looking for the editor: %w", err)
	}
	if editor == nil {
		editor, err = factories.Editors.CreateOne(func(u *models.User) {
			u.Name = "Ada Lovelace"
			u.Email = editorEmail
		})
		if err != nil {
			return fmt.Errorf("seeding the editor: %w", err)
		}
	}

	authors, err := factories.Users.Create(3)
	if err != nil {
		return fmt.Errorf("seeding authors: %w", err)
	}

	tags, err := factories.Tags.Create(4)
	if err != nil {
		return fmt.Errorf("seeding tags: %w", err)
	}

	for i, author := range authors {
		// Every author gets two published posts and one draft, so a
		// listing has something to show and a draft to hide.
		published, err := factories.Published.Create(2, func(p *models.Post) {
			p.AuthorID = author.ID
		})
		if err != nil {
			return fmt.Errorf("seeding posts: %w", err)
		}

		if _, err := factories.Posts.CreateOne(func(p *models.Post) {
			p.AuthorID = author.ID
		}); err != nil {
			return fmt.Errorf("seeding drafts: %w", err)
		}

		for _, post := range published {
			if _, err := factories.Comments.CreateOne(func(c *models.Comment) {
				c.PostID = post.ID
				c.AuthorID = editor.ID
			}); err != nil {
				return fmt.Errorf("seeding comments: %w", err)
			}

			tag := tags[(i+int(post.ID))%len(tags)]
			if err := database.Attach(post, "Tags", tag.ID); err != nil {
				return fmt.Errorf("tagging posts: %w", err)
			}
		}
	}

	// One attachment, so the polymorphic inverse has something to resolve.
	firstPost, err := database.Query[models.Post]().OrderBy("id").First()
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		return fmt.Errorf("reading the first post: %w", err)
	}
	if firstPost != nil {
		if err := database.Create(&models.Attachment{
			AttachableType: database.TableNameFor[models.Post](),
			AttachableID:   firstPost.ID,
			Disk:           "local",
			Path:           fmt.Sprintf("posts/%d/cover.png", firstPost.ID),
			Size:           2048,
		}); err != nil {
			return fmt.Errorf("seeding attachments: %w", err)
		}
	}

	return nil
}
