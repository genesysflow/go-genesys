package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/genesysflow/go-genesys/database"
	"github.com/genesysflow/go-genesys/example/app/jobs"
	"github.com/genesysflow/go-genesys/example/app/models"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/genesysflow/go-genesys/testutil/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Publishing marks the post live and queues the follow-up work, rather
// than doing it in the request that asked for it.
func TestPublishingQueuesTheFollowUpWork(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.post(t, author)

	h.signIn(t, h.editor(t))
	h.visit(t, "/posts/"+post.Slug).AssertOK()
	h.form(t, "/posts/"+post.Slug+"/publish", nil).
		AssertRedirect("/posts/" + post.Slug)

	// The post is live straight away.
	fresh, err := database.Find[models.Post](post.ID)
	require.NoError(t, err)
	require.NotNil(t, fresh.PublishedAt)

	// And the notification work is waiting on the queue, not done yet.
	assert.Equal(t, int64(1), h.queued(t))
	h.mailer.AssertNothingSent(t)

	// Running the worker does it.
	require.NoError(t, h.work(t))

	assert.Equal(t, int64(0), h.queued(t))
	h.mailer.AssertSentTo(t, author.Email)
	h.mailer.AssertSent(t, func(m *mail.Message) bool {
		return m.GetSubject() == post.Title+" is live"
	})

	// The database channel recorded it too, so the author sees it in the
	// application as well as in their inbox.
	dbtest.AssertDatabaseCount(t, "notifications", 1)
}

// An author may not publish their own post: that is the editor's call.
func TestAnAuthorMayNotPublishTheirOwnPost(t *testing.T) {
	h := boot(t)
	author := h.signIn(t, h.author(t))
	post := h.post(t, author)

	h.visit(t, "/posts/"+post.Slug).AssertOK()
	h.form(t, "/posts/"+post.Slug+"/publish", nil).AssertForbidden()

	fresh, err := database.Find[models.Post](post.ID)
	require.NoError(t, err)
	assert.Nil(t, fresh.PublishedAt)
	assert.Equal(t, int64(0), h.queued(t))
}

// The job publishes a post that is still a draft when the worker gets
// to it - the scheduler and the API can queue it directly.
func TestThePublishJobPublishesADraft(t *testing.T) {
	h := boot(t)
	author := h.author(t)
	post := h.post(t, author)

	require.NoError(t, h.jobs.Push(&jobs.PublishPost{PostID: post.ID}))
	require.NoError(t, h.work(t))

	fresh, err := database.Find[models.Post](post.ID)
	require.NoError(t, err)
	assert.NotNil(t, fresh.PublishedAt)
	h.mailer.AssertSentTo(t, author.Email)
}

// A post deleted before the worker reached it is not an error to retry:
// there is nothing left to publish.
func TestThePublishJobToleratesADeletedPost(t *testing.T) {
	h := boot(t)

	require.NoError(t, h.jobs.Push(&jobs.PublishPost{PostID: 4242}))
	require.NoError(t, h.work(t))

	failed, err := h.jobs.ListFailed()
	require.NoError(t, err)
	assert.Empty(t, failed, "a missing post should not fail the job")
	h.mailer.AssertNothingSent(t)
}

// The digest mails every author, once, with the week's titles.
func TestTheWeeklyDigest(t *testing.T) {
	h := boot(t)
	first := h.author(t)
	second := h.author(t)
	post := h.published(t, first)

	require.NoError(t, h.jobs.Push(&jobs.WeeklyDigest{Since: time.Now().Add(-7 * 24 * time.Hour)}))
	require.NoError(t, h.work(t))

	h.mailer.AssertSentCount(t, 2)
	h.mailer.AssertSentTo(t, first.Email)
	h.mailer.AssertSentTo(t, second.Email)
	h.mailer.AssertSent(t, func(m *mail.Message) bool {
		return strings.Contains(m.GetHTML(), post.Title) || strings.Contains(m.GetText(), post.Title)
	})
}

// A week with nothing published sends nothing: an empty digest is worse
// than none.
func TestTheWeeklyDigestSkipsAQuietWeek(t *testing.T) {
	h := boot(t)
	h.author(t)

	require.NoError(t, h.jobs.Push(&jobs.WeeklyDigest{Since: time.Now().Add(-7 * 24 * time.Hour)}))
	require.NoError(t, h.work(t))

	h.mailer.AssertNothingSent(t)
}

// A job whose payload nobody registered can never run, so the worker
// fails it immediately rather than retrying forever.
func TestAnUnregisteredJobFailsImmediately(t *testing.T) {
	h := boot(t)

	require.NoError(t, h.jobs.Push(&unregisteredJob{}))
	require.NoError(t, h.work(t))

	failed, err := h.jobs.ListFailed()
	require.NoError(t, err)
	assert.Len(t, failed, 1)
}

type unregisteredJob struct{}

func (j *unregisteredJob) JobName() string { return "nobody.registered.this" }
func (j *unregisteredJob) Handle() error   { return nil }

var _ queue.Job = (*unregisteredJob)(nil)
