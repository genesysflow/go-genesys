package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/genesysflow/go-genesys/database/schema"
	"github.com/genesysflow/go-genesys/http"
	"github.com/genesysflow/go-genesys/query"
)

// ErrTokenNotFound is returned when no token matches.
var ErrTokenNotFound = errors.New("auth: personal access token not found")

// ErrTokenExpired is returned when a token matched but has expired.
var ErrTokenExpired = errors.New("auth: personal access token expired")

// PersonalAccessToken is an API token issued to a user, in the shape
// Laravel Sanctum stores one.
type PersonalAccessToken struct {
	ID            int64
	TokenableType string
	TokenableID   int64
	Name          string
	Abilities     []string
	LastUsedAt    *time.Time
	ExpiresAt     *time.Time
	CreatedAt     time.Time
}

// Can reports whether the token carries an ability. A token holding "*"
// may do anything its user may do.
func (t *PersonalAccessToken) Can(ability string) bool {
	for _, granted := range t.Abilities {
		if granted == "*" || granted == ability {
			return true
		}
	}
	return false
}

// Cannot is the negation of Can.
func (t *PersonalAccessToken) Cannot(ability string) bool {
	return !t.Can(ability)
}

// Expired reports whether the token's expiry has passed.
func (t *PersonalAccessToken) Expired() bool {
	return t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt)
}

// CreatePersonalAccessTokensTable creates the table the repository uses.
// Call it from an application migration:
//
//	func (m *CreateTokensTable) Up(builder *schema.Builder) error {
//	    return auth.CreatePersonalAccessTokensTable(builder)
//	}
func CreatePersonalAccessTokensTable(builder *schema.Builder) error {
	return builder.Create("personal_access_tokens", func(table *schema.Blueprint) {
		table.BigIncrements("id")
		table.String("tokenable_type", 255)
		table.BigInteger("tokenable_id")
		table.String("name", 255)
		// The token column holds a hash, never the token itself.
		table.String("token", 64).Unique()
		table.Text("abilities").Nullable()
		table.Timestamp("last_used_at").Nullable()
		table.Timestamp("expires_at").Nullable()
		table.Timestamps()
		table.Index("tokenable_type", "tokenable_id")
	})
}

// DropPersonalAccessTokensTable drops the table.
func DropPersonalAccessTokensTable(builder *schema.Builder) error {
	return builder.DropIfExists("personal_access_tokens")
}

// TokenRepository issues and validates personal access tokens.
type TokenRepository struct {
	driver   string
	executor query.Executor
	table    string
}

// NewTokenRepository creates a repository over a connection. An empty
// table name defaults to "personal_access_tokens".
func NewTokenRepository(driver string, executor query.Executor, table string) *TokenRepository {
	if table == "" {
		table = "personal_access_tokens"
	}
	return &TokenRepository{driver: driver, executor: executor, table: table}
}

func (r *TokenRepository) query() *query.Builder {
	return query.New(r.driver, r.executor).Table(r.table)
}

