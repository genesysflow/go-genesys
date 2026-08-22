package jobs

import (
	"fmt"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/notifications"
	"github.com/genesysflow/go-genesys/support"
)

// WeeklyDigest mails every author a summary of the week. The scheduler
// pushes it onto the queue rather than running it, so a slow mail server
// cannot hold the scheduler up.
type WeeklyDigest struct {
	Since time.Time `json:"since"`
}

// JobName is the name the payload is registered under.
func (j *WeeklyDigest) JobName() string { return "blog.weekly-digest" }

// Handle builds and sends the digest.
func (j *WeeklyDigest) Handle() error {
	posts, err := database.Query[models.Post]().
		WhereNotNull("published_at").
		Where("published_at", ">=", j.Since).
		OrderByDesc("published_at").
		Get()
	if err != nil {
		return fmt.Errorf("reading the week's posts: %w", err)
	}
	if len(posts) == 0 {
		// Nothing happened this week. An empty digest is worse than none.
		return nil
	}

	authors, err := database.All[models.User]()
	if err != nil {
		return fmt.Errorf("reading authors: %w", err)
	}

	manager := notifications.Default()
	if manager == nil {
		return nil
	}

	titles := support.MapSlice(posts, func(p models.Post) string { return p.Title })

	for i := range authors {
		if err := manager.Send(
			&authors[i],
			&WeeklyDigestNotification{Titles: titles},
		); err != nil {
			return fmt.Errorf("sending the digest to %s: %w", authors[i].Email, err)
		}
	}

	return nil
}

// WeeklyDigestNotification is the digest itself.
type WeeklyDigestNotification struct {
	Titles []string `json:"titles"`
}

// Via sends the digest by mail only: it is a summary, not an alert.
func (n *WeeklyDigestNotification) Via(notifiable notifications.Notifiable) []string {
	return []string{"mail"}
}

// ToMail builds the summary.
func (n *WeeklyDigestNotification) ToMail(notifiable notifications.Notifiable) *mail.Message {
	body := fmt.Sprintf(
		"%s published this week:\n\n- %s",
		support.Str.PluralCount("post", len(n.Titles)),
		support.Str.Of(joinTitles(n.Titles)).Value(),
	)

	return mail.NewMessage().
		Subject(fmt.Sprintf("%d new %s this week", len(n.Titles), support.Str.PluralCount("post", len(n.Titles)))).
		Text(body)
}

// joinTitles renders the titles as a list.
func joinTitles(titles []string) string {
	joined := ""
	for i, title := range titles {
		if i > 0 {
			joined += "\n- "
		}
		joined += title
	}
	return joined
}
