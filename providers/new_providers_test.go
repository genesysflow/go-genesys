package providers

import (
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/crypt"
	"github.com/genesysflow/go-genesys/database/seed"
	facadeauth "github.com/genesysflow/go-genesys/facades/auth"
	facadecrypt "github.com/genesysflow/go-genesys/facades/crypt"
	facademail "github.com/genesysflow/go-genesys/facades/mail"
	facadeview "github.com/genesysflow/go-genesys/facades/view"
	"github.com/genesysflow/go-genesys/lang"
	"github.com/genesysflow/go-genesys/mail"
	"github.com/genesysflow/go-genesys/queue"
	"github.com/genesysflow/go-genesys/schedule"
	"github.com/genesysflow/go-genesys/testutil"
	"github.com/genesysflow/go-genesys/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewServiceProvider(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &ViewServiceProvider{Config: &view.Config{Path: t.TempDir()}}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))

	assert.NotNil(t, app.GetInstance("view"))
	assert.NotNil(t, facadeview.GetInstance())
	assert.Contains(t, provider.Provides(), "view")
	facadeview.SetInstance(nil)
}

func TestMailServiceProviderDefaultsToLogDriver(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &MailServiceProvider{}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))

	mailer := app.GetInstance("mailer")
	require.NotNil(t, mailer)
	assert.IsType(t, &mail.LogMailer{}, mailer)
	facademail.SetInstance(nil)
}

func TestMailServiceProviderArrayDriver(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &MailServiceProvider{Config: &mail.Config{Driver: "array", FromAddress: "a@b.c"}}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))
	assert.IsType(t, &mail.ArrayMailer{}, app.GetInstance("mailer"))
	facademail.SetInstance(nil)
}

func TestLangServiceProvider(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &LangServiceProvider{Config: &lang.Config{Path: t.TempDir(), Locale: "de"}}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))

	translator, ok := app.GetInstance("translator").(*lang.Translator)
	require.True(t, ok)
	assert.Equal(t, "de", translator.Locale())
}

func TestScheduleServiceProvider(t *testing.T) {
	app := testutil.NewMockApplication()
	defined := false
	provider := &ScheduleServiceProvider{
		Define: func(s *schedule.Schedule) {
			defined = true
			s.Call(func() error { return nil }).Daily()
		},
	}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))
	assert.True(t, defined)

	scheduler, ok := app.GetInstance("schedule").(*schedule.Schedule)
	require.True(t, ok)
	assert.Len(t, scheduler.Events(), 1)
}

func TestSeedServiceProvider(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &SeedServiceProvider{
		Define: func(r *seed.Runner) {
			r.AddFunc("demo", func() error { return nil })
		},
	}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))

	runner, ok := app.GetInstance("seeder").(*seed.Runner)
	require.True(t, ok)
	assert.Equal(t, []string{"demo"}, runner.Names())
}

func TestEncryptionServiceProvider(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &EncryptionServiceProvider{Key: crypt.GenerateKey()}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))

	encrypter, ok := app.GetInstance("encrypter").(*crypt.Encrypter)
	require.True(t, ok)
	payload, err := encrypter.EncryptString("x")
	require.NoError(t, err)
	plain, err := facadecrypt.DecryptString(payload)
	require.NoError(t, err)
	assert.Equal(t, "x", plain)
	facadecrypt.SetInstance(nil)
}

func TestEncryptionServiceProviderRejectsBadKey(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &EncryptionServiceProvider{Key: "base64:not-valid!!!"}
	require.NoError(t, provider.Register(app))
	assert.Error(t, provider.Boot(app))
}

type providerTestUser struct{ ID int64 }

func (u *providerTestUser) GetAuthIdentifier() any  { return u.ID }
func (u *providerTestUser) GetAuthPassword() string { return "" }

type stubUserProvider struct{}

func (p *stubUserProvider) RetrieveByID(id any) (auth.Authenticatable, error) {
	return &providerTestUser{ID: 1}, nil
}
func (p *stubUserProvider) RetrieveByCredentials(c map[string]any) (auth.Authenticatable, error) {
	return &providerTestUser{ID: 1}, nil
}
func (p *stubUserProvider) RetrieveByToken(token string) (auth.Authenticatable, error) {
	return &providerTestUser{ID: 1}, nil
}

func TestAuthServiceProviderRegistersGuards(t *testing.T) {
	app := testutil.NewMockApplication()
	provider := &AuthServiceProvider{UserProvider: &stubUserProvider{}}

	require.NoError(t, provider.Register(app))

	manager, ok := app.GetInstance("auth").(*auth.Manager)
	require.True(t, ok)
	_, err := manager.Guard("web")
	assert.NoError(t, err, "session guard registered")
	_, err = manager.Guard("api")
	assert.NoError(t, err, "token guard registered because provider supports tokens")

	gate, ok := app.GetInstance("gate").(*auth.Gate)
	require.True(t, ok)
	assert.NotNil(t, gate)

	facadeauth.SetInstance(nil)
}

func TestQueueServiceProviderBootWiresConfiguredConnections(t *testing.T) {
	cfg := testutil.NewMockConfig(map[string]any{
		"queue.default": "jobs",
		"queue.connections": map[string]any{
			"jobs":   map[string]any{"driver": "memory"},
			"inline": map[string]any{"driver": "sync"},
		},
	})
	app := testutil.NewMockApplicationWithConfig(cfg)
	provider := &QueueServiceProvider{}

	require.NoError(t, provider.Register(app))
	require.NoError(t, provider.Boot(app))

	manager, ok := app.GetInstance("queue").(*queue.Manager)
	require.True(t, ok)
	assert.Equal(t, "jobs", manager.DefaultConnection())

	conn, err := manager.Connection()
	require.NoError(t, err)
	assert.IsType(t, &queue.MemoryQueue{}, conn)

	inline, err := manager.Connection("inline")
	require.NoError(t, err)
	assert.IsType(t, &queue.SyncQueue{}, inline)
}
