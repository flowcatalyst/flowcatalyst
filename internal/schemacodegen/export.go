package schemacodegen

// Schema export (port of frontend/src/utils/schema-export.ts): a zip of the
// CURRENT JSON Schemas of a set of event types plus generated PHP, TypeScript,
// Python and Java sources.
//
// File structure inside the zip:
//
//	schemas/{application}/{subdomain}/{aggregate}/{event}.json
//	php/{Application}/{Subdomain}/{Aggregate}/{AggregateEventData}.php
//	typescript/{application}/{subdomain}/{aggregate}/{aggregateEventData}.ts
//	python/{application}/{subdomain}/{aggregate}/{aggregate_event_data}.py
//	java/com/{app}/events/{subdomain}/{aggregate}/{AggregateEventData}.java

import (
	"archive/zip"
	"bytes"
	"regexp"
	"time"
)

// ExportEventType is the data ExportZip needs about one event type.
type ExportEventType struct {
	Application, Subdomain, Aggregate, Event string
	// CurrentSchema is the schema string of the CURRENT spec version, or ""
	// when the event type has none (it is then skipped).
	CurrentSchema string
}

// zipEpoch is the fixed modification time of every entry, so the archive is
// deterministic.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// orderedFiles is a JS-object-like map of path -> content: assigning an
// existing path replaces its content but keeps its original position.
type orderedFiles struct {
	paths   []string
	content map[string]string
}

func (f *orderedFiles) set(path, content string) {
	if _, ok := f.content[path]; !ok {
		f.paths = append(f.paths, path)
	}
	f.content[path] = content
}

// ExportZip builds the schema export archive. Entries are written in the
// order the TS assigns them (fflate zipSync keeps object insertion order):
// per event type, in input order, the schema JSON then PHP, TypeScript,
// Python and Java; a path assigned twice keeps its first position with the
// last content. Every entry is deflated with a fixed 1980-01-01 timestamp,
// so output is deterministic.
//
// exported counts event types whose schema parsed (as in the TS), errs holds
// the per-event-type failures with the TS messages, and zip is nil when
// exported is 0 (the TS produces no download in that case). As in the TS, a
// schema that is not valid JSON is still added to the archive verbatim
// (when other event types exported) and reported in errs.
func ExportZip(ets []ExportEventType) (zipBytes []byte, exported int, errs []string, err error) {
	files := &orderedFiles{content: map[string]string{}}

	for _, et := range ets {
		if et.CurrentSchema == "" {
			continue
		}

		eventCode := et.Application + ":" + et.Subdomain + ":" + et.Aggregate + ":" + et.Event

		// JSON Schema
		parsed, perr := Parse(et.CurrentSchema)
		content := et.CurrentSchema // formatSchema falls back to the raw string
		if perr == nil {
			content = Stringify(parsed, "  ")
		}
		files.set("schemas/"+et.Application+"/"+et.Subdomain+"/"+et.Aggregate+"/"+et.Event+".json", content)
		if perr != nil {
			errs = append(errs, eventCode+": invalid schema JSON")
			continue
		}

		exportOne := func(label, path string, gen func(Schema, string) (string, error)) {
			out, gerr := gen(parsed, eventCode)
			if gerr != nil {
				errs = append(errs, eventCode+": "+label+" generation failed")
				return
			}
			files.set(path, out)
		}

		a, s, g, e := et.Application, et.Subdomain, et.Aggregate, et.Event
		exportOne("PHP",
			"php/"+pascalCase(a)+"/"+pascalCase(s)+"/"+pascalCase(g)+"/"+pascalCase(g)+pascalCase(e)+"Data.php", PHP)
		exportOne("TypeScript",
			"typescript/"+a+"/"+s+"/"+g+"/"+camelCase(g)+pascalCase(e)+"Data.ts", TypeScript)
		exportOne("Python",
			"python/"+a+"/"+s+"/"+g+"/"+exportSnakeCase(g)+"_"+exportSnakeCase(e)+"_data.py", Python)
		exportOne("Java",
			"java/com/"+jsLower(a)+"/events/"+jsLower(s)+"/"+jsLower(g)+"/"+pascalCase(g)+pascalCase(e)+"Data.java", Java)

		exported++
	}

	if exported == 0 {
		return nil, 0, errs, nil
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, path := range files.paths {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: scrub(path), Method: zip.Deflate, Modified: zipEpoch})
		if err != nil {
			return nil, exported, errs, err
		}
		if _, err := w.Write([]byte(scrub(files.content[path]))); err != nil {
			return nil, exported, errs, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, exported, errs, err
	}
	return buf.Bytes(), exported, errs, nil
}

var dashes = regexp.MustCompile(`-`)

// exportSnakeCase is schema-export.ts's toSnakeCase: the generators' version
// plus `.replace(/-/g, "_")`.
func exportSnakeCase(s string) string {
	return dashes.ReplaceAllString(toSnakeCase(s), "_")
}
