package auth

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/genesysflow/go-genesys/support"
)

// policyMethod is one resolved ability on a policy.
type policyMethod struct {
	fn         reflect.Value
	wantsModel bool
}

// policy holds the abilities resolved from a policy value.
type policy struct {
	abilities map[string]policyMethod
}

// RegisterPolicy binds a policy to a model type, Laravel's
// `Gate::policy(Post::class, PostPolicy::class)`:
//
//	type PostPolicy struct{}
//
//	func (p *PostPolicy) Update(user auth.Authenticatable, post *Post) bool {
//	    return post.AuthorID == user.GetAuthIdentifier()
//	}
//
//	auth.RegisterPolicy[Post](gate, &PostPolicy{})
//
// The gate then answers `gate.Allows(user, "update", post)` by calling
// the matching method. Method names map to abilities by kebab-casing:
// ViewAny answers "view-any" and "viewAny".
//
// Registration fails when a method looks like an ability but has the
// wrong signature, and when nothing usable is found: a policy that
// resolves nothing denies everything, which is indistinguishable from a
// working policy that says no.
func RegisterPolicy[T any](gate *Gate, value any) error {
	if value == nil {
		return fmt.Errorf("auth: policy for %s is nil", modelTypeName[T]())
	}

	resolved, err := resolvePolicy(value)
	if err != nil {
		return err
	}

	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.policies == nil {
		gate.policies = make(map[reflect.Type]*policy)
	}
	gate.policies[modelType[T]()] = resolved

	return nil
}

// resolvePolicy reflects over a policy value's exported methods.
func resolvePolicy(value any) (*policy, error) {
	valueOf := reflect.ValueOf(value)
	typeOf := valueOf.Type()

	resolved := &policy{abilities: make(map[string]policyMethod)}

	for i := 0; i < typeOf.NumMethod(); i++ {
		method := typeOf.Method(i)
		signature := method.Func.Type()

		// A method that does not answer with a bool is a helper, not an
		// ability, and is left alone. One that does but has the wrong
		// arguments is a typo that would otherwise deny silently for the
		// life of the application.
		if signature.NumOut() != 1 || signature.Out(0).Kind() != reflect.Bool {
			continue
		}

		wantsModel, ok := policySignature(signature)
		if !ok {
			return nil, fmt.Errorf(
				"auth: policy method %s.%s must be func(auth.Authenticatable[, model]) bool",
				typeOf, method.Name,
			)
		}

		resolved.abilities[abilityName(method.Name)] = policyMethod{
			fn:         valueOf.Method(i),
			wantsModel: wantsModel,
		}
	}

	if len(resolved.abilities) == 0 {
		return nil, fmt.Errorf("auth: policy %s defines no abilities", typeOf)
	}
	return resolved, nil
}

// authenticatableType is the interface a policy method's first argument
// must accept.
var authenticatableType = reflect.TypeOf((*Authenticatable)(nil)).Elem()

// policySignature reports whether a method is shaped like an ability,
// and whether it takes the model.
func policySignature(signature reflect.Type) (wantsModel bool, ok bool) {
	// The receiver counts as the first input.
	if signature.NumIn() < 2 || signature.NumIn() > 3 {
		return false, false
	}
	if signature.In(1) != authenticatableType {
		return false, false
	}
	return signature.NumIn() == 3, true
}

// abilityName maps a method name to its ability: ViewAny -> "view-any".
func abilityName(method string) string {
	return kebab(method)
}

// kebab converts a PascalCase method name to kebab-case.
func kebab(name string) string {
	return strings.ReplaceAll(support.ToSnakeCase(name), "_", "-")
}

// normalizeAbility renders an ability in the canonical kebab-case form,
// so "viewAny", "view_any", and "view-any" all address ViewAny.
func normalizeAbility(ability string) string {
	if strings.ContainsAny(ability, "-_") {
		return strings.ToLower(strings.ReplaceAll(ability, "_", "-"))
	}
	return kebab(ability)
}

// policyFor returns the policy registered for a model value's type.
func (g *Gate) policyFor(model any) *policy {
	if model == nil {
		return nil
	}
	return g.policyForType(reflect.Indirect(reflect.ValueOf(model)).Type())
}

// policyForType returns the policy registered for a model type.
func (g *Gate) policyForType(t reflect.Type) *policy {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.policies[t]
}

// callPolicy runs an ability against a resolved policy.
func callPolicy(resolved *policy, user Authenticatable, ability string, model any) (bool, bool) {
	method, found := resolved.abilities[normalizeAbility(ability)]
	if !found {
		return false, false
	}

	args := []reflect.Value{userArgument(user)}
	if method.wantsModel {
		if model == nil {
			return false, true
		}
		args = append(args, modelArgument(method.fn.Type().In(1), model))
	}

	return method.fn.Call(args)[0].Bool(), true
}

// userArgument renders the user as the interface value the method takes,
// keeping a nil user callable.
func userArgument(user Authenticatable) reflect.Value {
	if user == nil {
		return reflect.Zero(authenticatableType)
	}
	return reflect.ValueOf(user)
}

// modelArgument adapts the model to the parameter type, so a policy
// taking *Post accepts a Post and vice versa.
func modelArgument(want reflect.Type, model any) reflect.Value {
	value := reflect.ValueOf(model)
	if value.Type() == want {
		return value
	}

	if want.Kind() == reflect.Ptr && value.Kind() != reflect.Ptr && value.Type() == want.Elem() {
		pointer := reflect.New(value.Type())
		pointer.Elem().Set(value)
		return pointer
	}
	if want.Kind() != reflect.Ptr && value.Kind() == reflect.Ptr && value.Type().Elem() == want {
		return value.Elem()
	}

	return reflect.Zero(want)
}

// AllowsFor answers an ability that has no instance to act on, such as
// "create" or "view-any", by naming the model type:
//
//	auth.AllowsFor[Post](gate, user, "create")
func AllowsFor[T any](gate *Gate, user Authenticatable, ability string) bool {
	if allowed, decided := gate.runBefore(user, ability); decided {
		return allowed
	}

	resolved := gate.policyForType(modelType[T]())
	if resolved == nil {
		return false
	}

	allowed, _ := callPolicy(resolved, user, ability, nil)
	return allowed
}

// AuthorizeFor returns a 403 error unless the user may perform an
// instance-less ability on the model type.
func AuthorizeFor[T any](gate *Gate, user Authenticatable, ability string) error {
	if AllowsFor[T](gate, user, ability) {
		return nil
	}
	return forbidden()
}

// PolicyAbilities lists the abilities resolved for a model's policy, so
// a policy that resolves nothing can be told from one that denies.
func PolicyAbilities[T any](gate *Gate) []string {
	resolved := gate.policyForType(modelType[T]())
	if resolved == nil {
		return nil
	}

	abilities := make([]string, 0, len(resolved.abilities))
	for ability := range resolved.abilities {
		abilities = append(abilities, ability)
	}
	sort.Strings(abilities)
	return abilities
}

// modelType returns the reflect.Type policies are keyed by.
func modelType[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}

// modelTypeName names T for error messages.
func modelTypeName[T any]() string {
	return modelType[T]().String()
}
