// Package factories builds example models for seeders and tests.
package factories

import (
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/hash"
	"github.com/genesysflow/go-genesys/support"
	"github.com/genesysflow/go-genesys/support/faker"
)

// fake generates values. Seeded, so a seeded database is reproducible.
var fake = faker.NewSeeded(1)

// Password is the plaintext every factory-made user signs in with.
const Password = "password123"

// Users builds authors. Their password is always Password, so a test can
// sign in as one without reaching for the hash.
var Users = database.NewFactory(func(i int) *models.User {
	hashed, _ := hash.Make(Password)

	// The address carries a random component: a factory is used by
	// seeders that run more than once and by tests that run in parallel,
	// and a unique column is unforgiving of either.
	return &models.User{
		Name:      fake.Name(),
		Email:     fmt.Sprintf("author-%d-%s@example.com", i, support.Str.Random(6)),
		Password:  hashed,
		Birthdate: "1990-01-01",
		Role:      "author",
	}
})

// Editors may act on anyone's post.
var Editors = Users.State(func(u *models.User) { u.Role = "editor" })

// Posts builds drafts: a post is unpublished until something publishes it.
var Posts = database.NewFactory(func(i int) *models.Post {
	title := fake.Sentence(4)

	return &models.Post{
		Title: title,
		Slug:  fmt.Sprintf("%s-%d-%s", support.Str.Slug(title), i, support.Str.Random(4)),
		Body:  fake.Paragraph(3),
	}
})

// Published posts are visible to readers.
var Published = Posts.State(func(p *models.Post) {
	published := time.Now().Add(-time.Hour)
	p.PublishedAt = &published
})

// Comments builds replies.
var Comments = database.NewFactory(func(i int) *models.Comment {
	return &models.Comment{Body: fake.Sentence(8)}
})

// Tags builds labels.
var Tags = database.NewFactory(func(i int) *models.Tag {
	name := fake.Word()
	return &models.Tag{
		Name: name,
		Slug: fmt.Sprintf("%s-%d-%s", support.Str.Slug(name), i, support.Str.Random(4)),
	}
})
