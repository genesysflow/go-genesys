package log

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/genesysflow/go-genesys/contracts"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ----------------------------------------------------------------

// records decodes every JSON line written to buf.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "line: %s", line)
		out = append(out, rec)
	}
	return out
}

// lastRecord decodes the most recent JSON line written to buf.
func lastRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	all := records(t, buf)
	require.NotEmpty(t, all, "expected at least one log record")
	return all[len(all)-1]
}

// stubExit swaps the package exit hook for a recorder and restores it
// when the test finishes. The returned pointer collects the exit codes
// Fatal asked for, so a test can assert the process would have died.
func stubExit(t *testing.T) *[]int {
	t.Helper()
	original := osExit
	codes := &[]int{}
	osExit = func(code int) { *codes = append(*codes, code) }
	t.Cleanup(func() { osExit = original })
	return codes
}

// foreignLogger is a contracts.Logger from outside this package: it has
// no access to the unexported non-exiting fatal/panic writers, so a
// stack has to fall back to its exported Fatal/Panic.
type foreignLogger struct {
	calls []string
	level contracts.LogLevel
}

func (f *foreignLogger) Debug(msg string, fields ...any)              { f.calls = append(f.calls, "debug:"+msg) }
func (f *foreignLogger) Info(msg string, fields ...any)               { f.calls = append(f.calls, "info:"+msg) }
func (f *foreignLogger) Warn(msg string, fields ...any)               { f.calls = append(f.calls, "warn:"+msg) }
func (f *foreignLogger) Error(msg string, fields ...any)              { f.calls = append(f.calls, "error:"+msg) }
func (f *foreignLogger) Fatal(msg string, fields ...any)              { f.calls = append(f.calls, "fatal:"+msg) }
func (f *foreignLogger) Panic(msg string, fields ...any)              { f.calls = append(f.calls, "panic:"+msg) }
func (f *foreignLogger) WithField(string, any) contracts.Logger       { return f }
func (f *foreignLogger) WithFields(map[string]any) contracts.Logger   { return f }
func (f *foreignLogger) WithContext(context.Context) contracts.Logger { return f }
func (f *foreignLogger) WithError(error) contracts.Logger             { return f }
func (f *foreignLogger) Level() contracts.LogLevel                    { return f.level }
func (f *foreignLogger) SetLevel(level contracts.LogLevel)            { f.level = level }

// bufferedManager returns a manager whose "default" channel writes JSON to buf.
func bufferedManager(t *testing.T) (*LogManager, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	m := NewManager()
	m.AddChannel("default", NewJSON(buf))
	return m, buf
}

// --- LogManager wiring ------------------------------------------------------

func TestManager_NewManagerRegistersDefaultChannel(t *testing.T) {
	m := NewManager()

	require.NotNil(t, m)
	assert.NotNil(t, m.Channel("default"))
	assert.Same(t, m.Channel("default"), m.defaultLogger())
	assert.Equal(t, contracts.LogLevelInfo, m.Level())
}

func TestManager_AddChannelAndChannel(t *testing.T) {
	m := NewManager()
	audit := NewJSON(&bytes.Buffer{})

	m.AddChannel("audit", audit)

	assert.Same(t, audit, m.Channel("audit"))
	// The default channel is untouched by AddChannel.
	assert.NotSame(t, audit, m.Channel("default"))
}

func TestManager_AddChannelReplacesExistingName(t *testing.T) {
	m := NewManager()
	first := NewJSON(&bytes.Buffer{})
	second := NewJSON(&bytes.Buffer{})

	m.AddChannel("audit", first)
	m.AddChannel("audit", second)

	assert.Same(t, second, m.Channel("audit"))
}

func TestManager_ChannelUnknownFallsBackToDefault(t *testing.T) {
	m, buf := bufferedManager(t)

	unknown := m.Channel("does-not-exist")

	require.NotNil(t, unknown)
	assert.Same(t, m.Channel("default"), unknown, "an unknown channel resolves to the default one")

	unknown.Info("fallback")
	assert.Equal(t, "fallback", lastRecord(t, buf)["message"])
}

