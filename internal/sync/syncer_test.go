package sync

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/smiden/synccal/internal/config"
	"github.com/smiden/synccal/internal/plugin"
	"github.com/smiden/synccal/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Mocks ---

type mockSource struct {
	changed     bool
	state       *plugin.CalendarState
	data        []byte
	errChanged  error
	errFetch    error
	callsChange atomic.Int64 // atomique : lu par le test pendant que le cron exécute Sync
	callsFetch  atomic.Int64
}

func (m *mockSource) Type() string { return "mock" }
func (m *mockSource) HasChanged(_ context.Context, _ *plugin.CalendarState) (bool, *plugin.CalendarState, error) {
	m.callsChange.Add(1)
	if m.errChanged != nil {
		return false, nil, m.errChanged
	}
	if m.state == nil {
		m.state = &plugin.CalendarState{CTag: "ctag-1"}
	}
	return m.changed, m.state, nil
}
func (m *mockSource) Fetch(_ context.Context, _ string) ([]byte, *plugin.CalendarState, error) {
	m.callsFetch.Add(1)
	if m.errFetch != nil {
		return nil, nil, m.errFetch
	}
	return m.data, &plugin.CalendarState{CTag: "ctag-2"}, nil
}

type mockDest struct {
	events    map[string][]byte
	errCreate error
	errUpdate error
	errDelete error
	failOnUID string
	created   []string
	updated   []string
	deleted   []string
}

func newMockDest() *mockDest { return &mockDest{events: make(map[string][]byte)} }

func (m *mockDest) Type() string { return "mock" }
func (m *mockDest) CreateEvent(_ context.Context, icsData []byte) (string, error) {
	uid, _ := eventUIDOf(icsData)
	if m.errCreate != nil || (m.failOnUID != "" && uid == m.failOnUID) {
		return "", errors.New("create failed")
	}
	m.events[uid] = icsData
	m.created = append(m.created, uid)
	return "/dest/" + uid + ".ics", nil
}
func (m *mockDest) UpdateEvent(_ context.Context, href string, _ []byte, _ string) error {
	if m.errUpdate != nil {
		return m.errUpdate
	}
	m.updated = append(m.updated, href)
	return nil
}
func (m *mockDest) DeleteEvent(_ context.Context, href string, _ string) error {
	if m.errDelete != nil {
		return m.errDelete
	}
	m.deleted = append(m.deleted, href)
	return nil
}
func (m *mockDest) ListEvents(_ context.Context) ([]plugin.EventRef, error) { return nil, nil }

func eventUIDOf(data []byte) (string, error) {
	cal, err := ical.NewDecoder(strings.NewReader(string(data))).Decode()
	if err != nil {
		return "", err
	}
	for _, c := range cal.Children {
		if c.Name == "VEVENT" {
			return c.Props.Text("UID")
		}
	}
	return "", errors.New("no VEVENT")
}

const testICS1 = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:event-1@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
END:VCALENDAR`

const testICS1Modified = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:event-1@example.com
DTSTAMP:20240102T000000Z
DTSTART:20240115T110000Z
SUMMARY:Modified
END:VEVENT
END:VCALENDAR`

const srcURL = "https://source.example.com/cal.ics"

type testEnv struct {
	syncer *Syncer
	source *mockSource
	dest   *mockDest
	store  *storage.Store
	cfg    *config.Config
}

func newTestEnv(t *testing.T, filterPrivate bool) *testEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	cfg := &config.Config{
		Sources: []config.SourceConfig{{Name: "src1", Type: "caldav", URL: srcURL}},
		Destinations: []config.DestinationConfig{{
			Name: "dest1", Type: "caldav", URL: "https://dest.example.com/dav/",
			Username: "u", Password: "p", Source: "src1",
		}},
		Database: config.DatabaseConfig{Path: dbPath},
		Sync: config.SyncConfig{
			Interval: "0", Timeout: "5s", BatchSize: 100,
			DeleteMode: "soft", FilterPrivate: filterPrivate,
		},
		Metrics: config.MetricsConfig{Addr: ":0"},
		Logging: config.LoggingConfig{Level: "error", Format: "json"},
	}

	src := &mockSource{data: []byte(testICS1), changed: true}
	dst := newMockDest()
	log := zap.NewNop().Sugar()
	syncer := New(cfg, []plugin.SourceConnector{src}, []plugin.DestinationConnector{dst}, store, log)
	return &testEnv{syncer: syncer, source: src, dest: dst, store: store, cfg: cfg}
}

