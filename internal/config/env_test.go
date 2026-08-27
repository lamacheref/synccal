package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildEnvTree(t *testing.T) {
	t.Setenv("SYNCCAL_WEB_TOKEN", "t1")
	t.Setenv("SYNCCAL_SYNC_INTERVAL", "30m")
	t.Setenv("SYNCCAL_DESTINATIONS_0_PASSWORD", "p0")
	t.Setenv("SYNCCAL_DESTINATIONS_1_USERNAME", "u1")
	t.Setenv("SYNCCAL_DESTINATIONS_1_TRANSFORMERS_0_TYPE", "filter-private")
	t.Setenv("SYNCCAL_DESTINATIONS_1_TRANSFORMERS_0_OPTIONS_PREFIX", "foo")
	t.Setenv("SYNCCAL_EMPTY_IGNORED", "") // ignorée
	t.Setenv("SYNCCAL_CONFIG", "ignored") // exclue (réservée au YAML inline)

	tree, err := buildEnvTree()
	require.NoError(t, err)

	web := tree["web"].(map[string]any)
	assert.Equal(t, "t1", web["token"])

	sync := tree["sync"].(map[string]any)
	assert.Equal(t, "30m", sync["interval"])

	_, ok := tree["empty_ignored"]
	assert.False(t, ok, "valeur vide ignorée")
	_, ok = tree["config"]
	assert.False(t, ok, "SYNCCAL_CONFIG exclue de l'arbre")
}

func TestLoad_PureEnvConfig_NoFile(t *testing.T) {
	// Aucun fichier : la config est entièrement construite depuis les variables
	t.Setenv("SYNCCAL_SOURCES_0_NAME", "env-src")
	t.Setenv("SYNCCAL_SOURCES_0_TYPE", "caldav")
	t.Setenv("SYNCCAL_SOURCES_0_URL", "https://env.example.com/cal.ics")
	t.Setenv("SYNCCAL_DESTINATIONS_0_NAME", "env-dest")
	t.Setenv("SYNCCAL_DESTINATIONS_0_TYPE", "caldav")
	t.Setenv("SYNCCAL_DESTINATIONS_0_URL", "https://env-dest.example.com/dav/")
	t.Setenv("SYNCCAL_DESTINATIONS_0_USERNAME", "user")
	t.Setenv("SYNCCAL_DESTINATIONS_0_PASSWORD", "token")
	t.Setenv("SYNCCAL_DESTINATIONS_0_SOURCE", "env-src")
	t.Setenv("SYNCCAL_DATABASE_PATH", "/data/db.sqlite")
	t.Setenv("SYNCCAL_WEB_TOKEN", "ui-token")
	t.Setenv("SYNCCAL_SYNC_INTERVAL", "30m")

	cfg, err := Load("/does/not/exist.yaml")
	require.NoError(t, err)
	require.Len(t, cfg.Sources, 1)
	assert.Equal(t, "env-src", cfg.Sources[0].Name)
	assert.Equal(t, "caldav", cfg.Sources[0].Type)
	require.Len(t, cfg.Destinations, 1)
	assert.Equal(t, "env-dest", cfg.Destinations[0].Name)
	assert.Equal(t, "env-src", cfg.Destinations[0].Source)
	assert.Equal(t, "token", cfg.Destinations[0].Password)
	assert.Equal(t, "/data/db.sqlite", cfg.Database.Path)
	assert.Equal(t, "ui-token", cfg.Web.Token)
	assert.Equal(t, "30m", cfg.Sync.Interval)
}

func TestLoad_EnvAddsDestinationToFile(t *testing.T) {
	yaml := `
sources:
  - name: src1
    type: caldav
    url: https://example.com/cal.ics
destinations:
  - name: dest1
    type: caldav
    url: https://dest.example.com/
    username: user
    password: from-file
    source: src1
database:
  path: /tmp/db.sqlite
`
	path := writeTempYAML(t, yaml)

	// Surcharge d'un champ existant + ajout d'une seconde destination
	t.Setenv("SYNCCAL_DESTINATIONS_0_PASSWORD", "overridden")
	t.Setenv("SYNCCAL_DESTINATIONS_1_NAME", "dest2")
	t.Setenv("SYNCCAL_DESTINATIONS_1_TYPE", "caldav")
	t.Setenv("SYNCCAL_DESTINATIONS_1_URL", "https://dest2.example.com/")
	t.Setenv("SYNCCAL_DESTINATIONS_1_USERNAME", "u2")
	t.Setenv("SYNCCAL_DESTINATIONS_1_PASSWORD", "p2")
	t.Setenv("SYNCCAL_DESTINATIONS_1_SOURCE", "src1")

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Destinations, 2)
	assert.Equal(t, "overridden", cfg.Destinations[0].Password)
	assert.Equal(t, "dest2", cfg.Destinations[1].Name)
	assert.Equal(t, "src1", cfg.Destinations[1].Source)
}

