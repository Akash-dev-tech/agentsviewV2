package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestR2TeammatesFetchesAllPagesAndCachesDevices(t *testing.T) {
	const user = "usr_11111111-1111-4111-8111-111111111111"
	const first = "dev_22222222-2222-4222-8222-222222222222"
	const second = "dev_33333333-3333-4333-8333-333333333333"
	var requests atomic.Int32
	worker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/admin/devices", r.URL.Path)
		assert.Equal(t, "fixture-secret", r.Header.Get("X-Admin-Secret"))
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		assert.Equal(t, "1", r.URL.Query().Get("all"))
		switch requests.Add(1) {
		case 1:
			assert.Empty(t, r.URL.Query().Get("cursor"))
			_, _ = io.WriteString(w, `{"devices":[{"device_id":"`+first+`","user_id":"`+user+`","name":"Sam Lee","email":"sam@example.net","machine":"Laptop"}],"next_cursor":"a+b/="}`)
		case 2:
			assert.Equal(t, "a+b/=", r.URL.Query().Get("cursor"))
			_, _ = io.WriteString(w, `{"devices":[{"device_id":"`+second+`","user_id":"`+user+`","machine":"Desktop"}],"next_cursor":null}`)
		default:
			assert.Empty(t, r.URL.Query().Get("cursor"))
			_, _ = io.WriteString(w, `{"devices":[{"device_id":"`+first+`","user_id":"`+user+`","name":"Sam Morgan","machine":"Laptop"}],"next_cursor":null}`)
		}
	}))
	t.Cleanup(worker.Close)
	directory := newR2Directory(worker.URL, "fixture-secret")
	require.NotNil(t, directory)
	directory.client.Transport = worker.Client().Transport
	s := &Server{r2Directory: directory}
	response, err := s.humaR2Teammates(t.Context(), &struct{}{})
	require.NoError(t, err)
	require.Len(t, response.Body.Users, 1)
	assert.Equal(t, "Sam Lee", response.Body.Users[0].Name)
	assert.Equal(t, "sam@example.net", response.Body.Users[0].Email)
	assert.Equal(t, []string{first, second}, response.Body.Users[0].devices)
	data, err := directory.lookup(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, "Desktop", data.teammate(user, second).Machine)
	assert.Equal(t, int32(2), requests.Load())
	response, err = s.humaRefreshR2Teammates(t.Context(), &struct{}{})
	require.NoError(t, err)
	require.Len(t, response.Body.Users, 1)
	assert.Equal(t, "Sam Morgan", response.Body.Users[0].Name)
	assert.Equal(t, int32(3), requests.Load())
	_, err = s.humaRefreshR2Teammates(t.Context(), &struct{}{})
	require.NoError(t, err)
	assert.Equal(t, int32(3), requests.Load())
}
