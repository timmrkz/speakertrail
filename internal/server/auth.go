package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName     = "st_session"
	sessionTTL     = 30 * 24 * time.Hour
	loginAttempts  = 5
	loginWindow    = time.Minute
	passwordMaxLen = 200
)

// sessions signs and checks the single user's session cookie. The cookie
// holds only an expiry time and its HMAC, so nothing needs to be stored.
type sessions struct {
	secret []byte
	hash   []byte
	now    func() time.Time

	mu       sync.Mutex
	attempts map[string][]time.Time
}

func newSessions(secret, passwordHash string, now func() time.Time) *sessions {
	return &sessions{secret: []byte(secret), hash: []byte(passwordHash), now: now, attempts: map[string][]time.Time{}}
}

func (s *sessions) enabled() bool { return len(s.secret) >= 16 && len(s.hash) > 0 }

func (s *sessions) sign(expiry time.Time) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(expiry.Unix()))
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(buf)
	return base64.RawURLEncoding.EncodeToString(append(buf, mac.Sum(nil)...))
}

func (s *sessions) valid(value string) bool {
	if !s.enabled() {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) != 8+sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(raw[:8])
	if !hmac.Equal(mac.Sum(nil), raw[8:]) {
		return false
	}
	expiry := time.Unix(int64(binary.BigEndian.Uint64(raw[:8])), 0)
	return s.now().Before(expiry)
}

func (s *sessions) loggedIn(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	return err == nil && s.valid(c.Value)
}

// allowAttempt limits password guesses per client address.
func (s *sessions) allowAttempt(r *http.Request) bool {
	ip := clientIP(r)
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var recent []time.Time
	for _, t := range s.attempts[ip] {
		if now.Sub(t) < loginWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= loginAttempts {
		s.attempts[ip] = recent
		return false
	}
	s.attempts[ip] = append(recent, now)
	return true
}

func (s *sessions) checkPassword(pw string) bool {
	if !s.enabled() || len(pw) == 0 || len(pw) > passwordMaxLen {
		return false
	}
	return bcrypt.CompareHashAndPassword(s.hash, []byte(pw)) == nil
}

func (s *sessions) setCookie(w http.ResponseWriter, r *http.Request) {
	expiry := s.now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: s.sign(expiry), Path: "/", Expires: expiry,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clientIP(r *http.Request) string {
	// The platform's proxy appends the real client address last. Earlier
	// entries come from the client and cannot be trusted.
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		parts := strings.Split(f, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
