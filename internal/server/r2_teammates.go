package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/parser"
	syncpkg "go.kenn.io/agentsview/internal/sync"
)

type r2User struct {
	ID      string `json:"user_id"`
	Name    string `json:"name,omitempty"`
	Email   string `json:"email,omitempty"`
	devices []string
}

type r2Device struct {
	ID      string `json:"device_id"`
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Machine string `json:"machine"`
}

type r2DirectoryData struct {
	users   map[string]r2User
	devices map[string]r2Device
}

type r2Directory struct {
	endpoint  url.URL
	secret    string
	client    *http.Client
	gate      chan struct{}
	data      r2DirectoryData
	updated   time.Time
	attempted time.Time
	manualAt  time.Time
	nextPage  time.Time
	lastErr   error
}

func newR2DirectoryFromEnv() *r2Directory {
	return newR2Directory(os.Getenv("AV_WORKER_URL"), os.Getenv("AV_ADMIN_SECRET"))
}

func newR2Directory(base, secret string) *r2Directory {
	endpoint, err := url.Parse(strings.TrimSpace(base))
	if secret == "" || err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil {
		return nil
	}
	endpoint = endpoint.JoinPath("admin", "devices")
	endpoint.RawQuery, endpoint.Fragment = "", ""
	return &r2Directory{
		endpoint: *endpoint, secret: secret, gate: make(chan struct{}, 1),
		client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (d *r2Directory) lookup(ctx context.Context, refresh bool) (r2DirectoryData, error) {
	if d == nil {
		return r2DirectoryData{}, nil
	}
	select {
	case d.gate <- struct{}{}:
		defer func() { <-d.gate }()
	case <-ctx.Done():
		return r2DirectoryData{}, ctx.Err()
	}
	if !refresh && time.Since(d.updated) < 5*time.Minute {
		return d.data, nil
	}
	// Let the first manual refresh bypass a fresh cache. Coalesce repeated
	// manual clicks and back off on automatic retries after upstream errors.
	now := time.Now()
	if refresh {
		if now.Sub(d.manualAt) < time.Minute {
			return d.data, d.lastErr
		}
		d.manualAt = now
	} else if now.Sub(d.attempted) < time.Minute {
		return d.data, d.lastErr
	}
	d.attempted = now
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	data, err := d.fetch(ctx)
	d.lastErr = err
	if err == nil {
		d.data, d.updated = data, time.Now()
	}
	return d.data, err
}

func (d *r2Directory) fetch(ctx context.Context) (r2DirectoryData, error) {
	data := r2DirectoryData{users: make(map[string]r2User), devices: make(map[string]r2Device)}
	seen := make(map[string]bool)
	for cursor := ""; ; {
		devices, next, err := d.fetchPage(ctx, cursor)
		if err != nil {
			return r2DirectoryData{}, err
		}
		for _, device := range devices {
			if !parser.ValidR2UserID(device.UserID) || device.ID == "" {
				continue
			}
			if _, exists := data.devices[device.ID]; exists {
				continue
			}
			data.devices[device.ID] = device
			user := data.users[device.UserID]
			user.ID = device.UserID
			if user.Name == "" {
				user.Name = device.Name
			}
			if user.Email == "" {
				user.Email = device.Email
			}
			user.devices = append(user.devices, device.ID)
			data.users[user.ID] = user
		}
		if next == "" {
			return data, nil
		}
		if seen[next] {
			return r2DirectoryData{}, errors.New("teammate directory returned a repeated cursor")
		}
		seen[next] = true
		cursor = next
	}
}

func (d *r2Directory) fetchPage(ctx context.Context, cursor string) ([]r2Device, string, error) {
	// Space admin requests across pagination and manual refreshes.
	if wait := time.Until(d.nextPage); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	}
	d.nextPage = time.Now().Add(1100 * time.Millisecond)
	endpoint := d.endpoint
	q := url.Values{"limit": {"100"}, "all": {"1"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", errors.New("could not build teammate directory request")
	}
	req.Header.Set("X-Admin-Secret", d.secret)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, "", errors.New("could not reach teammate directory")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("teammate directory returned HTTP %d", resp.StatusCode)
	}
	var page struct {
		Devices    []r2Device `json:"devices"`
		Items      []r2Device `json:"items"`
		NextCursor string     `json:"next_cursor"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&page); err != nil {
		return nil, "", errors.New("could not decode teammate directory response")
	}
	if page.Devices == nil {
		page.Devices = page.Items
	}
	return page.Devices, page.NextCursor, nil
}

func (d r2DirectoryData) teammate(userID, deviceID string) *db.R2Teammate {
	user := d.users[userID]
	result := &db.R2Teammate{UserID: userID, Name: user.Name, Email: user.Email, Machine: deviceID}
	if device := d.devices[deviceID]; device.UserID == userID && device.Machine != "" {
		result.Machine = device.Machine
	}
	return result
}

// Resolve once per response, and only when it contains R2 sessions.
func (s *Server) r2TeammateLookup(ctx context.Context) func(string) *db.R2Teammate {
	load := sync.OnceValue(func() r2DirectoryData {
		data, _ := s.r2Directory.lookup(ctx, false)
		return data
	})
	return func(id string) *db.R2Teammate {
		user, device, ok := parser.R2SessionOwner(id)
		if !ok {
			return nil
		}
		return load().teammate(user, device)
	}
}

type r2TeammatesResponse struct {
	Configured bool     `json:"configured"`
	Users      []r2User `json:"users"`
	Error      string   `json:"error,omitempty"`
}

func (s *Server) r2Teammates(ctx context.Context, refresh bool) (*jsonOutput[r2TeammatesResponse], error) {
	data, err := s.r2Directory.lookup(ctx, refresh)
	if err != nil && refresh {
		return nil, apiError(http.StatusBadGateway, err.Error())
	}
	users := make([]r2User, 0, len(data.users))
	for _, user := range data.users {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b r2User) int {
		return strings.Compare(a.ID, b.ID)
	})
	result := r2TeammatesResponse{Configured: s.r2Directory != nil, Users: users}
	if err != nil {
		result.Error = err.Error()
	}
	return &jsonOutput[r2TeammatesResponse]{Body: result}, nil
}

func (s *Server) humaR2Teammates(ctx context.Context, _ *struct{}) (*jsonOutput[r2TeammatesResponse], error) {
	return s.r2Teammates(ctx, false)
}

func (s *Server) humaRefreshR2Teammates(ctx context.Context, _ *struct{}) (*jsonOutput[r2TeammatesResponse], error) {
	return s.r2Teammates(ctx, true)
}

type r2UserSyncInput struct {
	Body struct {
		UserID string `json:"user_id"`
	}
}

func (s *Server) humaSyncR2User(ctx context.Context, in *r2UserSyncInput) (*struct{}, error) {
	if !parser.ValidR2UserID(in.Body.UserID) {
		return nil, apiError(http.StatusBadRequest, "invalid R2 user ID")
	}
	if s.db.ReadOnly() {
		return &struct{}{}, nil
	}
	engine, err := s.syncEngineForRequest(ctx)
	if err != nil {
		return nil, err
	}
	if local, ok := s.db.(*db.DB); ok && local.NeedsResync() {
		return nil, apiError(http.StatusConflict, "archive needs a rebuild")
	}
	if err := engine.SyncR2UserContext(ctx, in.Body.UserID); err != nil {
		if errors.Is(err, syncpkg.ErrSyncInProgress) {
			return nil, apiError(http.StatusConflict, err.Error())
		}
		return nil, serverError(err)
	}
	return &struct{}{}, nil
}