// hashToken hashes a token's secret half. Tokens are high-entropy random
// values, so a fast hash is the right choice - unlike a password, there
// is nothing to brute-force.
func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Create issues a token for a user and returns the plaintext, which is
// the only time it exists: the database holds only its hash.
//
// abilities lists what the token may do; nil or empty means "*" (whatever
// its user may do). A nil expiresAt never expires.
func (r *TokenRepository) Create(tokenable Authenticatable, name string, abilities []string, expiresAt *time.Time) (string, *PersonalAccessToken, error) {
	if tokenable == nil {
		return "", nil, fmt.Errorf("auth: cannot issue a token without a user")
	}

	id, err := identifierAsInt64(tokenable.GetAuthIdentifier())
	if err != nil {
		return "", nil, err
	}

	if len(abilities) == 0 {
		abilities = []string{"*"}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	secret := hex.EncodeToString(raw)

	encodedAbilities, err := json.Marshal(abilities)
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	values := map[string]any{
		"tokenable_type": tokenableType(tokenable),
		"tokenable_id":   id,
		"name":           name,
		"token":          hashToken(secret),
		"abilities":      string(encodedAbilities),
		"created_at":     now,
		"updated_at":     now,
	}
	if expiresAt != nil {
		values["expires_at"] = expiresAt.UTC()
	}

	tokenID, err := r.query().InsertGetID(values)
	if err != nil {
		return "", nil, err
	}

	token := &PersonalAccessToken{
		ID:            tokenID,
		TokenableType: tokenableType(tokenable),
		TokenableID:   id,
		Name:          name,
		Abilities:     abilities,
		ExpiresAt:     expiresAt,
		CreatedAt:     now,
	}

	// "<id>|<secret>" so a lookup is one indexed read rather than a scan
	// over every hash, matching Sanctum's format.
	return strconv.FormatInt(tokenID, 10) + "|" + secret, token, nil
}

// Find validates a plaintext token and returns it, recording the use.
// It reports ErrTokenNotFound for anything that does not match and
// ErrTokenExpired for a token past its expiry.
func (r *TokenRepository) Find(plaintext string) (*PersonalAccessToken, error) {
	id, secret, ok := strings.Cut(plaintext, "|")
	if !ok {
		return nil, ErrTokenNotFound
	}

	tokenID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, ErrTokenNotFound
	}

	row, err := r.query().Where("id", tokenID).First()
	if err != nil || row == nil {
		return nil, ErrTokenNotFound
	}

	stored, _ := row["token"].(string)
	if stored == "" {
		if raw, isBytes := row["token"].([]byte); isBytes {
			stored = string(raw)
		}
	}

	// Constant-time so a wrong token cannot be narrowed down by timing.
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hashToken(secret))) != 1 {
		return nil, ErrTokenNotFound
	}

	token, err := scanToken(row)
	if err != nil {
		return nil, err
	}
	if token.Expired() {
		return nil, ErrTokenExpired
	}

	// Recording the use must not fail the request: the token is valid
	// whether or not the bookkeeping write lands.
	now := time.Now().UTC()
	if _, err := r.query().Where("id", token.ID).Update(map[string]any{
		"last_used_at": now,
		"updated_at":   now,
	}); err == nil {
		token.LastUsedAt = &now
	}

	return token, nil
}

// Revoke deletes one token.
func (r *TokenRepository) Revoke(id int64) error {
	_, err := r.query().Where("id", id).Delete()
	return err
}

// RevokeAll deletes every token belonging to a user, returning how many
// were removed - the "sign out everywhere" operation.
func (r *TokenRepository) RevokeAll(tokenable Authenticatable) (int64, error) {
	id, err := identifierAsInt64(tokenable.GetAuthIdentifier())
	if err != nil {
		return 0, err
	}

	return r.query().
		Where("tokenable_type", tokenableType(tokenable)).
		Where("tokenable_id", id).
		Delete()
}

// ListFor returns a user's tokens, newest first. The plaintext is not
// among them: it existed only when the token was created.
func (r *TokenRepository) ListFor(tokenable Authenticatable) ([]PersonalAccessToken, error) {
	id, err := identifierAsInt64(tokenable.GetAuthIdentifier())
	if err != nil {
		return nil, err
	}

	rows, err := r.query().
		Where("tokenable_type", tokenableType(tokenable)).
		Where("tokenable_id", id).
		OrderByDesc("id").
		Get()
	if err != nil {
		return nil, err
	}

	tokens := make([]PersonalAccessToken, 0, len(rows))
	for _, row := range rows {
		token, err := scanToken(row)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, *token)
	}
	return tokens, nil
}

// PruneExpired deletes tokens whose expiry has passed.
func (r *TokenRepository) PruneExpired() (int64, error) {
	return r.query().
		WhereNotNull("expires_at").
		Where("expires_at", "<", time.Now().UTC()).
		Delete()
}