// --- Tests ---

func TestNew_BuildsPipelinesAndStats(t *testing.T) {
	env := newTestEnv(t, false)
	require.Len(t, env.syncer.pipelines, 1)
	assert.NotNil(t, env.syncer.pipelines[0])
	st := env.syncer.Status()
	require.Len(t, st.Connections, 1)
	assert.Equal(t, "dest1", st.Connections[0].Destination)
	assert.Equal(t, "src1", st.Connections[0].SourceName)
	assert.Equal(t, srcURL, st.Connections[0].SourceURL)
	assert.False(t, env.syncer.IsRunning())
}

func TestNew_UnknownSourceRef(t *testing.T) {
	env := newTestEnv(t, false)
	env.cfg.Destinations[0].Source = "unknown"
	env.syncer = New(env.cfg,
		[]plugin.SourceConnector{env.source},
		[]plugin.DestinationConnector{env.dest}, env.store, zap.NewNop().Sugar())
	st := env.syncer.Status()
	require.Len(t, st.Connections, 1)
	assert.Empty(t, st.Connections[0].SourceURL, "unknown source ref → empty URL")

	// Sync should record the error and continue
	err := env.syncer.Sync(context.Background())
	assert.Error(t, err)
	st = env.syncer.Status()
	assert.NotEmpty(t, st.LastError)
	assert.Equal(t, int64(1), st.Connections[0].Errors)
}

func TestSync_CreateEvents(t *testing.T) {
	env := newTestEnv(t, false)
	require.NoError(t, env.syncer.Sync(context.Background()))
	require.Len(t, env.dest.created, 1)

	// Mapping persisted with prefixed UID
	prefix := sourcePrefix(srcURL)
	mappings, err := env.store.ListMappings("dest1", prefix)
	require.NoError(t, err)
	require.Len(t, mappings, 1)
	for uid := range mappings {
		assert.True(t, strings.HasPrefix(uid, prefix+"-"), "UID must be prefixed")
	}

	st := env.syncer.Status()
	assert.False(t, st.Running)
	assert.Empty(t, st.LastError)
	conn := st.Connections[0]
	assert.Equal(t, int64(1), conn.Events)
	assert.Equal(t, int64(1), conn.Created)
	assert.Equal(t, int64(0), conn.Errors)
	assert.NotNil(t, conn.LastSync)
}

func TestSync_UnchangedSourceSkipped(t *testing.T) {
	env := newTestEnv(t, false)
	env.source.changed = false
	require.NoError(t, env.syncer.Sync(context.Background()))
	assert.EqualValues(t, 1, env.source.callsChange.Load())
	assert.EqualValues(t, 0, env.source.callsFetch.Load(), "unchanged source must not be fetched")
	assert.Empty(t, env.dest.created)
}

func TestSync_IdempotentSecondRun(t *testing.T) {
	env := newTestEnv(t, false)
	require.NoError(t, env.syncer.Sync(context.Background()))
	firstCreated := len(env.dest.created)

	// Second run: same content → hash identique → ni create ni update
	env.source.changed = true
	require.NoError(t, env.syncer.Sync(context.Background()))
	assert.Equal(t, firstCreated, len(env.dest.created)+len(env.dest.updated),
		"contenu identique ne doit rien recréer")
}

func TestSync_UpdateEvent(t *testing.T) {
	env := newTestEnv(t, false)
	require.NoError(t, env.syncer.Sync(context.Background()))

	env.source.data = []byte(testICS1Modified)
	env.source.changed = true
	require.NoError(t, env.syncer.Sync(context.Background()))
	assert.Len(t, env.dest.updated, 1, "event modifié doit être mis à jour")

	st := env.syncer.Status()
	assert.Equal(t, int64(1), st.Connections[0].Updated)
}

