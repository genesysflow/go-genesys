package auth_test

import (
	"testing"

	"github.com/genesysflow/go-genesys/auth"
	"github.com/genesysflow/go-genesys/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Post is the model the policy guards.
type Post struct {
	database.Model
	AuthorID int64  `db:"author_id"`
	Title    string `db:"title"`
}

// PostPolicy is a policy in the shape an application writes one: a
// struct whose methods are the abilities.
type PostPolicy struct{}

func (p *PostPolicy) View(user auth.Authenticatable, post *Post) bool {
	return true
}

func (p *PostPolicy) Update(user auth.Authenticatable, post *Post) bool {
	return post.AuthorID == user.GetAuthIdentifier()
}

func (p *PostPolicy) Delete(user auth.Authenticatable, post *Post) bool {
	return post.AuthorID == user.GetAuthIdentifier()
}

// Create has no instance to act on.
func (p *PostPolicy) Create(user auth.Authenticatable) bool {
	return user != nil
}

// ViewAny is addressed as "view-any" or "viewAny".
func (p *PostPolicy) ViewAny(user auth.Authenticatable) bool {
	return true
}

func policyUser(id int64) *User {
	return &User{Model: database.Model{ID: id}}
}

func TestPolicyResolvesAbilityFromModel(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	alice, bob := policyUser(1), policyUser(2)
	alicePost := &Post{AuthorID: 1}

	assert.True(t, gate.Allows(alice, "update", alicePost))
	assert.False(t, gate.Allows(bob, "update", alicePost))
	assert.True(t, gate.Allows(bob, "view", alicePost))
	assert.True(t, gate.Denies(bob, "delete", alicePost))
}

// A value, not just a pointer, resolves to the same policy.
func TestPolicyAcceptsValueArguments(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	assert.True(t, gate.Allows(policyUser(1), "update", Post{AuthorID: 1}))
}

// Abilities with no instance name their model explicitly.
func TestPolicyAbilityWithoutInstance(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	assert.True(t, auth.AllowsFor[Post](gate, policyUser(1), "create"))
	assert.True(t, auth.AllowsFor[Post](gate, policyUser(1), "view-any"))
	assert.True(t, auth.AllowsFor[Post](gate, policyUser(1), "viewAny"))
	assert.False(t, auth.AllowsFor[Post](gate, nil, "create"))
}

// An ability the policy does not implement is denied, never allowed by
// omission.
func TestPolicyUnknownAbilityIsDenied(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	assert.False(t, gate.Allows(policyUser(1), "publish", &Post{AuthorID: 1}))
}

// A model with no policy is denied rather than falling through to some
// other type's policy.
func TestPolicyUnregisteredModelIsDenied(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	type Comment struct{ ID int64 }
	assert.False(t, gate.Allows(policyUser(1), "update", &Comment{ID: 1}))
}

// An explicitly defined ability wins: it is the more specific statement.
func TestDefinedAbilityBeatsPolicy(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))
	gate.Define("update", func(user auth.Authenticatable, args ...any) bool { return true })

	assert.True(t, gate.Allows(policyUser(2), "update", &Post{AuthorID: 1}))
}

// Before hooks still short-circuit everything, so an administrator
// override keeps working.
func TestBeforeHookBeatsPolicy(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	allow := true
	gate.Before(func(user auth.Authenticatable, ability string, args ...any) *bool { return &allow })

	assert.True(t, gate.Allows(policyUser(2), "update", &Post{AuthorID: 1}))
}

// A policy method with the wrong shape is a typo that would otherwise
// deny silently for the life of the application.
type brokenPolicy struct{}

func (p *brokenPolicy) Update(post *Post) bool { return true }

func TestRegisterPolicyRejectsWrongSignature(t *testing.T) {
	gate := auth.NewGate()

	err := auth.RegisterPolicy[Post](gate, &brokenPolicy{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Update")
}

// A policy with no usable methods is a registration mistake.
type emptyPolicy struct{}

func TestRegisterPolicyRejectsEmptyPolicy(t *testing.T) {
	gate := auth.NewGate()

	err := auth.RegisterPolicy[Post](gate, &emptyPolicy{})
	assert.Error(t, err)
}

// Authorize turns a denial into the 403 the error handler renders.
func TestPolicyAuthorize(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	require.NoError(t, gate.Authorize(policyUser(1), "update", &Post{AuthorID: 1}))

	err := gate.Authorize(policyUser(2), "update", &Post{AuthorID: 1})
	require.Error(t, err)
}

// Abilities are listed for debugging: a policy that resolves nothing is
// otherwise indistinguishable from one that denies everything.
func TestPolicyAbilities(t *testing.T) {
	gate := auth.NewGate()
	require.NoError(t, auth.RegisterPolicy[Post](gate, &PostPolicy{}))

	abilities := auth.PolicyAbilities[Post](gate)
	assert.ElementsMatch(t, []string{"view", "update", "delete", "create", "view-any"}, abilities)
}

// A policy may carry helpers: a method that does not answer with a bool
// is not an ability and must not fail registration.
type policyWithHelper struct{}

func (p *policyWithHelper) Update(user auth.Authenticatable, post *Post) bool {
	return true
}

func (p *policyWithHelper) Describe() string { return "helper" }

func TestRegisterPolicyIgnoresNonAbilityMethods(t *testing.T) {
	gate := auth.NewGate()

	require.NoError(t, auth.RegisterPolicy[Post](gate, &policyWithHelper{}))
	assert.Equal(t, []string{"update"}, auth.PolicyAbilities[Post](gate))
	assert.True(t, gate.Allows(policyUser(1), "update", &Post{}))
}