func TestManager_SetDefaultSwitchesTarget(t *testing.T) {
	m, defaultBuf := bufferedManager(t)
	auditBuf := &bytes.Buffer{}
	m.AddChannel("audit", NewJSON(auditBuf))

	m.SetDefault("audit")
	m.Info("after switch")

	assert.Empty(t, defaultBuf.String(), "the old default no longer receives records")
	assert.Equal(t, "after switch", lastRecord(t, auditBuf)["message"])
	assert.Same(t, m.Channel("audit"), m.defaultLogger())
}

func TestManager_SetDefaultIgnoresUnknownChannel(t *testing.T) {
	m, buf := bufferedManager(t)
	before := m.defaultLogger()

	m.SetDefault("nope")

	assert.Same(t, before, m.defaultLogger(), "an unknown name is silently ignored")
	m.Info("still default")
	assert.Equal(t, "still default", lastRecord(t, buf)["message"])
}

func TestManager_Stack(t *testing.T) {
	m, defaultBuf := bufferedManager(t)
	firstBuf, secondBuf := &bytes.Buffer{}, &bytes.Buffer{}
	m.AddChannel("first", NewJSON(firstBuf))
	m.AddChannel("second", NewJSON(secondBuf))

	m.Stack("first", "second").Info("stacked")

	// Every channel of the stack receives the record; channels outside
	// the stack - here the default one - do not.
	assert.Equal(t, "stacked", lastRecord(t, firstBuf)["message"])
	assert.Equal(t, "stacked", lastRecord(t, secondBuf)["message"])
	assert.Empty(t, defaultBuf.String())
}

func TestManager_StackFansOutEveryLevel(t *testing.T) {
	m, _ := bufferedManager(t)
	firstBuf, secondBuf := &bytes.Buffer{}, &bytes.Buffer{}
	m.AddChannel("first", NewJSON(firstBuf))
	m.AddChannel("second", NewJSON(secondBuf))

	stack := m.Stack("first", "second")
	stack.SetLevel(contracts.LogLevelDebug)
	require.Equal(t, contracts.LogLevelDebug, stack.Level(), "SetLevel reaches every member")

	stack.Debug("d")
	stack.Info("i")
	stack.Warn("w")
	stack.Error("e")

	for _, buf := range []*bytes.Buffer{firstBuf, secondBuf} {
		got := records(t, buf)
		require.Len(t, got, 4)
		assert.Equal(t, []any{"debug", "info", "warn", "error"},
			[]any{got[0]["level"], got[1]["level"], got[2]["level"], got[3]["level"]})
	}
}

func TestManager_StackLevelIsTheMostVerboseMember(t *testing.T) {
	m, _ := bufferedManager(t)
	quiet, loud := NewJSON(&bytes.Buffer{}), NewJSON(&bytes.Buffer{})
	quiet.SetLevel(contracts.LogLevelError)
	loud.SetLevel(contracts.LogLevelDebug)
	m.AddChannel("quiet", quiet)
	m.AddChannel("loud", loud)

	assert.Equal(t, contracts.LogLevelDebug, m.Stack("quiet", "loud").Level())
}

func TestManager_StackWithMethodsDeriveEveryChannel(t *testing.T) {
	m, _ := bufferedManager(t)
	firstBuf, secondBuf := &bytes.Buffer{}, &bytes.Buffer{}
	m.AddChannel("first", NewJSON(firstBuf))
	m.AddChannel("second", NewJSON(secondBuf))
	ctx := context.WithValue(context.Background(), RequestIDKey, "req-7")

	m.Stack("first", "second").
		WithField("app", "genesys").
		WithFields(map[string]any{"shard": 2}).
		WithContext(ctx).
		WithError(assert.AnError).
		Error("derived")

	for _, buf := range []*bytes.Buffer{firstBuf, secondBuf} {
		rec := lastRecord(t, buf)
		assert.Equal(t, "derived", rec["message"])
		assert.Equal(t, "genesys", rec["app"])
		assert.Equal(t, float64(2), rec["shard"])
		assert.Equal(t, "req-7", rec["request_id"])
		assert.Equal(t, assert.AnError.Error(), rec["error"])
	}
}