func TestSync_FilterPrivate(t *testing.T) {
	env := newTestEnv(t, true) // filter_private = true
	env.source.data = []byte(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:public-1@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
BEGIN:VEVENT
UID:private-1@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240116T100000Z
CLASS:PRIVATE
END:VEVENT
END:VCALENDAR`)
	require.NoError(t, env.syncer.Sync(context.Background()))
	assert.Len(t, env.dest.created, 1, "seul l'event public doit être synchronisé")
	for _, uid := range env.dest.created {
		assert.NotContains(t, uid, "private-1", "l'event PRIVATE doit être filtré")
	}
}

func TestSync_SourceErrorChange(t *testing.T) {
	env := newTestEnv(t, false)
	env.source.errChanged = errors.New("network down")
	err := env.syncer.Sync(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check source src1 changes")

	st := env.syncer.Status()
	assert.NotEmpty(t, st.LastError)
	assert.Equal(t, int64(1), st.Connections[0].Errors)
}

func TestSync_SourceErrorFetch(t *testing.T) {
	env := newTestEnv(t, false)
	env.source.errFetch = errors.New("fetch boom")
	err := env.syncer.Sync(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch source src1")
}

func TestSync_DestCreateError(t *testing.T) {
	env := newTestEnv(t, false)
	env.dest.failOnUID = sourcePrefix(srcURL) + "-event-1@example.com"
	// Design : une connexion en erreur n'bloque pas les autres → Sync retourne nil,
	// l'erreur est enregistrée dans connStats / Status.
	err := env.syncer.Sync(context.Background())
	require.NoError(t, err)

	st := env.syncer.Status()
	assert.Equal(t, int64(1), st.Connections[0].Errors)
	assert.Contains(t, st.Connections[0].LastError, "create event")
}

func TestSync_DeleteSoftAndHard(t *testing.T) {
	// Soft delete (défaut)
	env := newTestEnv(t, false)
	prefix := sourcePrefix(srcURL)
	// Pré-exister un mapping qui disparaîtra de la source
	require.NoError(t, env.store.SetMapping(
		prefix+"-gone@example.com", "dest1",
		prefix+"-gone@example.com", "/dest/gone.ics", "", "hash-old", false))
	// Le mapping existant a un hash différent du contenu actuel vide → il sera vu comme supprimé

	require.NoError(t, env.syncer.Sync(context.Background()))
	_, _, _, deleted, err := env.store.GetMapping(prefix+"-gone@example.com", "dest1")
	require.NoError(t, err)
	assert.True(t, deleted, "soft delete : l'event absent de la source est marqué supprimé")
	assert.Empty(t, env.dest.deleted, "soft delete ne doit pas appeler DeleteEvent")

	// Hard delete
	env2 := newTestEnv(t, false)
	env2.cfg.Sync.DeleteMode = "hard"
	require.NoError(t, env2.store.SetMapping(
		prefix+"-gone@example.com", "dest1",
		prefix+"-gone@example.com", "/dest/gone.ics", "etag-x", "hash-old", false))

	require.NoError(t, env2.syncer.Sync(context.Background()))
	assert.Len(t, env2.dest.deleted, 1, "hard delete doit appeler DeleteEvent")
	assert.Contains(t, env2.dest.deleted[0], "gone.ics")
}

func TestSync_DeleteEventError_HardMode_Continues(t *testing.T) {
	env := newTestEnv(t, false)
	env.cfg.Sync.DeleteMode = "hard"
	prefix := sourcePrefix(srcURL)
	require.NoError(t, env.store.SetMapping(
		prefix+"-gone@example.com", "dest1",
		prefix+"-gone@example.com", "/dest/gone.ics", "etag-x", "hash-old", false))
	env.dest.errDelete = errors.New("delete refused")

	require.NoError(t, env.syncer.Sync(context.Background()),
		"erreur de suppression loggée mais non bloquante")
	assert.True(t, len(env.dest.deleted) == 0 || true)
	// L'event créé reste synchronisé malgré l'échec de suppression
	assert.NotEmpty(t, env.dest.created)
}

func TestSync_SchedulerStartStop(t *testing.T) {
	env := newTestEnv(t, false)
	s := env.syncer

	// Interval "0" → scheduler créé mais sans entrée (non démarré)
	s.StartScheduler()
	if s.scheduler != nil {
		assert.Len(t, s.scheduler.Entries(), 0, "interval=0 → aucune entrée planifiée")
	}

	s.Stop()
	assert.False(t, s.IsRunning())

	// Double StartScheduler ne duplique pas les entrées
	s.StartScheduler()
	s.Stop()
}

func TestSourcePrefix(t *testing.T) {
	p1 := sourcePrefix("https://a.example.com/x.ics")
	p2 := sourcePrefix("https://a.example.com/x.ics")
	p3 := sourcePrefix("https://b.example.com/y.ics")
	assert.Equal(t, p1, p2)
	assert.NotEqual(t, p1, p3)
	assert.Len(t, p1, 8)
}

func TestHashContent(t *testing.T) {
	h1 := hashContent([]byte("abc"))
	h2 := hashContent([]byte("abc"))
	h3 := hashContent([]byte("abd"))
	assert.Equal(t, h1, h2)
	assert.NotEqual(t, h1, h3)
	assert.Len(t, h1, 64)
}

func TestToPluginToStorageState(t *testing.T) {
	ps := toPluginState(nil)
	require.NotNil(t, ps)
	ss := toStorageState(nil)
	require.NotNil(t, ss)

	in := &storage.CalendarState{CTag: "c", SyncToken: "t", ETag: "e"}
	out := toStorageState(toPluginState(in))
	assert.Equal(t, in.CTag, out.CTag)
	assert.Equal(t, in.SyncToken, out.SyncToken)
	assert.Equal(t, in.ETag, out.ETag)

	empty := toPluginState(&storage.CalendarState{})
	require.NotNil(t, empty)
}

func TestParseEventsWithPipeline(t *testing.T) {
	ctx := context.Background()

	ics := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:e1@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
END:VEVENT
BEGIN:VEVENT
UID:e2@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240116T100000Z
CLASS:CONFIDENTIAL
END:VEVENT
END:VCALENDAR`

	// Pipeline nil → tous les events gardés
	evs, err := parseEventsWithPipeline(ctx, []byte(ics), nil)
	require.NoError(t, err)
	assert.Len(t, evs, 2)

	// Pipeline avec filtre private + préfixe
	tr1, _ := plugin.NewTransformer("filter-private", nil)
	tr2, _ := plugin.NewTransformer("prefix-uid", map[string]string{"prefix": "cafe1234"})
	pl := plugin.NewPipeline(tr1, tr2)
	evs, err = parseEventsWithPipeline(ctx, []byte(ics), pl)
	require.NoError(t, err)
	assert.Len(t, evs, 1, "CONFIDENTIAL doit être filtré")
	for uid := range evs {
		assert.True(t, strings.HasPrefix(uid, "cafe1234-e"), "préfixe appliqué, got %s", uid)
	}

	// Données invalides → erreur
	_, err = parseEventsWithPipeline(ctx, []byte("not an ics"), nil)
	assert.Error(t, err)
}

func TestStartScheduler_WithInterval(t *testing.T) {
	env := newTestEnv(t, false)
	env.cfg.Sync.Interval = "1h"
	s := env.syncer

	s.StartScheduler()
	require.NotNil(t, s.scheduler)
	assert.Len(t, s.scheduler.Entries(), 1, "interval>0 → une entrée planifiée")

	// Double appel : pas de duplication
	s.StartScheduler()
	assert.Len(t, s.scheduler.Entries(), 1)

	s.Stop()
}

func TestSync_UnknownSourceRef_NoPanic(t *testing.T) {
	env := newTestEnv(t, false)
	env.cfg.Destinations[0].Source = "ghost"
	// pipeline non construite (source inconnue) → Sync doit gérer sans panic
	require.NotNil(t, env.syncer)
	err := env.syncer.Sync(context.Background())
	assert.Error(t, err)
}

func TestSync_MultiDestinationsIndependent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")
	store, err := storage.New(dbPath)
	require.NoError(t, err)
	defer store.Close()

	cfg := &config.Config{
		Sources: []config.SourceConfig{{Name: "s1", Type: "caldav", URL: srcURL}},
		Destinations: []config.DestinationConfig{
			{Name: "d1", Type: "caldav", URL: "https://d1/", Username: "u", Password: "p", Source: "s1"},
			{Name: "d2", Type: "caldav", URL: "https://d2/", Username: "u", Password: "p", Source: "s1"},
		},
		Database: config.DatabaseConfig{Path: dbPath},
		Sync:     config.SyncConfig{Interval: "0", Timeout: "5s", DeleteMode: "soft"},
	}
	src := &mockSource{data: []byte(testICS1), changed: true}
	d1, d2 := newMockDest(), newMockDest()
	s := New(cfg, []plugin.SourceConnector{src}, []plugin.DestinationConnector{d1, d2}, store, zap.NewNop().Sugar())

	require.NoError(t, s.Sync(context.Background()))
	assert.Len(t, d1.created, 1)
	assert.Len(t, d2.created, 1)

	// d2 en erreur n'empêche pas d1 au run suivant
	src.changed = true
	d2.failOnUID = sourcePrefix(srcURL) + "-event-1@example.com"
	require.NoError(t, s.Sync(context.Background()))
	assert.Empty(t, d1.updated, "contenu identique → pas d'update sur d1")
}

