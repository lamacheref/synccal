package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// envPrefix est le préfixe des variables d'environnement de configuration.
const envPrefix = "SYNCCAL"

// envConfigVar désigne le nom de la variable contenant le YAML inline complet.
const envConfigVar = envPrefix + "_CONFIG"

// buildEnvTree transforme les variables SYNCCAL_* en arbre YAML natif
// (maps Go et listes []any).
//
// Chaque segment (séparé par `_`) est une clé de map ; un segment numérique
// est un index de liste. Exemples :
//
//	SYNCCAL_WEB_TOKEN=secret                            -> web.token
//	SYNCCAL_SYNC_INTERVAL=30m                           -> sync.interval
//	SYNCCAL_DESTINATIONS_0_PASSWORD=token               -> destinations[0].password
//	SYNCCAL_DESTINATIONS_0_TRANSFORMERS_0_TYPE=f        -> destinations[0].transformers[0].type
//	SYNCCAL_DESTINATIONS_0_TRANSFORMERS_0_OPTIONS_PREFIX=x -> ...options.prefix
//
// Les valeurs vides sont ignorées.
func buildEnvTree() (map[string]any, error) {
	tree := make(map[string]any)
	names := make([]string, 0)
	for _, kv := range os.Environ() {
		names = append(names, strings.SplitN(kv, "=", 2)[0])
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasPrefix(name, envPrefix+"_") || name == envConfigVar {
			continue
		}
		val := os.Getenv(name)
		if val == "" {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(name, envPrefix+"_"), "_")
		if err := insertPath(tree, segs, val); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return tree, nil
}

// insertPath insère une valeur au chemin donné. Quand le segment suivant est
// numérique, la clé courante devient une liste (élément créé si nécessaire).
func insertPath(node map[string]any, segs []string, val string) error {
	seg := segs[0]
	if len(segs) == 1 {
		if _, err := strconv.Atoi(seg); err != nil {
			seg = strings.ToLower(seg)
		}
		node[seg] = val
		return nil
	}
	if idx, err := strconv.Atoi(segs[1]); err == nil {
		seg = strings.ToLower(seg)
		list, ok := node[seg].([]any)
		if !ok {
			list = make([]any, 0, idx+1)
		}
		for len(list) <= idx {
			list = append(list, map[string]any{})
		}
		child := list[idx].(map[string]any)
		if err := insertPath(child, segs[2:], val); err != nil {
			return err
		}
		node[seg] = list
		return nil
	}
	seg = strings.ToLower(seg)
	child, ok := node[seg].(map[string]any)
	if !ok {
		child = make(map[string]any)
		node[seg] = child
	}
	return insertPath(child, segs[1:], val)
}

// hasSynccalEnv indique si au moins une variable SYNCCAL_* (hors CONFIG) existe.
func hasSynccalEnv() bool {
	for _, kv := range os.Environ() {
		name := strings.SplitN(kv, "=", 2)[0]
		if strings.HasPrefix(name, envPrefix+"_") && name != envConfigVar && os.Getenv(name) != "" {
			return true
		}
	}
	return false
}

// loadFromEnv retourne le YAML inline fourni via SYNCCAL_CONFIG.
func loadFromEnv() ([]byte, bool) {
	if val := os.Getenv(envConfigVar); val != "" {
		return []byte(val), true
	}
	return nil, false
}

// deepMerge fusionne src dans dst : maps récursivement, listes par index,
// scalaires écrasés.
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		switch sv := sv.(type) {
		case map[string]any:
			existing, ok := dst[k].(map[string]any)
			if !ok {
				existing = make(map[string]any)
				dst[k] = existing
			}
			deepMerge(existing, sv)
		case []any:
			existing, ok := dst[k].([]any)
			if !ok {
				existing = nil
			}
			dst[k] = mergeLists(existing, sv)
		default:
			dst[k] = sv
		}
	}
}

// mergeLists étend dst avec les éléments de src (par index), en fusionnant
// récursivement les maps qui se recouvrent.
func mergeLists(dst, src []any) []any {
	out := make([]any, 0, max(len(dst), len(src)))
	out = append(out, dst...)
	for i, sv := range src {
		if i < len(out) {
			if dm, ok := out[i].(map[string]any); ok {
				if sm, sok := sv.(map[string]any); sok {
					deepMerge(dm, sm)
					continue
				}
			}
			out[i] = sv
			continue
		}
		out = append(out, sv)
	}
	return out
}
