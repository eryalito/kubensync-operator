package reconciler

import (
	"strings"
	"testing"
	"time"
)

const (
	validJSONDoc = `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"a"}}`
	errDocument1 = "error decoding rendered manifest document 1"
)

// decodeManifestsWithDeadline fails the test instead of hanging it if
// decodeManifests stops making progress.
func decodeManifestsWithDeadline(t *testing.T, manifests string) (int, error) {
	t.Helper()
	type result struct {
		count int
		err   error
	}
	done := make(chan result, 1)
	go func() {
		objs, err := decodeManifests(manifests)
		done <- result{count: len(objs), err: err}
	}()
	select {
	case r := <-done:
		return r.count, r.err
	case <-time.After(10 * time.Second):
		t.Fatal("decodeManifests did not return: the document loop is not making progress")
		return 0, nil
	}
}

func TestDecodeManifests(t *testing.T) {
	tests := []struct {
		name      string
		manifests string
		want      int
		wantErr   string
	}{
		{
			name:      "empty template",
			manifests: "",
		},
		{
			name: "multiple yaml documents",
			manifests: `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: a
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: b
`,
			want: 2,
		},
		{
			name: "empty documents are skipped",
			manifests: `---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: a
---
`,
			want: 1,
		},
		{
			name:      "json stream",
			manifests: validJSONDoc + "\n" + validJSONDoc + "\n",
			want:      2,
		},
		{
			// Flow style starts with '{', so it is handed to the JSON decoder
			// first and only reaches YAML on the fallback.
			name:      "yaml flow style",
			manifests: "{apiVersion: v1, kind: ConfigMap, metadata: {name: a}}\n",
			want:      1,
		},
		{
			name:      "crlf line endings",
			manifests: "---\r\napiVersion: v1\r\nkind: ConfigMap\r\nmetadata:\r\n  name: a\r\n",
			want:      1,
		},
		// A malformed YAML document does not stall the decoder, so these never
		// spun. They used to be logged and skipped, applying the remaining
		// documents and still reporting the resource as ready.
		{
			name:      "yaml document with a bad document before it",
			manifests: "---\n\tbad: yes\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n",
			wantErr:   errDocument1,
		},
		{
			name:      "yaml document with a bad document after it",
			manifests: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n---\n\tbad: yes\n",
			wantErr:   "error decoding rendered manifest document 2",
		},
		// Regression: the decoder latches the syntax error and returns it from
		// every later Decode without consuming input once more than one JSON
		// document has decoded. Asserting the document number pins that the loop
		// aborts on it rather than decoding again until maxTemplateDocuments.
		{
			name:      "unambiguous json stream with a syntax error",
			manifests: validJSONDoc + "\n" + validJSONDoc + "\n{,}\n",
			wantErr:   "error decoding rendered manifest document 3",
		},
		// Regression: with a single decoded document and fewer than 4 trailing
		// bytes the decoder cannot fall back to YAML, and every later Decode
		// returns "decoding failed as both JSON and YAML" without consuming input.
		{
			name:      "json document with a short invalid tail",
			manifests: validJSONDoc + " x",
			wantErr:   "error decoding rendered manifest document 2",
		},
		{
			name:      "invalid short input",
			manifests: "{x",
			wantErr:   errDocument1,
		},
		{
			name:      "invalid yaml",
			manifests: "apiVersion: v1\n\tkind: ConfigMap\n",
			wantErr:   errDocument1,
		},
		{
			name:      "too many documents",
			manifests: strings.Repeat(validJSONDoc+"\n", maxTemplateDocuments+1),
			wantErr:   "holds more than",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := decodeManifestsWithDeadline(t, tt.manifests)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got %d objects", tt.wantErr, count)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != tt.want {
				t.Fatalf("expected %d objects, got %d", tt.want, count)
			}
		})
	}
}

// The returned error must not echo the rendered template, which can hold Secret
// data, because it is persisted in the ManagedResource status.
func TestDecodeManifestsErrorDoesNotEchoInput(t *testing.T) {
	_, err := decodeManifestsWithDeadline(t, `{"apiVersion":"v1","kind":"Secret","stringData":{"token":"s3cr3t-token"}} x`)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "s3cr3t-token") {
		t.Fatalf("error echoes the rendered template: %v", err)
	}
}