func TestManager_StackDeduplicatesChannels(t *testing.T) {
	m, defaultBuf := bufferedManager(t)
	firstBuf := &bytes.Buffer{}
	m.AddChannel("first", NewJSON(firstBuf))

	// "ghost" is unknown and resolves to the default channel, exactly as
	// Channel does; repeated names are collapsed so nothing is logged twice.
	m.Stack("first", "first", "ghost", "default").Info("once")

	assert.Len(t, records(t, firstBuf), 1)
	assert.Len(t, records(t, defaultBuf), 1)
}

func TestManager_StackWithASingleChannelReturnsThatChannel(t *testing.T) {
	m, _ := bufferedManager(t)
	m.AddChannel("first", NewJSON(&bytes.Buffer{}))

	assert.Same(t, m.Channel("first"), m.Stack("first"))
	assert.Same(t, m.Channel("first"), m.Stack("first", "first"))
}

func TestManager_StackFatalWritesEverywhereThenExitsOnce(t *testing.T) {
	m, _ := bufferedManager(t)
	firstBuf, secondBuf := &bytes.Buffer{}, &bytes.Buffer{}
	m.AddChannel("first", NewJSON(firstBuf))
	m.AddChannel("second", NewJSON(secondBuf))
	codes := stubExit(t)

	m.Stack("first", "second").Fatal("stacked fatal", "reason", "disk full")

	for _, buf := range []*bytes.Buffer{firstBuf, secondBuf} {
		rec := lastRecord(t, buf)
		assert.Equal(t, "fatal", rec["level"])
		assert.Equal(t, "stacked fatal", rec["message"])
		assert.Equal(t, "disk full", rec["reason"])
	}
	assert.Equal(t, []int{1}, *codes, "the stack exits once, after every channel wrote")
}

func TestManager_StackPanicWritesEverywhereThenPanics(t *testing.T) {
	m, _ := bufferedManager(t)
	firstBuf, secondBuf := &bytes.Buffer{}, &bytes.Buffer{}
	m.AddChannel("first", NewJSON(firstBuf))
	m.AddChannel("second", NewJSON(secondBuf))

	assert.PanicsWithValue(t, "stacked panic", func() {
		m.Stack("first", "second").Panic("stacked panic")
	})

	for _, buf := range []*bytes.Buffer{firstBuf, secondBuf} {
		rec := lastRecord(t, buf)
		assert.Equal(t, "panic", rec["level"])
		assert.Equal(t, "stacked panic", rec["message"])
	}
}

func TestManager_StackWithoutChannelsUsesDefault(t *testing.T) {
	m, buf := bufferedManager(t)

	stack := m.Stack()

	assert.Same(t, m.defaultLogger(), stack)
	stack.Info("default stack")
	assert.Equal(t, "default stack", lastRecord(t, buf)["message"])
}

func TestManager_StackWithUnknownChannelUsesDefault(t *testing.T) {
	m, buf := bufferedManager(t)

	m.Stack("ghost").Info("ghost stack")

	assert.Equal(t, "ghost stack", lastRecord(t, buf)["message"])
}

// --- LogManager level methods ----------------------------------------------

func TestManager_LevelMethodsDelegateToDefaultChannel(t *testing.T) {
	m, buf := bufferedManager(t)
	m.SetLevel(contracts.LogLevelDebug)

	m.Debug("a debug line", "k", "v")
	m.Info("an info line")
	m.Warn("a warn line")
	m.Error("an error line")

	got := records(t, buf)
	require.Len(t, got, 4)

	assert.Equal(t, "debug", got[0]["level"])
	assert.Equal(t, "a debug line", got[0]["message"])
	assert.Equal(t, "v", got[0]["k"])
	assert.Equal(t, "info", got[1]["level"])
	assert.Equal(t, "an info line", got[1]["message"])
	assert.Equal(t, "warn", got[2]["level"])
	assert.Equal(t, "a warn line", got[2]["message"])
	assert.Equal(t, "error", got[3]["level"])
	assert.Equal(t, "an error line", got[3]["message"])
}

func TestManager_DebugIsDroppedAtInfoLevel(t *testing.T) {
	m, buf := bufferedManager(t)

	require.Equal(t, contracts.LogLevelInfo, m.Level())
	m.Debug("invisible")

	assert.Empty(t, buf.String())
}

