package ingestion

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	"go.yaml.in/yaml/v3"
)

// File is an ingestion as a definition file, the form `fcdev ingest apply`
// reads and `export` writes: the fields the API takes to create one, without
// what the platform adds (id, versions, times, the set that owns it).
// Application is a code, Client an id or identifier. Active is nil when the
// file does not say: an apply then creates the ingestion paused when its source
// names a config entry (its settings and secrets are set after the apply) and
// active otherwise, and leaves the state of one that exists as it is.
type File struct {
	Application string
	Client      string
	Code        string
	Active      *bool
	Definition
}

// UnmarshalJSON reads the JSON form of a file strictly, as the API form of the
// same fields.
func (f *File) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("a definition is an object")
	}
	*f = File{}
	for key, dst := range map[string]any{"application": &f.Application, "client": &f.Client, "code": &f.Code, "active": &f.Active} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		delete(fields, key)
	}
	rest, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(rest, &f.Definition)
}

// MarshalJSON writes the JSON form UnmarshalJSON reads: the identity beside the
// definition's own fields.
func (f File) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(f.Definition)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	identity := map[string]any{"application": f.Application, "client": f.Client, "code": f.Code}
	if f.Active != nil {
		identity["active"] = *f.Active
	}
	for key, value := range identity {
		if fields[key], err = json.Marshal(value); err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

// ParseFiles reads every YAML document of a definition file. A document that
// lists clients (clients: [...]) in place of client gives one definition per
// client (expandClients). An extension's
// source is written flat, its type as the kind and its settings beside it
// (kind: camel-route, yaml: ...); a built-in kind or kind: custom is read as
// it is. Unknown fields are errors.
func ParseFiles(data []byte) ([]File, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var files []File
	for n := 1; ; n++ {
		var doc any
		if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
			return files, nil
		} else if err != nil {
			return nil, fmt.Errorf("document %d: %w", n, err)
		}
		if doc == nil {
			continue // an empty document, e.g. a leading ---
		}
		docs := []any{doc}
		if m, ok := doc.(map[string]any); ok {
			perClient, err := expandClients(m)
			if err != nil {
				return nil, fmt.Errorf("document %d: %w", n, err)
			}
			docs = docs[:0]
			for _, d := range perClient {
				unflattenSource(d)
				docs = append(docs, d)
			}
		}
		for _, d := range docs {
			raw, err := json.Marshal(d)
			if err != nil {
				return nil, fmt.Errorf("document %d: %w", n, err)
			}
			var f File
			if err := json.Unmarshal(raw, &f); err != nil {
				return nil, fmt.Errorf("document %d: %w", n, err)
			}
			files = append(files, f)
		}
	}
}

// AllClients, as a definition's client (written clients: "*" in a file),
// stands for every client there is when the set is applied: the apply makes
// one ingestion per client, whatever its status (a suspended or inactive
// client's ingestions take no jobs and run on no runtime, so its ingestion is
// kept, not deleted, while it is not active). A client made after the apply
// gets its ingestion at the next apply.
const AllClients = "*"

// expandClients reads a document that names its clients as a list
// (clients: [acme, globex]) as one document per client, each with that client
// and everything else the same, in the order listed. A document with client
// is returned as it is, and clients: "*" as one document whose client is
// AllClients, which the apply expands. Each client's ingestion is its own: its settings and
// secrets are that client's, it keeps its own place in its source, and a set
// apply deletes it once its client is no longer listed.
func expandClients(doc map[string]any) ([]map[string]any, error) {
	raw, listed := doc["clients"]
	if !listed {
		return []map[string]any{doc}, nil
	}
	if _, both := doc["client"]; both {
		return nil, errors.New("client and clients: give one or the other")
	}
	if raw == AllClients {
		one := maps.Clone(doc)
		delete(one, "clients")
		one["client"] = AllClients
		return []map[string]any{one}, nil
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, errors.New(`clients: a list of client ids or identifiers, at least one, or "*" for every client`)
	}
	seen := map[string]bool{}
	out := make([]map[string]any, 0, len(list))
	for i, item := range list {
		ref, ok := item.(string)
		if !ok || strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("clients[%d]: a client id or identifier", i)
		}
		if strings.TrimSpace(ref) == AllClients {
			return nil, fmt.Errorf(`clients[%d]: "*" is every client, written alone (clients: "*"), not in a list`, i)
		}
		key := strings.ToLower(strings.TrimSpace(ref))
		if seen[key] {
			return nil, fmt.Errorf("clients[%d]: %s is listed twice", i, ref)
		}
		seen[key] = true
		one := maps.Clone(doc)
		delete(one, "clients")
		one["client"] = ref
		out = append(out, one)
	}
	return out, nil
}

// unflattenSource turns a flat extension source into {kind: custom, type,
// settings}, or {kind: custom, type, config} when it names a config entry
// (anything else written beside config is kept where it is, for Validate to
// refuse).
func unflattenSource(doc map[string]any) {
	src, ok := doc["source"].(map[string]any)
	if !ok {
		return
	}
	kind, ok := src["kind"].(string)
	if !ok || IsBuiltInKind(kind) {
		return
	}
	if _, named := src["config"]; named {
		flat := maps.Clone(src)
		flat["kind"], flat["type"] = KindCustom, kind
		doc["source"] = flat
		return
	}
	settings := make(map[string]any, len(src))
	for k, v := range src {
		if k != "kind" {
			settings[k] = v
		}
	}
	doc["source"] = map[string]any{"kind": KindCustom, "type": kind, "settings": settings}
}

// YAML renders the file: fields in a fixed order, keepFailedPayloads left out
// when off, an extension's source flat. Parsing the output gives the same file.
func (f File) YAML() ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	add := func(key string, value any) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		node, err := jsonNode(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, node)
		return nil
	}
	var source any = f.Source
	switch c := f.Source.(type) {
	case Custom:
		flat, err := marshalTagged(c.Type, c.Settings)
		if err != nil {
			return nil, err
		}
		source = json.RawMessage(flat)
	case Configured:
		if c.Base == KindCustom {
			flat, err := marshalTagged(c.Type, struct {
				Config ConfigRef `json:"config"`
			}{c.Config})
			if err != nil {
				return nil, err
			}
			source = json.RawMessage(flat)
		}
	}
	steps := []struct {
		key   string
		value any
		skip  bool
	}{
		{"application", f.Application, false},
		{"client", f.Client, false},
		{"code", f.Code, false},
		{"name", f.Name, false},
		{"active", f.Active != nil && *f.Active, f.Active == nil},
		{"keepFailedPayloads", true, !f.KeepFailedPayloads},
		{"source", source, false},
		{"split", f.Split, false},
		{"target", f.Target, false},
	}
	for _, s := range steps {
		if s.skip {
			continue
		}
		if err := add(s.key, s.value); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// jsonNode builds a YAML node from JSON, keeping the object key order.
func jsonNode(raw []byte) (*yaml.Node, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return readNode(dec)
}

func readNode(dec *json.Decoder) (*yaml.Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		kind := yaml.SequenceNode
		if t == '{' {
			kind = yaml.MappingNode
		}
		n := &yaml.Node{Kind: kind}
		for dec.More() {
			if kind == yaml.MappingNode {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.(string)})
			}
			child, err := readNode(dec)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, child)
		}
		if _, err := dec.Token(); err != nil { // the closing delimiter
			return nil, err
		}
		return n, nil
	case string:
		n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: t}
		if strings.Contains(t, "\n") {
			n.Style = yaml.LiteralStyle
		}
		return n, nil
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(t.String(), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: t.String()}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(t)}, nil
	default: // null
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
}
