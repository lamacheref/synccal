package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// envPrefix est le préfixe des variables d'environnement de surcharge.
const envPrefix = "SYNCCAL"

// applyEnvOverrides écrase les valeurs de la configuration avec les variables
// d'environnement SYNCCAL_* correspondantes.
//
// La clé est le chemin du champ dans la structure : majuscules, séparateur `_`.
// Exemples :
//
//	SYNCCAL_WEB_TOKEN=secret              -> web.token
//	SYNCCAL_SYNC_INTERVAL=30m             -> sync.interval
//	SYNCCAL_DATABASE_PATH=/data/db.sqlite -> database.path
//	SYNCCAL_DESTINATIONS_0_PASSWORD=token -> destinations[0].password
//
// Les valeurs vides sont ignorées (une variable définie mais vide ne masque
// pas la valeur du fichier). Les champs de type map ne sont pas surchargeables.
func applyEnvOverrides(cfg *Config) error {
	return applyEnvRecursive(reflect.ValueOf(cfg).Elem(), envPrefix)
}

func applyEnvRecursive(v reflect.Value, prefix string) error {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("type non supporté pour %s: %s", prefix, v.Kind())
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		key := envKeyFor(field)
		if key == "" {
			continue
		}
		full := prefix + "_" + key

		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.Struct:
			if err := applyEnvRecursive(fv, full); err != nil {
				return err
			}
		case reflect.Slice:
			if err := applyEnvSlice(fv, full); err != nil {
				return err
			}
		case reflect.Map:
			// options libres (transformer.options) : pas de surcharge env
		default:
			val, ok := os.LookupEnv(full)
			if !ok || val == "" {
				continue
			}
			if err := setScalar(fv, val); err != nil {
				return fmt.Errorf("%s: %w", full, err)
			}
		}
	}
	return nil
}

// applyEnvSibling permet la surcharge des éléments existants d'une liste :
// SYNCCAL_DESTINATIONS_0_PASSWORD écrase destinations[0].password si l'entrée
// existe déjà dans le fichier (créer des entrées se fait via le YAML/UI).
func applyEnvSlice(v reflect.Value, prefix string) error {
	if v.Type().Elem().Kind() != reflect.Struct {
		return nil // listes de scalaires : non géré
	}
	for i := 0; i < v.Len(); i++ {
		if err := applyEnvRecursive(v.Index(i), prefix+"_"+strconv.Itoa(i)); err != nil {
			return err
		}
	}
	return nil
}

// envKeyFor déduit le nom de variable à partir du tag mapstructure/yaml.
func envKeyFor(field reflect.StructField) string {
	tag := field.Tag.Get("mapstructure")
	if tag == "" {
		tag = field.Tag.Get("yaml")
	}
	name := strings.Split(tag, ",")[0]
	if name == "" || name == "-" {
		return ""
	}
	return strings.ToUpper(name)
}

// setScalar convertit une chaîne d'environnement vers le type du champ.
func setScalar(field reflect.Value, val string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(val)
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("bool invalide %q", val)
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return fmt.Errorf("entier invalide %q", val)
		}
		field.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return fmt.Errorf("entier non signé invalide %q", val)
		}
		field.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("flottant invalide %q", val)
		}
		field.SetFloat(f)
	default:
		return fmt.Errorf("type non surchargeable: %s", field.Kind())
	}
	return nil
}

// loadFromEnv retourne la configuration inline fournie via SYNCCAL_CONFIG
// (contenu YAML complet), utilisée à la place du fichier si définie.
func loadFromEnv() ([]byte, bool) {
	if val := os.Getenv("SYNCCAL_CONFIG"); val != "" {
		return []byte(val), true
	}
	return nil, false
}
