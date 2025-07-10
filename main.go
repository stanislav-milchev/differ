package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/r3labs/diff/v3"
)

type ChangeType string

const (
	Added     ChangeType = "added"
	Removed   ChangeType = "removed"
	Changed   ChangeType = "changed"
	Unchanged ChangeType = "unchanged"
)

type DiffMap map[string]ChangeType

type DiffResult struct {
	Path string
	Type string
	From string
	To   string
}

func main() {
	var outputFile string
	flag.StringVar(&outputFile, "o", "diff.html", "Output HTML file")
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Println("Usage: jsondiff file1.json file2.json [-o output.html]")
		os.Exit(1)
	}

	file1, file2 := flag.Arg(0), flag.Arg(1)
	json1 := readJSONInterface(file1)
	json2 := readJSONInterface(file2)

	changes, err := diff.Diff(json1, json2)
	if err != nil {
		log.Fatalf("Failed to diff: %v", err)
	}

	diffMap := buildDiffMap(changes)
	diffTable := buildDiffTable(changes)

	json1Sorted := sortJSON(json1)
	json2Sorted := sortJSON(json2)

	f, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	// Load template from external file instead of embedded string
	tpl := template.Must(template.New("diff").Funcs(template.FuncMap{
		"renderJSON": func(v interface{}, path string) template.HTML {
			return renderJSON(v, path, diffMap)
		},
	}).ParseFiles("template.html"))

	err = tpl.ExecuteTemplate(f, "template.html", map[string]interface{}{
		"Original": json1Sorted,
		"Modified": json2Sorted,
		"Diffs":    diffTable,
	})

	if err != nil {
		log.Fatalf("Failed to write HTML: %v", err)
	}

	fmt.Printf("Diff written to %s\n", outputFile)
}

func readJSONInterface(filename string) interface{} {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", filename, err)
	}

	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		log.Fatalf("Invalid JSON in %s: %v", filename, err)
	}
	return parsed
}

func buildDiffMap(changes []diff.Change) DiffMap {
	m := make(DiffMap)
	for _, c := range changes {
		p := strings.Join(c.Path, ".")
		var ct ChangeType
		switch c.Type {
		case "create":
			ct = Added
		case "delete":
			ct = Removed
		case "update":
			ct = Changed
		default:
			ct = Unchanged
		}
		m[p] = ct
	}
	return m
}

func buildDiffTable(changes []diff.Change) []DiffResult {
	results := make([]DiffResult, 0, len(changes))
	for _, c := range changes {
		results = append(results, DiffResult{
			Path: strings.Join(c.Path, "."),
			Type: c.Type,
			From: fmt.Sprintf("%v", c.From),
			To:   fmt.Sprintf("%v", c.To),
		})
	}
	return results
}

func renderJSON(v interface{}, path string, diffMap DiffMap) template.HTML {
	switch val := v.(type) {
	case map[string]interface{}:
		var sb strings.Builder
		sb.WriteString(`<div class="json-object">{`)
		sb.WriteString(`<ul class="json-list">`)
		keys := sortedKeys(val)
		for i, k := range keys {
			vv := val[k]
			p := pathKey(path, k)
			changeType := getChangeType(diffMap, p)

			sb.WriteString(fmt.Sprintf(`<li class="json-key %s">`, changeType))
			sb.WriteString(`<span class="key">"` + escapeHTML(k) + `"</span>: `)
			sb.WriteString(string(renderJSON(vv, p, diffMap)))
			if i < len(keys)-1 {
				sb.WriteString(",")
			}
			sb.WriteString("</li>")
		}
		sb.WriteString("</ul>}")
		sb.WriteString("</div>")
		return template.HTML(sb.String())

	case []interface{}:
		var sb strings.Builder
		sb.WriteString(`<div class="json-array">[`)
		sb.WriteString(`<ul class="json-list">`)
		for i, vv := range val {
			p := pathKey(path, fmt.Sprintf("%d", i))
			changeType := getChangeType(diffMap, p)
			sb.WriteString(fmt.Sprintf(`<li class="json-key %s">`, changeType))
			sb.WriteString(string(renderJSON(vv, p, diffMap)))
			if i < len(val)-1 {
				sb.WriteString(",")
			}
			sb.WriteString("</li>")
		}
		sb.WriteString("</ul>]")
		sb.WriteString("</div>")
		return template.HTML(sb.String())

	case string:
		return template.HTML(`<span class="json-string">"` + escapeHTML(val) + `"</span>`)

	case float64:
		return template.HTML(fmt.Sprintf(`<span class="json-number">%v</span>`, val))

	case bool:
		return template.HTML(fmt.Sprintf(`<span class="json-bool">%v</span>`, val))

	case nil:
		return template.HTML(`<span class="json-null">null</span>`)

	default:
		return template.HTML(escapeHTML(fmt.Sprintf("%v", val)))
	}
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&#39;",
	)
	return r.Replace(s)
}

// Modify pathKey to unify path style for arrays and objects:
func pathKey(base, key string) string {
	if base == "" {
		return key
	}
	// if key is numeric index (array), append with dot instead of brackets
	if _, err := strconv.Atoi(key); err == nil {
		return base + "." + key
	}
	return base + "." + key
}

func getChangeType(diffMap DiffMap, path string) string {
	if v, ok := diffMap[path]; ok {
		return string(v)
	}
	return string(Unchanged)
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		sorted := make(map[string]interface{}, len(val))
		keys := sortedKeys(val)
		for _, k := range keys {
			sorted[k] = sortJSON(val[k])
		}
		return sorted
	case []interface{}:
		for i := range val {
			val[i] = sortJSON(val[i])
		}
		return val
	default:
		return v
	}
}
