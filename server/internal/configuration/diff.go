package configuration

import (
	"encoding/json"
	"fmt"
	"sort"
)

const (
	ChangeAdded     = "added"
	ChangeRemoved   = "removed"
	ChangeChanged   = "changed"
	ChangeUnchanged = "unchanged"
)

// EntityDiff describes one entity between two versions (SH-054). Unchanged
// entities are included with ChangeUnchanged so the UI can render "No
// changes" instead of omitting them. Secret references diff by name only —
// values never exist in the model, so they can never appear here.
type EntityDiff struct {
	Kind   string        `json:"kind"` // service | accessory | role | env | secret
	Name   string        `json:"name"`
	Change string        `json:"change"`
	Fields []FieldChange `json:"fields,omitempty"`
}

type FieldChange struct {
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// Diff structurally compares two desired states, grouped by entity and sorted
// by kind then name.
func Diff(before, after DesiredState) []EntityDiff {
	var result []EntityDiff
	result = append(result, diffEntities("service", asDocuments(before.Services), asDocuments(after.Services))...)
	result = append(result, diffEntities("accessory", asDocuments(before.Accessories), asDocuments(after.Accessories))...)
	result = append(result, diffEntities("role", asDocuments(before.Roles), asDocuments(after.Roles))...)
	result = append(result, diffFlat("env", before.Env, after.Env)...)
	result = append(result, diffNames("secret", before.SecretRefs, after.SecretRefs)...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Name < result[j].Name
	})
	return result
}

// asDocuments flattens typed specs into field -> rendered value maps via
// JSON, so the field-level diff needs no per-type comparison code.
func asDocuments[V any](entities map[string]V) map[string]map[string]string {
	documents := make(map[string]map[string]string, len(entities))
	for name, entity := range entities {
		documents[name] = toDocument(entity)
	}
	return documents
}

func toDocument(entity any) map[string]string {
	encoded, err := json.Marshal(entity)
	if err != nil {
		return map[string]string{"value": fmt.Sprintf("%v", entity)}
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return map[string]string{"value": string(encoded)}
	}
	fields, isObject := generic.(map[string]any)
	if !isObject {
		return map[string]string{"value": renderValue(generic)}
	}
	document := make(map[string]string, len(fields))
	for field, value := range fields {
		document[field] = renderValue(value)
	}
	return document
}

func renderValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func diffEntities(kind string, before, after map[string]map[string]string) []EntityDiff {
	var result []EntityDiff
	for _, name := range unionKeys(before, after) {
		beforeDocument, existedBefore := before[name]
		afterDocument, existsAfter := after[name]
		switch {
		case !existedBefore:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeAdded})
		case !existsAfter:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeRemoved})
		default:
			fields := diffFields(beforeDocument, afterDocument)
			change := ChangeUnchanged
			if len(fields) > 0 {
				change = ChangeChanged
			}
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: change, Fields: fields})
		}
	}
	return result
}

func diffFields(before, after map[string]string) []FieldChange {
	var fields []FieldChange
	for _, field := range unionKeys(before, after) {
		if before[field] != after[field] {
			fields = append(fields, FieldChange{Field: field, From: before[field], To: after[field]})
		}
	}
	return fields
}

func diffFlat(kind string, before, after map[string]string) []EntityDiff {
	var result []EntityDiff
	for _, name := range unionKeys(before, after) {
		beforeValue, existedBefore := before[name]
		afterValue, existsAfter := after[name]
		switch {
		case !existedBefore:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeAdded})
		case !existsAfter:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeRemoved})
		case beforeValue != afterValue:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeChanged,
				Fields: []FieldChange{{Field: "value", From: beforeValue, To: afterValue}}})
		default:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeUnchanged})
		}
	}
	return result
}

// diffNames reports membership changes only — used for secret references,
// which never carry values.
func diffNames(kind string, before, after []string) []EntityDiff {
	beforeSet := toSet(before)
	afterSet := toSet(after)
	var result []EntityDiff
	for _, name := range unionKeys(beforeSet, afterSet) {
		switch {
		case !beforeSet[name]:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeAdded})
		case !afterSet[name]:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeRemoved})
		default:
			result = append(result, EntityDiff{Kind: kind, Name: name, Change: ChangeUnchanged})
		}
	}
	return result
}

func toSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

func unionKeys[V any](first, second map[string]V) []string {
	seen := map[string]bool{}
	for key := range first {
		seen[key] = true
	}
	for key := range second {
		seen[key] = true
	}
	return sortedKeys(seen)
}
