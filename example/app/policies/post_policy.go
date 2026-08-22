// Package policies holds the example application's authorization rules.
package policies

import (
	genesysauth "github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/example/app/models"
)

// PostPolicy decides who may act on a post. Each method is an ability:
// Update answers "update", ViewAny answers "view-any".
//
//	auth.RegisterPolicy[models.Post](gate, &policies.PostPolicy{})
type PostPolicy struct{}

// ViewAny lets anyone browse the index.
func (p *PostPolicy) ViewAny(user genesysauth.Authenticatable) bool {
	return true
}

// View hides an unpublished post from everyone but its author and the
// editors.
func (p *PostPolicy) View(user genesysauth.Authenticatable, post *models.Post) bool {
	if post.Published() {
		return true
	}
	return p.owns(user, post)
}

// Create lets any signed-in user write.
func (p *PostPolicy) Create(user genesysauth.Authenticatable) bool {
	return user != nil
}

// Update is the author's, or an editor's.
func (p *PostPolicy) Update(user genesysauth.Authenticatable, post *models.Post) bool {
	return p.owns(user, post)
}

// Delete follows Update.
func (p *PostPolicy) Delete(user genesysauth.Authenticatable, post *models.Post) bool {
	return p.owns(user, post)
}

// Publish is an editor's alone: an author writes, an editor decides what
// goes out.
func (p *PostPolicy) Publish(user genesysauth.Authenticatable, post *models.Post) bool {
	editor, ok := user.(*models.User)
	return ok && editor.IsEditor()
}

// owns reports whether the user wrote the post, or edits everything.
func (p *PostPolicy) owns(user genesysauth.Authenticatable, post *models.Post) bool {
	author, ok := user.(*models.User)
	if !ok || author == nil {
		return false
	}
	return author.IsEditor() || post.AuthorID == author.ID
}
