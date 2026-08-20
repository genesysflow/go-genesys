package mail

// Renderer renders a named view to a string; view.Manager and the
// locale-pinned translator views both satisfy compatible shapes.
type Renderer interface {
	RenderString(name string, data map[string]any) (string, error)
}

// ViewHTML renders a view as the message's HTML body - templated
// emails through the framework's view layer:
//
//	message := mail.NewMessage().
//	    To(user.Email).
//	    Subject("Welcome!").
//	    ViewHTML(views, "emails.welcome", map[string]any{"name": user.Name})
//
// A render error is deferred and surfaced by Send/Bytes.
func (m *Message) ViewHTML(r Renderer, name string, data map[string]any) *Message {
	html, err := r.RenderString(name, data)
	if err != nil {
		m.viewErr = err
		return m
	}
	m.htmlBody = html
	return m
}

// ViewText renders a view as the message's plain-text body.
func (m *Message) ViewText(r Renderer, name string, data map[string]any) *Message {
	text, err := r.RenderString(name, data)
	if err != nil {
		m.viewErr = err
		return m
	}
	m.textBody = text
	return m
}
