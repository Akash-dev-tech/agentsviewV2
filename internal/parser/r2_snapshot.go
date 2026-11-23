package parser

import (
	"context"
	"regexp"
	"strings"
)

const r2UUID = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

var (
	r2UserID     = regexp.MustCompile(`^usr_` + r2UUID + `$`)
	r2DeviceID   = regexp.MustCompile(`^dev_` + r2UUID + `$`)
	r2SnapshotID = regexp.MustCompile(`^(sha256-[0-9a-fA-F]{64}|upload-` + r2UUID + `)$`)
	r2PathHash   = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

type R2SnapshotKey struct {
	UserID, DeviceID, SnapshotID, PathHash, Filename string
	Agent                                            AgentType
}

func ValidR2UserID(id string) bool { return r2UserID.MatchString(id) }

func ParseR2SnapshotKey(key string) (R2SnapshotKey, bool) {
	p := strings.Split(key, "/")
	if len(p) != 10 || p[0] != "users" || !ValidR2UserID(p[1]) ||
		p[2] != "devices" || !r2DeviceID.MatchString(p[3]) ||
		(p[4] != "codex" && p[4] != "claude") || p[5] != "snapshots" ||
		!r2SnapshotID.MatchString(p[6]) || p[7] != "p" || !r2PathHash.MatchString(p[8]) ||
		len(p[9]) <= len(".jsonl") || !strings.HasSuffix(p[9], ".jsonl") || strings.ContainsAny(p[9], "\\\x00") {
		return R2SnapshotKey{}, false
	}
	return R2SnapshotKey{p[1], p[3], p[6], p[8], p[9], AgentType(p[4])}, true
}

func R2SnapshotFromURI(uri string) (R2SnapshotKey, bool) {
	if !strings.HasPrefix(uri, "s3://") {
		return R2SnapshotKey{}, false
	}
	_, key := parseS3URI(uri)
	return ParseR2SnapshotKey(key)
}

// Namespace isolates copies of a transcript at different paths and remains
// constant across snapshot uploads.
func (k R2SnapshotKey) Namespace() string {
	return "r2." + k.UserID + "." + k.DeviceID + "." + k.PathHash
}

// SessionID keeps the primary archive row stable even if a later upload uses
// a different object filename for the same logical path.
func (k R2SnapshotKey) SessionID() string {
	if k.Agent == AgentCodex {
		return "codex:" + k.PathHash
	}
	return k.PathHash
}

func R2SessionOwner(id string) (userID, deviceID string, ok bool) {
	host, _ := StripHostPrefix(id)
	p := strings.Split(host, ".")
	if len(p) != 4 || p[0] != "r2" || !ValidR2UserID(p[1]) || !r2DeviceID.MatchString(p[2]) || !r2PathHash.MatchString(p[3]) {
		return "", "", false
	}
	return p[1], p[2], true
}

// R2UserRoot intersects a configured bucket/users root with the selected user.
// Legacy roots outside that prefix remain available to regular discovery.
func R2UserRoot(root, userID string) (string, bool) {
	if !ValidR2UserID(userID) || !strings.HasPrefix(root, "s3://") {
		return "", false
	}
	bucket, key := parseS3URI(root)
	userKey := "users/" + userID
	switch key = strings.TrimSuffix(key, "/"); {
	case bucket == "":
		return "", false
	case key == "", key == "users", key == userKey:
		return "s3://" + bucket + "/" + userKey, true
	case strings.HasPrefix(key, userKey+"/"):
		return strings.TrimSuffix(root, "/"), true
	default:
		return "", false
	}
}

func ListR2SnapshotObjects(ctx context.Context, root string) ([]S3Object, error) {
	objects, err := listS3Context(ctx, root)
	if err != nil {
		return nil, err
	}
	return latestR2Snapshots(objects), nil
}

func latestR2Snapshots(objects []S3Object) []S3Object {
	type logicalKey struct {
		bucket, user, device, hash string
		agent                      AgentType
	}
	positions := make(map[logicalKey]int)
	var latest []S3Object
	for _, obj := range objects {
		k, ok := R2SnapshotFromURI(obj.URI)
		if !ok {
			continue
		}
		bucket, _ := parseS3URI(obj.URI)
		key := logicalKey{bucket, k.UserID, k.DeviceID, k.PathHash, k.Agent}
		i, found := positions[key]
		if !found {
			positions[key] = len(latest)
			latest = append(latest, obj)
		} else if current := latest[i]; obj.LastModified.After(current.LastModified) ||
			(obj.LastModified.Equal(current.LastModified) && obj.URI < current.URI) {
			latest[i] = obj
		}
	}
	return latest
}

func R2SnapshotFile(obj S3Object) (DiscoveredFile, bool) {
	k, ok := R2SnapshotFromURI(obj.URI)
	if !ok || (k.Agent == AgentCodex && CodexSessionUUIDFromFilename(k.Filename) == "") ||
		(k.Agent == AgentClaude && k.Filename == "sessions-index.jsonl") {
		return DiscoveredFile{}, false
	}
	return DiscoveredFile{
		Path: obj.URI, Agent: k.Agent, Machine: k.DeviceID,
		SourceSize: obj.Size, SourceMtime: obj.LastModified.UnixNano(), SourceFingerprint: obj.Fingerprint,
		TranscriptSize: obj.Size, TranscriptMtime: obj.LastModified.UnixNano(),
	}, true
}
