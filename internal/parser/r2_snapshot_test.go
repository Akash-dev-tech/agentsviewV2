package parser

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	r2TestUser   = "usr_11111111-1111-4111-8111-111111111111"
	r2TestDevice = "dev_22222222-2222-4222-8222-222222222222"
	r2TestUpload = "upload-33333333-3333-4333-8333-333333333333"
)

func TestParseR2SnapshotKey(t *testing.T) {
	hash := strings.Repeat("b", 64)
	key := "users/" + r2TestUser + "/devices/" + r2TestDevice + "/codex/snapshots/sha256-" + strings.Repeat("a", 64) + "/p/" + hash + "/rollout.jsonl"
	parsed, ok := ParseR2SnapshotKey(key)
	require.True(t, ok)
	assert.Equal(t, R2SnapshotKey{r2TestUser, r2TestDevice, "sha256-" + strings.Repeat("a", 64), hash, "rollout.jsonl", AgentCodex}, parsed)
	for _, invalid := range []string{
		"users/sam@example.net/workstation/codex/rollout.jsonl",
		strings.Replace(key, "/codex/", "/cursor/", 1),
		strings.Replace(key, "/p/", "/paths/", 1),
		strings.Replace(key, hash, "bad-hash", 1),
		strings.Replace(key, "sha256-", "md5-", 1),
		key + ".bak",
	} {
		_, ok := ParseR2SnapshotKey(invalid)
		assert.False(t, ok, invalid)
	}
}

func TestR2SnapshotDiscoveryChoosesNewestLogicalFile(t *testing.T) {
	root := "s3://example/users"
	prefix := root + "/" + r2TestUser + "/devices/" + r2TestDevice + "/claude/snapshots/"
	hash := strings.Repeat("b", 64)
	older := prefix + "sha256-" + strings.Repeat("f", 64) + "/p/" + hash + "/old.jsonl"
	newer := prefix + r2TestUpload + "/p/" + hash + "/session.jsonl"
	otherPath := prefix + r2TestUpload + "/p/" + strings.Repeat("c", 64) + "/session.jsonl"
	otherDevice := strings.Replace(newer, r2TestDevice, "dev_44444444-4444-4444-8444-444444444444", 1)
	otherUser := strings.Replace(newer, r2TestUser, "usr_55555555-5555-4555-8555-555555555555", 1)
	otherAgent := strings.Replace(newer, "/claude/", "/codex/", 1)
	timeOne := time.Unix(100, 0)
	timeTwo := time.Unix(200, 0)
	oldList := listS3Objects
	t.Cleanup(func() { listS3Objects = oldList })
	listS3Objects = func(got string) ([]S3Object, error) {
		assert.Equal(t, root, got)
		return []S3Object{
			{URI: newer, LastModified: timeTwo, Size: 100},
			{URI: older, LastModified: timeOne, Size: 200},
			{URI: otherPath, LastModified: timeOne},
			{URI: otherDevice, LastModified: timeOne},
			{URI: otherUser, LastModified: timeOne},
			{URI: otherAgent, LastModified: timeTwo},
		}, nil
	}
	files := s3PrefixScan(root, claudeS3Scanner())
	require.Len(t, files, 4)
	assert.Equal(t, newer, files[0].Path)
	assert.Equal(t, timeTwo.UnixNano(), files[0].SourceMtime)
	assert.EqualValues(t, 100, files[0].SourceSize)
	assert.Equal(t, r2TestDevice, files[0].Machine)
	assert.Equal(t, otherPath, files[1].Path)
	assert.Equal(t, otherDevice, files[2].Path)
	assert.Equal(t, otherUser, files[3].Path)
}

func TestR2SelectedUserListsOnlyTheirPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/example", r.URL.Path)
		assert.Equal(t, "users/"+r2TestUser+"/", r.URL.Query().Get("prefix"))
		assert.Equal(t, "2", r.URL.Query().Get("list-type"))
		_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AWS_S3_ENDPOINT", server.URL)
	t.Setenv("AWS_REGION", "auto")
	t.Setenv("AWS_ACCESS_KEY_ID", "fixture-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-secret")
	root, ok := R2UserRoot("s3://example/users", r2TestUser)
	require.True(t, ok)
	_, err := ListR2SnapshotObjects(t.Context(), root)
	require.NoError(t, err)
}
