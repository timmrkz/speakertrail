package api

import (
	"net/http"
	"strconv"
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
	// The app says which parts cannot go, and why, so the window only
	// shows it.
	var now templateJSON
	call(t, a, "GET", "/api/template", "", &now)
	kept := map[string]string{}
	for _, p := range now.Parts {
		kept[p.Name] = p.Kept
	}
	if kept["origin"] != "Origin is where the message starts, so it stays." || kept["opener"] != "Origin still uses #opener#. Take it out there first." || kept["wave"] != "" {
		t.Errorf("kept: %q", kept)
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
	// Undo in the Edit menu puts it back where it was.
	if code := call(t, a, "POST", "/api/undo", "{}", nil); code != http.StatusOK {
		t.Errorf("undo of the part taken out: %d", code)
	}
	call(t, a, "GET", "/api/template", "", &tj)
	before := tj
	if !strings.Contains(names(before), "wave") {
		t.Errorf("after undo the parts are %s, want wave back", names(before))
	}
	tj = templateJSON{}
	if code := call(t, a, "DELETE", "/api/template", "", &tj); code != http.StatusOK || tj.Changed || names(tj) != first {
		t.Errorf("reset: %d %s", code, names(tj))
	}
	call(t, a, "POST", "/api/undo", "{}", nil)
	call(t, a, "GET", "/api/template", "", &tj)
	if names(tj) != names(before) || !tj.Changed {
		t.Errorf("reset undone: %s", names(tj))
	}
	var ex map[string]string
	if code := call(t, a, "GET", "/api/template/example", "", &ex); code != http.StatusOK ||
		!strings.Contains(ex["text"], "weil [reason]") || !strings.Contains(ex["text"], " [first_name],\n\n") {
		t.Errorf("example: %d %q", code, ex["text"])
	}
	// One seed makes one message, so the example keeps its shape while
	// Tim edits, and other seeds make others.
	seen := map[string]bool{}
	for seed := range 40 {
		var one, again map[string]string
		path := "/api/template/example?seed=" + strconv.Itoa(seed)
		call(t, a, "GET", path, "", &one)
		call(t, a, "GET", path, "", &again)
		if one["text"] != again["text"] {
			t.Fatalf("seed %d made two messages:\n%s\n%s", seed, one["text"], again["text"])
		}
		seen[one["text"]] = true
	}
	if len(seen) < 5 {
		t.Errorf("40 seeds made only %d messages", len(seen))
	}
}
