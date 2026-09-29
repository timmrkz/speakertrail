package pipeline_test

import (
	"testing"

	"github.com/timmrkz/speakertrail/internal/pipeline"
)

// Errors as the fetcher and the model record them, with invented hosts.
func TestExplain(t *testing.T) {
	for _, c := range []struct {
		msg    string
		status int
		reason string
		kind   string
	}{
		{"https://beispiel.example/: disallowed by robots.txt", 0, "robots.txt does not allow the bot", pipeline.FailBlocked},
		{"www.instagram.com: host is never requested", 0, "LinkedIn and Instagram are never requested", pipeline.FailBlocked},
		{"get https://beispiel.example/: status 403", 403, "The site refuses the bot", pipeline.FailBlocked},
		{"get https://beispiel.example/: status 410", 0, "The page is gone", pipeline.FailGone},
		{"get https://beispiel.example/: dial tcp: lookup beispiel.example: no such host", 0, "The website's name no longer exists", pipeline.FailGone},
		{"get https://beispiel.example/: status 429", 429, "The site asks the bot to slow down", pipeline.FailPassing},
		{"get https://beispiel.example/: status 502", 502, "The site had an error of its own, HTTP 502", pipeline.FailPassing},
		{"get https://beispiel.example/: status 418", 418, "The site answered with an error, HTTP 418", pipeline.FailLasting},
		{`get "https://beispiel.example/": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, 0, "The site did not answer in time", pipeline.FailPassing},
		{"get https://beispiel.example/: dial tcp 192.0.2.1:443: connect: connection refused", 0, "The site refused the connection", pipeline.FailPassing},
		{"get https://beispiel.example/: tls: failed to verify certificate: x509: certificate has expired", 0, "The site's certificate is not valid", pipeline.FailLasting},
		{"get https://beispiel.example/: stopped after 10 redirects: too many redirects", 0, "The page sends the bot in circles", pipeline.FailLasting},
		{"no Chromium found, set CHROME_PATH", 0, "The headless browser did not start", pipeline.FailEngine},
		{"no language model answers. Is Docker Model Runner on? Turn it on with: docker desktop enable model-runner (dial tcp: refused)", 0,
			"The language model did not answer. Is Docker Model Runner on?", pipeline.FailEngine},
		{"the language model failed: no answer within 2m0s", 0, "The language model took too long", pipeline.FailPassing},
		{"the model's answer is not JSON: unexpected end", 0, "The language model gave no usable answer", pipeline.FailPassing},
		{"something nobody expected", 0, "Something unexpected went wrong", pipeline.FailPassing},
	} {
		reason, kind := pipeline.Explain(c.msg, c.status)
		if reason != c.reason || kind != c.kind {
			t.Errorf("%q: %q %s, want %q %s", c.msg, reason, kind, c.reason, c.kind)
		}
	}
}