func TestLoad_EnvTransformerOptions(t *testing.T) {
	yaml := `
sources:
  - name: src1
    type: caldav
    url: https://example.com/cal.ics
destinations:
  - name: dest1
    type: caldav
    url: https://dest.example.com/
    username: u
    password: p
    source: src1
database:
  path: /tmp/db.sqlite
`
	path := writeTempYAML(t, yaml)
	t.Setenv("SYNCCAL_DESTINATIONS_0_TRANSFORMERS_0_TYPE", "prefix-summary")
	t.Setenv("SYNCCAL_DESTINATIONS_0_TRANSFORMERS_0_OPTIONS_PREFIX", "[S] ")

	cfg, err := Load(path)
	require.NoError(t, err)
	require.Len(t, cfg.Destinations[0].Transformers, 1)
	assert.Equal(t, "prefix-summary", cfg.Destinations[0].Transformers[0].Type)
	assert.Equal(t, "[S] ", cfg.Destinations[0].Transformers[0].Options["prefix"])
}

func TestLoad_InvalidEnvValue(t *testing.T) {
	yaml := `
sources:
  - name: src1
    type: caldav
    url: https://example.com/cal.ics
destinations:
  - name: dest1
    type: caldav
    url: https://dest.example.com/
    username: u
    password: p
    source: src1
database:
  path: /tmp/db.sqlite
sync:
  batch_size: 100
`
	path := writeTempYAML(t, yaml)
	t.Setenv("SYNCCAL_SYNC_BATCH_SIZE", "pas-un-nombre")

	_, err := Load(path)
	require.Error(t, err, "valeur env invalide pour un champ numérique")
}

func TestLoad_FromSynccalConfigEnv(t *testing.T) {
	inline := `
sources:
  - name: env-src
    type: caldav
    url: https://env.example.com/cal.ics
destinations:
  - name: env-dest
    type: caldav
    url: https://env-dest.example.com/
    username: u
    password: p
    source: env-src
database:
  path: /data/db.sqlite
`
	t.Setenv("SYNCCAL_CONFIG", inline)
	// Combiné avec une surcharge ponctuelle
	t.Setenv("SYNCCAL_WEB_TOKEN", "inline-and-env")

	cfg, err := Load("/does/not/exist.yaml")
	require.NoError(t, err)
	assert.Equal(t, "env-src", cfg.Sources[0].Name)
	assert.Equal(t, "env-dest", cfg.Destinations[0].Name)
	assert.Equal(t, "inline-and-env", cfg.Web.Token)
}

func TestLoad_NoFileNoEnvFails(t *testing.T) {
	// Pas de fichier, pas de variable SYNCCAL_* → erreur remontée
	_, err := Load("/does/not/exist.yaml")
	assert.Error(t, err)
}

func TestHasSynccalEnv(t *testing.T) {
	// Nettoyer toutes les variables SYNCCAL_* du process de test
	var names []string
	for _, kv := range tEnvSnapshot() {
		names = append(names, kv)
	}
	for _, n := range names {
		if len(n) >= len(envPrefix)+1 && n[:len(envPrefix)+1] == envPrefix+"_" {
			t.Setenv(n, "")
		}
	}
	assert.False(t, hasSynccalEnv())

	t.Setenv("SYNCCAL_WEB_TOKEN", "x")
	assert.True(t, hasSynccalEnv())
}

// tEnvSnapshot capture les variables d'environnement (helper simple pour tests).
func tEnvSnapshot() []string {
	var out []string
	for _, kv := range os.Environ() {
		out = append(out, strings.SplitN(kv, "=", 2)[0])
	}
	return out
}