func TestManager_SetLevelAndLevel(t *testing.T) {
	m, buf := bufferedManager(t)
	other := NewJSON(&bytes.Buffer{})
	m.AddChannel("other", other)

	m.SetLevel(contracts.LogLevelError)

	assert.Equal(t, contracts.LogLevelError, m.Level())
	assert.Equal(t, contracts.LogLevelError, m.Channel("default").Level())
	// SetLevel only reaches the default channel; other channels keep theirs.
	assert.Equal(t, contracts.LogLevelInfo, other.Level())

	m.Warn("dropped")
	assert.Empty(t, buf.String())
	m.Error("kept")
	assert.Equal(t, "kept", lastRecord(t, buf)["message"])
}

func TestManager_FatalWritesRecordThenExits(t *testing.T) {
	m, buf := bufferedManager(t)
	codes := stubExit(t)

	m.Fatal("fatal line", "reason", "disk full")

	// The record reaches the default channel before the process dies.
	rec := lastRecord(t, buf)
	assert.Equal(t, "fatal", rec["level"])
	assert.Equal(t, "fatal line", rec["message"])
	assert.Equal(t, "disk full", rec["reason"])
	assert.Equal(t, []int{1}, *codes, "delegation to the default channel still exits")
}

func TestManager_PanicWritesRecordThenPanics(t *testing.T) {
	m, buf := bufferedManager(t)

	assert.PanicsWithValue(t, "panic line", func() {
		m.Panic("panic line")
	})

	// The record was written before the stack unwound.
	rec := lastRecord(t, buf)
	assert.Equal(t, "panic", rec["level"])
	assert.Equal(t, "panic line", rec["message"])
}

// --- LogManager With* -------------------------------------------------------

func TestManager_WithFieldWritesToDefaultChannel(t *testing.T) {
	m, buf := bufferedManager(t)

	m.WithField("user_id", 7).Info("with field")

	rec := lastRecord(t, buf)
	assert.Equal(t, float64(7), rec["user_id"])
	assert.Equal(t, "with field", rec["message"])

	// The default channel itself is not mutated.
	m.Info("plain")
	assert.NotContains(t, lastRecord(t, buf), "user_id")
}

func TestManager_WithFields(t *testing.T) {
	m, buf := bufferedManager(t)

	m.WithFields(map[string]any{"service": "api", "shard": 3}).Warn("with fields")

	rec := lastRecord(t, buf)
	assert.Equal(t, "api", rec["service"])
	assert.Equal(t, float64(3), rec["shard"])
	assert.Equal(t, "warn", rec["level"])
}

func TestManager_WithError(t *testing.T) {
	m, buf := bufferedManager(t)

	m.WithError(assert.AnError).Error("with error")

	rec := lastRecord(t, buf)
	assert.Equal(t, assert.AnError.Error(), rec["error"])
	assert.Equal(t, "with error", rec["message"])
}

func TestManager_WithContext(t *testing.T) {
	m, buf := bufferedManager(t)
	ctx := context.WithValue(context.Background(), RequestIDKey, "req-42")

	m.WithContext(ctx).Info("with context")

	assert.Equal(t, "req-42", lastRecord(t, buf)["request_id"])
}

func TestManager_WithMethodsFollowTheCurrentDefault(t *testing.T) {
	m, defaultBuf := bufferedManager(t)
	auditBuf := &bytes.Buffer{}
	m.AddChannel("audit", NewJSON(auditBuf))
	m.SetDefault("audit")

	m.WithField("scope", "audit").Info("routed")

	assert.Empty(t, defaultBuf.String())
	assert.Equal(t, "audit", lastRecord(t, auditBuf)["scope"])
}

// --- Logger: Fatal / Panic --------------------------------------------------

func TestLogger_FatalWritesRecordThenExits(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)
	codes := stubExit(t)

	logger.Fatal("boom")

	rec := lastRecord(t, buf)
	assert.Equal(t, "fatal", rec["level"])
	assert.Equal(t, "boom", rec["message"])
	assert.Equal(t, []int{1}, *codes, "the record is written, then the process exits with 1")
}

