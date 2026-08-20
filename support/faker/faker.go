// Package faker generates realistic fake data for factories, seeders,
// and tests. A Faker is deterministic when seeded:
//
//	fake := faker.New()          // random
//	fake := faker.NewSeeded(42)  // reproducible
//
//	user := &User{
//	    Name:  fake.Name(),
//	    Email: fake.Email(),
//	}
package faker

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Faker generates fake data from an internal random source.
type Faker struct {
	r *rand.Rand
}

// New creates a randomly seeded faker.
func New() *Faker {
	return NewSeeded(time.Now().UnixNano())
}

// NewSeeded creates a deterministic faker for reproducible fixtures.
func NewSeeded(seed int64) *Faker {
	return &Faker{r: rand.New(rand.NewSource(seed))}
}

var firstNames = []string{
	"Ada", "Alan", "Grace", "Linus", "Margaret", "Dennis", "Barbara", "Ken",
	"Donald", "Radia", "Edsger", "Frances", "John", "Joan", "Niklaus", "Adele",
}

var lastNames = []string{
	"Lovelace", "Turing", "Hopper", "Torvalds", "Hamilton", "Ritchie",
	"Liskov", "Thompson", "Knuth", "Perlman", "Dijkstra", "Allen", "Backus",
	"Clarke", "Wirth", "Goldberg",
}

var words = []string{
	"time", "way", "year", "work", "world", "life", "hand", "part", "child",
	"eye", "place", "case", "point", "group", "company", "number", "fact",
	"house", "night", "water", "room", "mother", "area", "money", "story",
	"month", "book", "job", "word", "business", "issue", "side", "kind",
}

var domains = []string{"example.com", "example.org", "example.net", "test.dev"}

func (f *Faker) pick(list []string) string {
	return list[f.r.Intn(len(list))]
}

// FirstName returns a first name.
func (f *Faker) FirstName() string { return f.pick(firstNames) }

// LastName returns a last name.
func (f *Faker) LastName() string { return f.pick(lastNames) }

// Name returns a full name.
func (f *Faker) Name() string { return f.FirstName() + " " + f.LastName() }

// Username returns a lowercase username.
func (f *Faker) Username() string {
	return strings.ToLower(f.FirstName()) + fmt.Sprintf("%d", f.r.Intn(1000))
}

// Email returns a unique-looking email on an example domain.
func (f *Faker) Email() string {
	return fmt.Sprintf("%s.%s%d@%s",
		strings.ToLower(f.FirstName()), strings.ToLower(f.LastName()),
		f.r.Intn(10000), f.pick(domains))
}

// Word returns a single lowercase word.
func (f *Faker) Word() string { return f.pick(words) }

// Words returns n space-separated words.
func (f *Faker) Words(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = f.Word()
	}
	return strings.Join(parts, " ")
}

// Sentence returns a capitalised sentence of about n words.
func (f *Faker) Sentence(n int) string {
	s := f.Words(n)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

// Paragraph returns n sentences.
func (f *Faker) Paragraph(n int) string {
	sentences := make([]string, n)
	for i := range sentences {
		sentences[i] = f.Sentence(4 + f.r.Intn(8))
	}
	return strings.Join(sentences, " ")
}

// Int returns an int in [0, max).
func (f *Faker) Int(max int) int { return f.r.Intn(max) }

// IntBetween returns an int in [min, max] inclusive.
func (f *Faker) IntBetween(min, max int) int {
	if max <= min {
		return min
	}
	return min + f.r.Intn(max-min+1)
}

// Float returns a float64 in [min, max).
func (f *Faker) Float(min, max float64) float64 {
	return min + f.r.Float64()*(max-min)
}

// Bool returns true with the given probability (0..1).
func (f *Faker) Bool(probability float64) bool {
	return f.r.Float64() < probability
}

// Phone returns a fake phone number.
func (f *Faker) Phone() string {
	return fmt.Sprintf("+1-%03d-%03d-%04d", f.r.Intn(900)+100, f.r.Intn(900)+100, f.r.Intn(10000))
}

// URL returns a fake URL.
func (f *Faker) URL() string {
	return fmt.Sprintf("https://%s/%s", f.pick(domains), f.Word())
}

// Slug returns n hyphenated words.
func (f *Faker) Slug(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = f.Word()
	}
	return strings.Join(parts, "-")
}

// Past returns a time up to d in the past.
func (f *Faker) Past(d time.Duration) time.Time {
	return time.Now().Add(-time.Duration(f.r.Int63n(int64(d))))
}

// Future returns a time up to d in the future.
func (f *Faker) Future(d time.Duration) time.Time {
	return time.Now().Add(time.Duration(f.r.Int63n(int64(d))))
}

// Pick returns one of the given values.
func Pick[T any](f *Faker, values ...T) T {
	return values[f.r.Intn(len(values))]
}
