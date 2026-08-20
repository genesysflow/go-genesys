package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSpec is a parsed 5-field cron expression
// (minute hour day-of-month month day-of-week).
type cronSpec struct {
	minute     map[int]bool
	hour       map[int]bool
	dayOfMonth map[int]bool
	month      map[int]bool
	dayOfWeek  map[int]bool

	domRestricted bool
	dowRestricted bool
}

type cronField struct {
	min, max int
}

var cronFields = []cronField{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // day of month
	{1, 12}, // month
	{0, 6},  // day of week (0 = Sunday)
}

// parseCron parses a 5-field cron expression such as "*/5 8-18 * * 1-5".
func parseCron(expr string) (*cronSpec, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("schedule: cron expression must have 5 fields, got %q", expr)
	}

	sets := make([]map[int]bool, 5)
	for i, field := range fields {
		set, err := parseCronField(field, cronFields[i].min, cronFields[i].max)
		if err != nil {
			return nil, fmt.Errorf("schedule: invalid cron field %q in %q: %w", field, expr, err)
		}
		sets[i] = set
	}

	return &cronSpec{
		minute:        sets[0],
		hour:          sets[1],
		dayOfMonth:    sets[2],
		month:         sets[3],
		dayOfWeek:     sets[4],
		domRestricted: fields[2] != "*",
		dowRestricted: fields[4] != "*",
	}, nil
}

// parseCronField parses one field: "*", "*/n", "a", "a-b", "a-b/n", and
// comma-separated combinations.
func parseCronField(field string, min, max int) (map[int]bool, error) {
	set := make(map[int]bool)
	for _, part := range strings.Split(field, ",") {
		step := 1
		rangePart := part

		if idx := strings.Index(part, "/"); idx >= 0 {
			rangePart = part[:idx]
			parsed, err := strconv.Atoi(part[idx+1:])
			if err != nil || parsed < 1 {
				return nil, fmt.Errorf("invalid step in %q", part)
			}
			step = parsed
		}

		lo, hi := min, max
		switch {
		case rangePart == "*" || rangePart == "":
			// full range
		case strings.Contains(rangePart, "-"):
			bounds := strings.SplitN(rangePart, "-", 2)
			var err1, err2 error
			lo, err1 = strconv.Atoi(bounds[0])
			hi, err2 = strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range %q", rangePart)
			}
		default:
			value, err := strconv.Atoi(rangePart)
			if err != nil {
				return nil, fmt.Errorf("invalid value %q", rangePart)
			}
			lo, hi = value, value
		}

		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("value out of range [%d-%d]", min, max)
		}
		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}
	return set, nil
}

// matches reports whether the spec is due at the given time
// (minute granularity).
func (s *cronSpec) matches(t time.Time) bool {
	if !s.minute[t.Minute()] || !s.hour[t.Hour()] || !s.month[int(t.Month())] {
		return false
	}

	domMatch := s.dayOfMonth[t.Day()]
	dowMatch := s.dayOfWeek[int(t.Weekday())]

	// Standard cron: when both day fields are restricted, either may match.
	if s.domRestricted && s.dowRestricted {
		return domMatch || dowMatch
	}
	return domMatch && dowMatch
}