func TestLogger_PanicWritesRecordThenPanics(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	assert.PanicsWithValue(t, "kaboom", func() {
		logger.Panic("kaboom", "code", 500)
	})

	rec := lastRecord(t, buf)
	assert.Equal(t, "panic", rec["level"])
	assert.Equal(t, "kaboom", rec["message"])
	assert.Equal(t, float64(500), rec["code"])
}

func TestLogger_FatalAndPanicRecordsRespectTheLevelFilter(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)
	logger.SetLevel(contracts.LogLevelPanic)
	codes := stubExit(t)

	logger.Fatal("fatal is below panic")
	assert.Empty(t, buf.String(), "the fatal record is filtered out when the level is panic")
	assert.Equal(t, []int{1}, *codes, "...but the process still exits, as zerolog's own Fatal does")

	assert.PanicsWithValue(t, "panic passes", func() {
		logger.Panic("panic passes")
	})
	assert.Equal(t, "panic passes", lastRecord(t, buf)["message"])
}

// --- Logger: With* isolation and levels ------------------------------------

func TestLogger_WithFieldKeepsParentFields(t *testing.T) {
	buf := &bytes.Buffer{}
	parent := NewJSON(buf).WithField("app", "genesys")

	child := parent.WithField("request", "abc")
	child.Info("child")

	rec := lastRecord(t, buf)
	assert.Equal(t, "genesys", rec["app"], "inherited from the parent")
	assert.Equal(t, "abc", rec["request"])

	parent.Info("parent")
	rec = lastRecord(t, buf)
	assert.Equal(t, "genesys", rec["app"])
	assert.NotContains(t, rec, "request", "the parent is not mutated by WithField")
}

func TestLogger_WithFieldsOverridesInheritedKeys(t *testing.T) {
	buf := &bytes.Buffer{}
	parent := NewJSON(buf).WithField("stage", "first")

	parent.WithFields(map[string]any{"stage": "second"}).Info("override")
	assert.Equal(t, "second", lastRecord(t, buf)["stage"])

	parent.Info("parent")
	assert.Equal(t, "first", lastRecord(t, buf)["stage"])
}

func TestLogger_WithContextDoesNotMutateParent(t *testing.T) {
	buf := &bytes.Buffer{}
	parent := NewJSON(buf)

	ctx := context.WithValue(context.Background(), RequestIDKey, "req-99")
	parent.WithContext(ctx).Info("child")
	assert.Equal(t, "req-99", lastRecord(t, buf)["request_id"])

	parent.Info("parent")
	assert.NotContains(t, lastRecord(t, buf), "request_id")
}

func TestLogger_WithContextCarriesFieldsAndLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	base := NewJSON(buf)
	base.SetLevel(contracts.LogLevelWarn)

	ctx := context.WithValue(context.Background(), RequestIDKey, "req-1")
	child := base.WithField("app", "genesys").WithContext(ctx)

	assert.Equal(t, contracts.LogLevelWarn, child.Level(), "the level is inherited")
	child.Info("dropped")
	assert.Empty(t, buf.String(), "the inherited zerolog level still filters")

	child.Warn("kept")
	rec := lastRecord(t, buf)
	assert.Equal(t, "genesys", rec["app"])
	assert.Equal(t, "req-1", rec["request_id"])
}

func TestLogger_WithContextWithoutRequestID(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	logger.WithContext(context.Background()).Info("no request id")

	assert.NotContains(t, lastRecord(t, buf), "request_id")
}

func TestLogger_WithContextLegacyStringKey(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	var legacyKey any = "request_id" // the pre-typed-key convention
	ctx := context.WithValue(context.Background(), legacyKey, "legacy-7")

	logger.WithContext(ctx).Info("legacy key")

	assert.Equal(t, "legacy-7", lastRecord(t, buf)["request_id"])
}

func TestLogger_WithErrorDoesNotMutateParent(t *testing.T) {
	buf := &bytes.Buffer{}
	parent := NewJSON(buf)

	parent.WithError(assert.AnError).Error("child")
	assert.Equal(t, assert.AnError.Error(), lastRecord(t, buf)["error"])

	parent.Error("parent")
	assert.NotContains(t, lastRecord(t, buf), "error")
}

