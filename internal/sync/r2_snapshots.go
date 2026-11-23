package sync

import (
	"context"
	"errors"
	"fmt"

	"go.kenn.io/agentsview/internal/parser"
)

// SyncR2UserContext imports one user's current snapshots through the normal
// archive writer. A scoped listing does not reconcile missing source paths.
func (e *Engine) SyncR2UserContext(ctx context.Context, userID string) error {
	if e.sourceSyncErr != nil {
		return e.sourceSyncErr
	}
	if e.refuseWriteInForceParse("SyncR2UserContext") {
		return nil
	}
	if err := e.lockSync(true); err != nil {
		return err
	}
	changed := false
	defer func() {
		e.syncMu.Unlock()
		if changed {
			e.emit("sessions")
			e.emit("messages")
		}
	}()
	ctx, release := withS3CodexParentCache(ctx)
	defer release()
	roots := make(map[string]map[parser.AgentType]bool)
	for _, agent := range []parser.AgentType{parser.AgentClaude, parser.AgentCodex} {
		for _, configured := range e.sources().agentDirs[agent] {
			root, ok := parser.R2UserRoot(configured, userID)
			if !ok {
				continue
			}
			if roots[root] == nil {
				roots[root] = make(map[parser.AgentType]bool)
			}
			roots[root][agent] = true
		}
	}
	var files []parser.DiscoveredFile
	var errs error
	for root, agents := range roots {
		objects, err := parser.ListR2SnapshotObjects(ctx, root)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		for _, obj := range objects {
			if file, ok := parser.R2SnapshotFile(obj); ok && agents[file.Agent] {
				if key, _ := parser.R2SnapshotFromURI(file.Path); key.UserID == userID {
					files = append(files, file)
				}
			}
		}
	}
	for _, file := range dedupeDiscoveredFiles(files) {
		if err := ctx.Err(); err != nil {
			return errors.Join(errs, err)
		}
		preserved, sessionsChanged, err := e.processAndWriteSessionFile(ctx, file, s3DiscoveredSessionID(file))
		changed = changed || sessionsChanged || (!preserved && err == nil)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("import R2 snapshot: %w", err))
		}
	}
	return errs
}
