package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kinds of failure, by what fixes them.
const (
	// FailBlocked: the site refuses the bot. The engine makes the source
	// manual by itself.
	FailBlocked = "blocked"
	// FailGone: the page or the whole website no longer exists. The engine
	// retires the source by itself.
	FailGone = "gone"
	// FailLasting: the site will not work for the bot without a change on
	// its side, so retiring the source is the fix.
	FailLasting = "lasting"
	// FailPassing: the site failed this once. Another check may work.
	FailPassing = "passing"
	// FailEngine: something on this side is missing, like the model or the
	// browser. No source is to blame.
	FailEngine = "engine"
)

var statusIn = regexp.MustCompile(`status (\d{3})`)

// Explain says in plain words why a check, read or lookup failed, from the
// error it recorded and the HTTP status, and which kind of failure it is.
func Explain(msg string, status int) (reason, kind string) {
	if status == 0 {
		if m := statusIn.FindStringSubmatch(msg); m != nil {
			status, _ = strconv.Atoi(m[1])
		}
	}
	low := strings.ToLower(msg)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(low, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("tavily answered 401", "brave answered 401", "tavily answered 403", "brave answered 403"):
		return "The search provider does not accept the key", FailEngine
	case has("tavily answered", "brave answered"):
		return "The search provider refused the search, maybe its limit is reached", FailEngine
	case has("disallowed by robots.txt"):
		return "robots.txt does not allow the bot", FailBlocked
	case has("host is never requested"):
		return "LinkedIn and Instagram are never requested", FailBlocked
	case status == 401 || status == 403:
		return "The site refuses the bot", FailBlocked
	case status == 404 || status == 410:
		return "The page is gone", FailGone
	case has("no such host"):
		return "The website's name no longer exists", FailGone
	case has("no language model answers"):
		return "The language model did not answer. Is Docker Model Runner on?", FailEngine
	case has("is not on this machine yet"):
		return "The language model is not on this machine yet", FailEngine
	case has("no chromium found", "start chromium", "no headless browser"):
		return "The headless browser did not start", FailEngine
	case has("no answer within"):
		return "The language model took too long", FailPassing
	case has("the language model failed", "the model's answer", "the model gave no answer", "the model answered"):
		return "The language model gave no usable answer", FailPassing
	case status == 429:
		return "The site asks the bot to slow down", FailPassing
	case status >= 500:
		return fmt.Sprintf("The site had an error of its own, HTTP %d", status), FailPassing
	case has("certificate", "x509", "tls:"):
		return "The site's certificate is not valid", FailLasting
	case has("too many redirects"):
		return "The page sends the bot in circles", FailLasting
	case has("timeout", "deadline exceeded"):
		return "The site did not answer in time", FailPassing
	case has("connection refused"):
		return "The site refused the connection", FailPassing
	case has("connection reset", "eof", "broken pipe"):
		return "The site broke off the connection", FailPassing
	case status >= 400:
		return fmt.Sprintf("The site answered with an error, HTTP %d", status), FailLasting
	}
	return "Something unexpected went wrong", FailPassing
}