func TestLogger_InlineFieldsEdgeCases(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewJSON(buf)

	// A dangling key without a value is ignored, and non-string keys are skipped.
	logger.Info("edge cases", "kept", "yes", 42, "skipped-key", "dangling")

	rec := lastRecord(t, buf)
	assert.Equal(t, "yes", rec["kept"])
	assert.NotContains(t, rec, "dangling")
	assert.NotContains(t, rec, "42")
	assert.NotContains(t, rec, "skipped-key")
}

func TestLogger_LevelFiltering(t *testing.T) {
	tests := []struct {
		name    string
		level   contracts.LogLevel
		emitted []string
	}{
		{"debug", contracts.LogLevelDebug, []string{"debug", "info", "warn", "error"}},
		{"info", contracts.LogLevelInfo, []string{"info", "warn", "error"}},
		{"warn", contracts.LogLevelWarn, []string{"warn", "error"}},
		{"error", contracts.LogLevelError, []string{"error"}},
		{"fatal", contracts.LogLevelFatal, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := NewJSON(buf)
			logger.SetLevel(tt.level)
			require.Equal(t, tt.level, logger.Level())

			logger.Debug("debug")
			logger.Info("info")
			logger.Warn("warn")
			logger.Error("error")

			levels := []string{}
			for _, rec := range records(t, buf) {
				levels = append(levels, rec["level"].(string))
			}
			assert.Equal(t, tt.emitted, levels)
		})
	}
}

func TestToZerologLevel(t *testing.T) {
	tests := []struct {
		in   contracts.LogLevel
		want zerolog.Level
	}{
		{contracts.LogLevelDebug, zerolog.DebugLevel},
		{contracts.LogLevelInfo, zerolog.InfoLevel},
		{contracts.LogLevelWarn, zerolog.WarnLevel},
		{contracts.LogLevelError, zerolog.ErrorLevel},
		{contracts.LogLevelFatal, zerolog.FatalLevel},
		{contracts.LogLevelPanic, zerolog.PanicLevel},
		{contracts.LogLevel(99), zerolog.InfoLevel}, // unknown levels fall back to info
		{contracts.LogLevel(-1), zerolog.InfoLevel},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, toZerologLevel(tt.in), "level %d", tt.in)
	}
}

func TestNewJSON_DefaultsToStdout(t *testing.T) {
	logger := NewJSON()

	require.NotNil(t, logger)
	assert.Equal(t, contracts.LogLevelInfo, logger.Level())

	// Redirect before emitting anything so the test stays quiet.
	buf := &bytes.Buffer{}
	logger.SetOutput(buf)
	logger.Info("redirected")
	assert.Equal(t, "redirected", lastRecord(t, buf)["message"])
}

// --- stackLogger edge cases -------------------------------------------------

func TestStack_ForeignChannelsOwnTheirFatalAndPanic(t *testing.T) {
	m, _ := bufferedManager(t)
	buf := &bytes.Buffer{}
	foreign := &foreignLogger{}
	m.AddChannel("native", NewJSON(buf))
	m.AddChannel("foreign", foreign)
	codes := stubExit(t)

	stack := m.Stack("native", "foreign")
	stack.Fatal("bye")

	assert.Equal(t, "fatal", lastRecord(t, buf)["level"])
	assert.Equal(t, []string{"fatal:bye"}, foreign.calls, "a foreign channel is asked to Fatal itself")
	assert.Equal(t, []int{1}, *codes)

	assert.PanicsWithValue(t, "boom", func() { stack.Panic("boom") })
	assert.Equal(t, "panic", lastRecord(t, buf)["level"])
	assert.Equal(t, []string{"fatal:bye", "panic:boom"}, foreign.calls)
}

func TestStack_NilChannelFallsBackToDefault(t *testing.T) {
	m, buf := bufferedManager(t)
	m.AddChannel("broken", nil)

	stack := m.Stack("broken")

	assert.Same(t, m.defaultLogger(), stack, "a stack with no usable channel is the default one")
	stack.Info("still logged")
	assert.Equal(t, "still logged", lastRecord(t, buf)["message"])
}

func TestStack_EmptyStackReportsTheDefaultLevel(t *testing.T) {
	assert.Equal(t, contracts.LogLevelInfo, (&stackLogger{}).Level())
}
