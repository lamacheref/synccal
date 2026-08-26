package storage

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewAndPing(t *testing.T) {
	s := newTestStore(t)
	assert.NoError(t, s.Ping())
	assert.NoError(t, s.Close())
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := New("/nonexistent/dir/test.db")
	assert.Error(t, err)
}

func TestSetAndGetMapping(t *testing.T) {
	s := newTestStore(t)
	err := s.SetMapping("uid-1", "dest1", "uid-1", "/dest1/uid-1.ics", "etag1", "hash1", false)
	require.NoError(t, err)

	uid, href, etag, deleted, err := s.GetMapping("uid-1", "dest1")
	require.NoError(t, err)
	assert.Equal(t, "uid-1", uid)
	assert.Equal(t, "/dest1/uid-1.ics", href)
	assert.Equal(t, "etag1", etag)
	assert.False(t, deleted)
}

func TestGetMapping_NotFound(t *testing.T) {
	s := newTestStore(t)
	uid, href, etag, deleted, err := s.GetMapping("nonexistent", "dest1")
	require.NoError(t, err)
	assert.Empty(t, uid)
	assert.Empty(t, href)
	assert.Empty(t, etag)
	assert.False(t, deleted)
}

func TestSetMapping_Update(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.SetMapping("uid-1", "dest1", "uid-1", "/href1", "etag1", "hash1", false))
	require.NoError(t, s.SetMapping("uid-1", "dest1", "uid-1", "/href2", "etag2", "hash2", true))
	_, href, etag, deleted, err := s.GetMapping("uid-1", "dest1")
	require.NoError(t, err)
	assert.Equal(t, "/href2", href)
	assert.Equal(t, "etag2", etag)
	assert.True(t, deleted)
}

func TestListMappings(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.SetMapping("a-uid1", "dest1", "a-uid1", "/a/1", "", "h1", false))
	require.NoError(t, s.SetMapping("a-uid2", "dest1", "a-uid2", "/a/2", "", "h2", false))
	require.NoError(t, s.SetMapping("b-uid1", "dest1", "b-uid1", "/b/1", "", "h3", false))
	require.NoError(t, s.SetMapping("a-uid3", "dest1", "a-uid3", "/a/3", "", "h4", true)) // deleted

	// No prefix filter
	m, err := s.ListMappings("dest1", "")
	require.NoError(t, err)
	assert.Len(t, m, 3)
	assert.Equal(t, "h1", m["a-uid1"])
	assert.NotContains(t, m, "a-uid3")

	// Prefix filter
	m, err = s.ListMappings("dest1", "a")
	require.NoError(t, err)
	assert.Len(t, m, 2)
	assert.Contains(t, m, "a-uid1")
	assert.Contains(t, m, "a-uid2")
	assert.NotContains(t, m, "b-uid1")

	// Different dest
	m, err = s.ListMappings("dest2", "")
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestListEvents(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.SetMapping("uid1", "dest1", "uid1", "/d1/uid1", "etag1", "h1", false))
	require.NoError(t, s.SetMapping("uid2", "dest1", "uid2", "/d1/uid2", "etag2", "h2", true))
	require.NoError(t, s.SetMapping("uid3", "dest2", "uid3", "/d2/uid3", "etag3", "h3", false))

	// All
	evs, err := s.ListEvents("", 10, 0)
	require.NoError(t, err)
	assert.Len(t, evs, 3)

	// Filter by dest
	evs, err = s.ListEvents("dest1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, evs, 2)

	// Pagination
	evs, err = s.ListEvents("", 1, 0)
	require.NoError(t, err)
	assert.Len(t, evs, 1)
	evs, err = s.ListEvents("", 1, 1)
	require.NoError(t, err)
	assert.Len(t, evs, 1)

	// Limit edge cases
	evs, err = s.ListEvents("", 0, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, evs)
	evs, err = s.ListEvents("", 2000, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, evs)
	evs, err = s.ListEvents("", 10, -5)
	require.NoError(t, err)
	assert.NotEmpty(t, evs)
}

func TestGetSetSourceState(t *testing.T) {
	s := newTestStore(t)
	// Not found returns empty
	st, err := s.GetSourceState("https://example.com/cal.ics")
	require.NoError(t, err)
	assert.Empty(t, st.CTag)

	// Set and get
	require.NoError(t, s.SetSourceState("https://example.com/cal.ics", &CalendarState{CTag: "ctag1", SyncToken: "tok1", ETag: "etag1"}))
	st, err = s.GetSourceState("https://example.com/cal.ics")
	require.NoError(t, err)
	assert.Equal(t, "ctag1", st.CTag)
	assert.Equal(t, "tok1", st.SyncToken)
	assert.Equal(t, "etag1", st.ETag)

	// Update
	require.NoError(t, s.SetSourceState("https://example.com/cal.ics", &CalendarState{CTag: "ctag2"}))
	st, err = s.GetSourceState("https://example.com/cal.ics")
	require.NoError(t, err)
	assert.Equal(t, "ctag2", st.CTag)
	assert.Empty(t, st.SyncToken)
}

func TestStore_MultipleDestinationsIsolation(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.SetMapping("uid1", "dest1", "uid1", "/d1/uid1", "", "h1", false))
	require.NoError(t, s.SetMapping("uid1", "dest2", "uid1", "/d2/uid1", "", "h2", false))
	uid, _, _, _, err := s.GetMapping("uid1", "dest1")
	require.NoError(t, err)
	assert.Equal(t, "uid1", uid)
	m1, _ := s.ListMappings("dest1", "")
	m2, _ := s.ListMappings("dest2", "")
	assert.Len(t, m1, 1)
	assert.Len(t, m2, 1)
	assert.Equal(t, "h1", m1["uid1"])
	assert.Equal(t, "h2", m2["uid1"])
}
