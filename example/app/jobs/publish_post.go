// Package jobs holds the example application's queued work.
package jobs

import (
	"errors"
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/notifications"
	"github.com/genesysflow/go-genesys/queue"
)

// PublishPost publishes a post and notifies its author. It runs on a
// worker, so a slow notification never holds up the request that asked
// for it.
type PublishPost struct {
	PostID int64 `json:"post_id"`
}

// JobName is the name the payload is registered under.
func (j *PublishPost) JobName() string { return "posts.publish" }

// Tries bounds the retries: a post that cannot be published after three
// attempts needs a person, not a fourth attempt.
func (j *PublishPost) Tries() int { return 3 }

// Timeout bounds one attempt.
func (j *PublishPost) Timeout() time.Duration { return 30 * time.Second }

// Handle publishes the post.
func (j *PublishPost) Handle() error {
	post, err := database.Find[models.Post](j.PostID)
	if errors.Is(err, database.ErrNotFound) {
		// The post was deleted before the worker got to it. Nothing to
		// publish, and nothing to retry.
		return nil
	}
	if err != nil {
		return fmt.Errorf("publishing post %d: %w", j.PostID, err)
	}

	if post.PublishedAt == nil {
		published := time.Now()
		post.PublishedAt = &published
		if err := database.Update(post); err != nil {
			return fmt.Errorf("publishing post %d: %w", j.PostID, err)
		}
	}

	author, err := database.Find[models.User](post.AuthorID)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	manager := notifications.Default()
	if manager == nil {
		return nil
	}

	// The user is the notifiable: it routes mail to their address and
	// the database channel to their id, so one Send reaches both.
	return manager.Send(
		author,
		&PostPublishedNotification{PostTitle: post.Title, PostSlug: post.Slug},
	)
}

// Register makes the jobs resolvable by workers. A worker that pops a
// payload it cannot name cannot run it, so this is called at boot.
func Register() {
	queue.Register[PublishPost]()
	queue.Register[WeeklyDigest]()
}

// PostPublishedNotification tells an author their post is live. It goes
// out by mail and is recorded in the database so the author sees it in
// the app too.
type PostPublishedNotification struct {
	PostTitle string `json:"post_title"`
	PostSlug  string `json:"post_slug"`
}

// Via lists the channels this notification uses.
func (n *PostPublishedNotification) Via(notifiable notifications.Notifiable) []string {
	return []string{"mail", "database"}
}

// ToMail builds the email.
func (n *PostPublishedNotification) ToMail(notifiable notifications.Notifiable) *mail.Message {
	return mail.NewMessage().
		Subject(n.PostTitle + " is live").
		HTML(fmt.Sprintf(`<p>Your post <strong>%s</strong> is now published.</p>`, n.PostTitle)).
		Text(n.PostTitle + " is now published.")
}

// ToDatabase builds the stored payload.
func (n *PostPublishedNotification) ToDatabase(notifiable notifications.Notifiable) map[string]any {
	return map[string]any{"title": n.PostTitle, "slug": n.PostSlug}
}