// scanToken maps a row onto a token.
func scanToken(row map[string]any) (*PersonalAccessToken, error) {
	token := &PersonalAccessToken{}

	switch id := row["id"].(type) {
	case int64:
		token.ID = id
	case int:
		token.ID = int64(id)
	}

	token.TokenableType, _ = row["tokenable_type"].(string)
	if id, err := identifierAsInt64(row["tokenable_id"]); err == nil {
		token.TokenableID = id
	}
	token.Name, _ = row["name"].(string)

	if abilities := stringValue(row["abilities"]); abilities != "" {
		if err := json.Unmarshal([]byte(abilities), &token.Abilities); err != nil {
			return nil, fmt.Errorf("auth: token abilities are not valid JSON: %w", err)
		}
	}

	if used, ok := parseBrokerTime(row["last_used_at"]); ok {
		token.LastUsedAt = &used
	}
	if expires, ok := parseBrokerTime(row["expires_at"]); ok {
		token.ExpiresAt = &expires
	}
	if created, ok := parseBrokerTime(row["created_at"]); ok {
		token.CreatedAt = created
	}

	return token, nil
}

// stringValue renders a column value that may come back as bytes.
func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

// identifierAsInt64 narrows a user identifier to the column type.
func identifierAsInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case uint64:
		return int64(typed), nil
	case float64:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("auth: unsupported token owner identifier %T", value)
	}
}

// tokenableType names the owner's type, so tokens for different models
// cannot be confused with one another.
func tokenableType(tokenable Authenticatable) string {
	return fmt.Sprintf("%T", tokenable)
}

// contextTokenKey holds the token the guard authenticated with.
const contextTokenKey = "genesys.auth.token"

// TokenFrom returns the personal access token the request authenticated
// with, or nil when it did not use one.
func TokenFrom(ctx *http.Context) *PersonalAccessToken {
	token, _ := ctx.Get(contextTokenKey).(*PersonalAccessToken)
	return token
}

// PersonalAccessTokenGuard authenticates requests with tokens issued by
// a TokenRepository, Sanctum's guard.
type PersonalAccessTokenGuard struct {
	name       string
	repository *TokenRepository
	provider   UserProvider

	// Header and QueryParam configure where the token is read from.
	// The query parameter is off by default: a token in a URL ends up in
	// logs, referrers, and browser history.
	QueryParam string
}

// NewPersonalAccessTokenGuard creates a guard over a token repository.
func NewPersonalAccessTokenGuard(name string, repository *TokenRepository, provider UserProvider) *PersonalAccessTokenGuard {
	return &PersonalAccessTokenGuard{name: name, repository: repository, provider: provider}
}

func (g *PersonalAccessTokenGuard) cacheKey() string {
	return "genesys.auth.pat." + g.name
}

// TokenFromRequest reads the presented token.
func (g *PersonalAccessTokenGuard) TokenFromRequest(ctx *http.Context) string {
	if token := ctx.Request().BearerToken(); token != "" {
		return token
	}
	if g.QueryParam != "" {
		return ctx.Query(g.QueryParam)
	}
	return ""
}

// User returns the authenticated user, or nil.
func (g *PersonalAccessTokenGuard) User(ctx *http.Context) Authenticatable {
	if cached, ok := ctx.Get(g.cacheKey()).(Authenticatable); ok {
		return cached
	}

	plaintext := g.TokenFromRequest(ctx)
	if plaintext == "" {
		return nil
	}

	token, err := g.repository.Find(plaintext)
	if err != nil {
		return nil
	}

	user, err := g.provider.RetrieveByID(token.TokenableID)
	if err != nil || user == nil {
		return nil
	}

	ctx.Set(g.cacheKey(), user)
	ctx.Set(contextTokenKey, token)
	return user
}

// Check reports whether the request carries a valid token.
func (g *PersonalAccessTokenGuard) Check(ctx *http.Context) bool {
	return g.User(ctx) != nil
}

// ID returns the authenticated user's identifier, or nil.
func (g *PersonalAccessTokenGuard) ID(ctx *http.Context) any {
	if user := g.User(ctx); user != nil {
		return user.GetAuthIdentifier()
	}
	return nil
}

// RequireAbility rejects a request whose token does not carry every one
// of the given abilities. Put it after the auth middleware, which is
// what resolves the token.
func RequireAbility(abilities ...string) http.MiddlewareFunc {
	return func(ctx *http.Context, next func() error) error {
		token := TokenFrom(ctx)
		if token == nil {
			return forbidden()
		}

		for _, ability := range abilities {
			if token.Cannot(ability) {
				return forbidden()
			}
		}
		return next()
	}
}
