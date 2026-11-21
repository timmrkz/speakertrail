package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func names(tj templateJSON) string {
	var out []string
	for _, p := range tj.Parts {
		out = append(out, p.Name)
	}
	return strings.Join(out, " ")
}

// Tim edits the template in Settings: a part's variations, a new part, a
// part taken out and put back where it was, and his first template back.
func TestTimEditsTheTemplate(t *testing.T) {
	a, _, _ := newAPI(t)
	var tj templateJSON
	if code := call(t, a, "GET", "/api/template", "", &tj); code != http.StatusOK || tj.Changed ||
		!strings.HasPrefix(names(tj), "origin greeting opener hook_frame") || len(tj.Fields) != 4 {
		t.Fatalf("at first: %d %s %+v", code, names(tj), tj.Fields)
	}
	first := names(tj)
	if code := call(t, a, "PUT", "/api/template/parts/greeting", `{"variations":["Hallo","Hi",""]}`, &tj); code != http.StatusOK || !tj.Changed {
		t.Errorf("a part changed: %d %+v", code, tj)
	}
	if code := call(t, a, "PUT", "/api/template/parts/wave", `{"variations":["👋"],"at":1}`, &tj); code != http.StatusOK ||
		!strings.HasPrefix(names(tj), "origin wave greeting") {
		t.Errorf("a part added: %d %s", code, names(tj))
	}
	// What cannot work is refused and says why.
	var refused map[string]string
	for path, body := range map[string]string{
		"/api/template/parts/greeting": `{"variations":["#nope#"]}`,
		"/api/template/parts/Bad":      `{"variations":["x"]}`,
		"/api/template/parts/reason":   `{"variations":["x"]}`,
	} {
		if code := call(t, a, "PUT", path, body, &refused); code != http.StatusBadRequest || !strings.HasSuffix(refused["error"], ".") {
			t.Errorf("%s %s: %d %v", path, body, code, refused)
		}
	}
	if code := call(t, a, "DELETE", "/api/template/parts/opener", "", &refused); code != http.StatusBadRequest ||
		refused["error"] != "Origin still uses #opener#. Take it out there first." {
		t.Errorf("a part in use taken out: %d %v", code, refused)
	}
	if code := call(t, a, "DELETE", "/api/template/parts/nope", "", nil); code != http.StatusNotFound {
		t.Errorf("a part that is not there: %d", code)
	}
	if code := call(t, a, "DELETE", "/api/template/parts/wave", "", &tj); code != http.StatusOK || strings.Contains(names(tj), "wave") {
		t.Errorf("a part taken out: %d %s", code, names(tj))
	}
	// Undo puts it back where it was.
	call(t, a, "PUT", "/api/template/parts/wave", `{"variations":["👋"],"at":1}`, &tj)
	before := tj
	tj = templateJSON{}
	if code := call(t, a, "DELETE", "/api/template", "", &tj); code != http.StatusOK || tj.Changed || names(tj) != first {
		t.Errorf("reset: %d %s", code, names(tj))
	}
	raw, _ := json.Marshal(map[string]any{"parts": before.Parts})
	body := string(raw)
	if code := call(t, a, "PUT", "/api/template", body, &tj); code != http.StatusOK || names(tj) != names(before) {
		t.Errorf("reset undone: %d %s", code, names(tj))
	}
	var ex map[string]string
	if code := call(t, a, "GET", "/api/template/example", "", &ex); code != http.StatusOK ||
		!strings.Contains(ex["text"], "weil [reason]") || !strings.Contains(ex["text"], " [first_name],\n\n") {
		t.Errorf("example: %d %q", code, ex["text"])
	}
}