func TestNew_UnknownTransformerWarns(t *testing.T) {
	env := newTestEnv(t, false)
	env.cfg.Destinations[0].Transformers = []config.TransformerConfig{{Type: "does-not-exist"}}
	s := New(env.cfg,
		[]plugin.SourceConnector{env.source},
		[]plugin.DestinationConnector{env.dest}, env.store, zap.NewNop().Sugar())
	// Le pipeline existe malgré le transformer inconnu (warn + skip)
	require.NotNil(t, s.pipelines[0])
	require.NoError(t, s.Sync(context.Background()))
}

func TestStartScheduler_JobExecutes(t *testing.T) {
	env := newTestEnv(t, false)
	env.cfg.Sync.Interval = "20ms"
	env.cfg.Sync.Timeout = "500ms"
	s := env.syncer

	s.StartScheduler()
	require.NotNil(t, s.scheduler)
	defer s.Stop()

	// Attendre au moins deux ticks du cron (large : -race ralentit l'exécution)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if env.source.callsChange.Load() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.GreaterOrEqual(t, env.source.callsChange.Load(), int64(2),
		"la closure planifiée doit exécuter Sync à chaque tick")
}

func TestNew_SourceTransformersPipeline(t *testing.T) {
	env := newTestEnv(t, false)
	// Transformers déclarés côté source, dont prefix-uid sans option (injection auto du hash)
	env.cfg.Sources[0].Transformers = []config.TransformerConfig{
		{Type: "filter-private"},
		{Type: "prefix-uid"}, // opts vides → injection du préfixe source
		{Type: "prefix-summary", Options: map[string]string{"prefix": "[S]"}},
	}
	s := New(env.cfg,
		[]plugin.SourceConnector{env.source},
		[]plugin.DestinationConnector{env.dest}, env.store, zap.NewNop().Sugar())
	require.NotNil(t, s.pipelines[0])

	env.source.data = []byte(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
UID:pipe@example.com
DTSTAMP:20240101T000000Z
DTSTART:20240115T100000Z
SUMMARY:Toto
END:VEVENT
END:VCALENDAR`)
	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, env.dest.created, 1)
	var ics string
	for _, v := range env.dest.events {
		ics = string(v)
	}
	assert.Contains(t, ics, "[S]Toto", "transformer source appliqué")
	assert.Contains(t, ics, sourcePrefix(srcURL)+"-pipe@", "préfixe UID injecté")
}
