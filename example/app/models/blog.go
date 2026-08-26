package models

import (
	"time"

	"github.com/genesysflow/go-genesys/database"
)

// Post is an article written by a user. It carries the relations the
// example exercises: an author (belongsTo), comments (hasMany), tags
// (morphToMany) and attachments (morphMany).
type Post struct {
	database.Model
	AuthorID    int64      `db:"author_id" json:"author_id"`
	Title       string     `db:"title" json:"title"`
	Slug        string     `db:"slug" json:"slug"`
	Body        string     `db:"body" json:"body"`
	PublishedAt *time.Time `db:"published_at" json:"published_at"`
	DeletedAt   *time.Time `db:"deleted_at" json:"-"`

	Author        *User         `rel:"belongsTo,fk:author_id" json:"author,omitempty"`
	Comments      []*Comment    `rel:"hasMany,fk:post_id" json:"comments,omitempty"`
	Tags          []Tag         `rel:"morphToMany,as:taggable,prk:tag_id" json:"tags,omitempty"`
	Attachments   []*Attachment `rel:"morphMany,as:attachable" json:"attachments,omitempty"`
	CommentsCount int64         `db:"-" json:"comments_count"`
}

// Published reports whether the post is visible to readers.
func (p *Post) Published() bool {
	return p.PublishedAt != nil && !p.PublishedAt.After(time.Now())
}

// Hidden keeps the soft-delete column out of API responses.
func (p *Post) Hidden() []string { return []string{"deleted_at"} }

// Appends adds computed fields to the serialized form.
func (p *Post) Appends() map[string]any {
	return map[string]any{
		"published": p.Published(),
		"excerpt":   excerpt(p.Body, 60),
	}
}

// excerpt truncates a body for listings.
func excerpt(body string, limit int) string {
	runes := []rune(body)
	if len(runes) <= limit {
		return body
	}
	return string(runes[:limit]) + "..."
}

// Comment is a reader's response to a post.
type Comment struct {
	database.Model
	PostID   int64  `db:"post_id" json:"post_id"`
	AuthorID int64  `db:"author_id" json:"author_id"`
	Body     string `db:"body" json:"body"`

	Post        *Post         `rel:"belongsTo,fk:post_id" json:"-"`
	Author      *User         `rel:"belongsTo,fk:author_id" json:"author,omitempty"`
	Attachments []*Attachment `rel:"morphMany,as:attachable" json:"attachments,omitempty"`
}

// Tag labels posts. It is attached polymorphically, so the same tag can
// later label other models without a second pivot table.
type Tag struct {
	database.Model
	Name string `db:"name" json:"name"`
	Slug string `db:"slug" json:"slug"`
}

// Attachment is a file belonging to whatever attached it - a post or a
// comment. The inverse relation resolves the owner from its type column.
type Attachment struct {
	database.Model
	AttachableType string `db:"attachable_type" json:"attachable_type"`
	AttachableID   int64  `db:"attachable_id" json:"attachable_id"`
	Disk           string `db:"disk" json:"disk"`
	Path           string `db:"path" json:"path"`
	Size           int64  `db:"size" json:"size"`

	Attachable any `db:"-" rel:"morphTo,as:attachable" json:"-"`
}
