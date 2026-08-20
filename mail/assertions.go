package mail

// TestingT is the subset of *testing.T the assertion helpers need.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertSentCount asserts how many messages were captured.
func (m *ArrayMailer) AssertSentCount(t TestingT, expected int) {
	t.Helper()
	if got := len(m.Sent()); got != expected {
		t.Errorf("expected %d sent messages, got %d", expected, got)
	}
}

// AssertNothingSent asserts no messages were captured.
func (m *ArrayMailer) AssertNothingSent(t TestingT) {
	t.Helper()
	if sent := m.Sent(); len(sent) > 0 {
		t.Errorf("expected no sent messages, got %d (first subject: %q)", len(sent), sent[0].GetSubject())
	}
}

// AssertSent asserts at least one captured message matches the
// predicate and returns the matches.
func (m *ArrayMailer) AssertSent(t TestingT, match func(*Message) bool) []*Message {
	t.Helper()
	var matches []*Message
	for _, message := range m.Sent() {
		if match(message) {
			matches = append(matches, message)
		}
	}
	if len(matches) == 0 {
		t.Errorf("expected a sent message matching the predicate; %d messages captured", len(m.Sent()))
	}
	return matches
}

// AssertSentTo asserts a message was sent to the given address.
func (m *ArrayMailer) AssertSentTo(t TestingT, address string) {
	t.Helper()
	for _, message := range m.Sent() {
		for _, recipient := range message.Recipients() {
			if recipient == address {
				return
			}
		}
	}
	t.Errorf("expected a message sent to %q; %d messages captured", address, len(m.Sent()))
}
