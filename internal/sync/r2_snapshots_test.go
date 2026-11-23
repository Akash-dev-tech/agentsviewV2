package sync

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/parser"
	"go.kenn.io/agentsview/internal/testjsonl"
)

func TestR2SyncReplacesTheArchivedSnapshot(t *testing.T) {
	const user = "usr_11111111-1111-4111-8111-111111111111"
	const device = "dev_22222222-2222-4222-8222-222222222222"
	const oldThread = "33333333-3333-4333-8333-333333333333"
	const newThread = "44444444-4444-4444-8444-444444444444"
	const prefix = "users/" + user + "/"
	hash := strings.Repeat("a", 64)
	base := prefix + "devices/" + device + "/codex/snapshots/"
	oldKey := base + "sha256-" + strings.Repeat("f", 64) + "/p/" + hash + "/rollout-2026-01-01T00-00-00-" + oldThread + ".jsonl"
	newKey := base + "upload-55555555-5555-4555-8555-555555555555/p/" + hash + "/rollout-2026-01-01T00-00-00-" + newThread + ".jsonl"
	transcript := func(thread, message string) string {
		return testjsonl.JoinJSONL(
			testjsonl.CodexSessionMetaJSON(thread, "/example/project", "user", "2026-01-01T00:00:00Z"),
			testjsonl.CodexMsgJSON("user", message, "2026-01-01T00:00:01Z"),
		)
	}
	oldBody, newBody := transcript(oldThread, "Old snapshot"), transcript(newThread, "New snapshot")
	var uploaded atomic.Bool
	var oldDownloads, newDownloads atomic.Int32
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			if uploaded.Load() {
				assert.Equal(t, prefix, r.URL.Query().Get("prefix"))
			} else {
				assert.Equal(t, "users/", r.URL.Query().Get("prefix"))
			}
			_, _ = io.WriteString(w, "<ListBucketResult><IsTruncated>false</IsTruncated>")
			writeEntry := func(key, modified, body string) {
				_, _ = fmt.Fprintf(w, "<Contents><Key>%s</Key><LastModified>%s</LastModified><Size>%d</Size></Contents>", key, modified, len(body))
			}
			writeEntry(oldKey, "2026-01-01T00:00:00Z", oldBody)
			if uploaded.Load() {
				writeEntry(newKey, "2026-01-02T00:00:00Z", newBody)
			}
			_, _ = io.WriteString(w, "</ListBucketResult>")
			return
		}
		switch r.URL.Path {
		case "/example/" + oldKey:
			oldDownloads.Add(1)
			_, _ = io.WriteString(w, oldBody)
		case "/example/" + newKey:
			newDownloads.Add(1)
			_, _ = io.WriteString(w, newBody)
		default:
			assert.Fail(t, "unexpected S3 request", "%s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(store.Close)
	t.Setenv("AWS_S3_ENDPOINT", store.URL)
	t.Setenv("AWS_REGION", "auto")
	t.Setenv("AWS_ACCESS_KEY_ID", "fixture-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-secret")
	ctx := t.Context()
	archive := openTestDB(t)
	engine := NewEngine(ctx, archive, EngineConfig{
		AgentDirs: map[parser.AgentType][]string{parser.AgentCodex: {"s3://example/users"}},
		Machine:   "central",
	})
	t.Cleanup(engine.Close)
	require.Equal(t, 1, engine.SyncAll(ctx, nil).Synced)
	before, err := archive.ListSessions(ctx, db.SessionFilter{R2UserID: user})
	require.NoError(t, err)
	require.Len(t, before.Sessions, 1)
	assert.Equal(t, new("Old snapshot"), before.Sessions[0].FirstMessage)

	uploaded.Store(true)
	require.NoError(t, engine.SyncR2UserContext(ctx, user))
	after, err := archive.ListSessions(ctx, db.SessionFilter{R2UserID: user})
	require.NoError(t, err)
	require.Len(t, after.Sessions, 1)
	assert.Equal(t, before.Sessions[0].ID, after.Sessions[0].ID)
	assert.Equal(t, new("New snapshot"), after.Sessions[0].FirstMessage)
	assert.Equal(t, device, after.Sessions[0].Machine)
	assert.Equal(t, "s3://example/"+newKey, archive.GetSessionFilePath(ctx, after.Sessions[0].ID))
	assert.Equal(t, int32(1), oldDownloads.Load())
	assert.Equal(t, int32(1), newDownloads.Load())
}
