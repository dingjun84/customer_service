package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// Password hashing parameters. PBKDF2-HMAC-SHA256 is implemented inline because
// the project ships exactly one dependency and a 30-line KDF is not worth adding
// golang.org/x/crypto to a security-sensitive build.
const (
	pbkdf2Iterations = 120000
	pbkdf2KeyLen     = 32
	pbkdf2SaltLen    = 16
	passwordScheme   = "pbkdf2_sha256"
	minPasswordLen   = 8
)

// HashPassword returns pbkdf2_sha256$<iter>$<salt>$<hash> with a fresh salt.
func HashPassword(password string) (string, error) {
	if len([]rune(password)) < minPasswordLen {
		return "", domain.ValidationError(fmt.Sprintf("密码至少 %d 位", minPasswordLen))
	}
	salt := make([]byte, pbkdf2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := pbkdf2Key([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLen)
	return fmt.Sprintf("%s$%d$%s$%s", passwordScheme, pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword compares a candidate against an encoded hash in constant time.
// Unknown schemes and malformed hashes fail closed.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordScheme {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 || iter > 10_000_000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2Key([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// pbkdf2Key is PBKDF2-HMAC-SHA256 (RFC 8018).
func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	u := make([]byte, hashLen)
	t := make([]byte, hashLen)
	var counter [4]byte

	for block := 1; block <= blocks; block++ {
		prf.Reset()
		prf.Write(salt)
		counter[0] = byte(block >> 24)
		counter[1] = byte(block >> 16)
		counter[2] = byte(block >> 8)
		counter[3] = byte(block)
		prf.Write(counter[:])
		sum := prf.Sum(nil)
		copy(t, sum)
		copy(u, sum)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for i := range u {
				t[i] ^= u[i]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

// newSessionToken returns a 256-bit URL-safe token and the digest to persist.
func newSessionToken() (token, digest string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

// hashToken is the at-rest representation of a session cookie.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Login throttle
// ---------------------------------------------------------------------------

// throttle blocks online password guessing. It is in-process by design: the MVP
// runs a single server, and an attacker who can restart the process has bigger
// problems than a reset counter.
type throttle struct {
	mu       sync.Mutex
	failures map[string]throttleEntry
	max      int
	window   time.Duration
}

type throttleEntry struct {
	count   int
	resetAt time.Time
}

func newThrottle(max int, window time.Duration) *throttle {
	return &throttle{failures: map[string]throttleEntry{}, max: max, window: window}
}

// allow reports whether the key may attempt a login right now.
func (t *throttle) allow(key string, now time.Time) (bool, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.failures[key]
	if !ok || now.After(e.resetAt) {
		delete(t.failures, key)
		return true, 0
	}
	if e.count >= t.max {
		return false, int(e.resetAt.Sub(now).Seconds())
	}
	return true, 0
}

// fail records one failed attempt.
func (t *throttle) fail(key string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.failures[key]
	if !ok || now.After(e.resetAt) {
		e = throttleEntry{resetAt: now.Add(t.window)}
	}
	e.count++
	t.failures[key] = e
	if len(t.failures) > 4096 {
		// Bound memory under a distributed spray: drop entries that already expired.
		for k, v := range t.failures {
			if now.After(v.resetAt) {
				delete(t.failures, k)
			}
		}
	}
}

// reset clears the counter after a successful login.
func (t *throttle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, key)
}

// ---------------------------------------------------------------------------
// Auth service
// ---------------------------------------------------------------------------

// Auth owns accounts and sessions.
type Auth struct {
	db       *sql.DB
	ttl      time.Duration
	throttle *throttle
	now      func() time.Time
}

// NewAuth builds the auth service. ttl is the session lifetime.
func NewAuth(db *sql.DB, ttl time.Duration) *Auth {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Auth{
		db:       db,
		ttl:      ttl,
		throttle: newThrottle(5, 5*time.Minute),
		now:      func() time.Time { return time.Now().UTC().Truncate(time.Second) },
	}
}

// BootstrapUser creates the built-in administrator when the table is empty.
//
// It is idempotent and never overwrites an existing account, so changing
// ADMIN_PASSWORD later has no effect on a database that already has users: that
// is deliberate, otherwise a leaked env var could silently reset production
// credentials on the next restart.
func (a *Auth) BootstrapUser(ctx context.Context, username, password, displayName string) (created bool, err error) {
	var count int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
		return false, fmt.Errorf("count admin users: %w", err)
	}
	if count > 0 {
		return false, nil
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	if displayName == "" {
		displayName = "系统管理员"
	}
	hash, err := HashPassword(password)
	if err != nil {
		return false, fmt.Errorf("内置管理员密码不合法: %w", err)
	}
	now := a.now()
	_, err = a.db.ExecContext(ctx, `INSERT INTO admin_users
		(id, username, password_hash, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'admin', 'active', ?, ?)`,
		domain.NewID(), username, hash, displayName, fmtTime(now), fmtTime(now))
	if err != nil {
		return false, fmt.Errorf("create bootstrap admin: %w", err)
	}
	return true, nil
}

// Login verifies credentials and opens a session. It returns the raw token that
// belongs in the cookie; only the digest is stored.
func (a *Auth) Login(ctx context.Context, username, password, remoteAddr, userAgent string) (User, string, time.Time, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return User{}, "", time.Time{}, domain.ValidationError("请输入用户名和密码")
	}
	now := a.now()
	key := username + "|" + remoteAddr
	if ok, wait := a.throttle.allow(key, now); !ok {
		return User{}, "", time.Time{}, domain.NewError(domain.CodeRateLimited, fmt.Sprintf("登录尝试过于频繁，请 %d 秒后再试", wait))
	}

	var u User
	err := a.db.QueryRowContext(ctx, `SELECT id, username, password_hash, display_name, role, status,
		created_at, COALESCE(last_login_at, '')
		FROM admin_users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Status, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Spend the same time on a missing account so timing does not enumerate users.
		VerifyPassword("pbkdf2_sha256$120000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
		a.throttle.fail(key, now)
		return User{}, "", time.Time{}, domain.NewError(domain.CodeUnauthorized, "用户名或密码错误")
	}
	if err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("load admin user: %w", err)
	}
	if !VerifyPassword(u.PasswordHash, password) {
		a.throttle.fail(key, now)
		return User{}, "", time.Time{}, domain.NewError(domain.CodeUnauthorized, "用户名或密码错误")
	}
	if u.Status != "active" {
		return User{}, "", time.Time{}, domain.NewError(domain.CodeForbidden, "该账号已被停用")
	}

	token, digest, err := newSessionToken()
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	expires := now.Add(a.ttl)
	if _, err := a.db.ExecContext(ctx, `INSERT INTO admin_sessions
		(id, admin_user_id, created_at, expires_at, remote_addr, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)`,
		digest, u.ID, fmtTime(now), fmtTime(expires), truncate(remoteAddr, 64), truncate(userAgent, 200)); err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = ?, updated_at = ? WHERE id = ?`,
		fmtTime(now), fmtTime(now), u.ID); err != nil {
		return User{}, "", time.Time{}, fmt.Errorf("touch last_login_at: %w", err)
	}
	a.throttle.reset(key)
	u.LastLoginAt = fmtTime(now)
	return u, token, expires, nil
}

// ResolveSession returns the live session for a cookie token.
func (a *Auth) ResolveSession(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, domain.NewError(domain.CodeUnauthorized, "未登录")
	}
	digest := hashToken(token)
	now := a.now()

	var (
		u         User
		expiresAt string
		revoked   sql.NullString
	)
	err := a.db.QueryRowContext(ctx, `SELECT u.id, u.username, u.display_name, u.role, u.status,
		u.created_at, COALESCE(u.last_login_at, ''), s.expires_at, s.revoked_at
		FROM admin_sessions s JOIN admin_users u ON u.id = s.admin_user_id
		WHERE s.id = ?`, digest).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Status, &u.CreatedAt, &u.LastLoginAt, &expiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, domain.NewError(domain.CodeUnauthorized, "登录已失效，请重新登录")
	}
	if err != nil {
		return Session{}, fmt.Errorf("load session: %w", err)
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return Session{}, fmt.Errorf("parse session expiry: %w", err)
	}
	if revoked.Valid || !now.Before(exp) {
		return Session{}, domain.NewError(domain.CodeUnauthorized, "登录已过期，请重新登录")
	}
	if u.Status != "active" {
		return Session{}, domain.NewError(domain.CodeForbidden, "该账号已被停用")
	}
	return Session{User: u, ExpiresAt: exp}, nil
}

// Logout revokes the session behind a cookie token. Revoking an unknown token is
// a no-op so a stale cookie can always clear itself.
func (a *Auth) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	_, err := a.db.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		fmtTime(a.now()), hashToken(token))
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// PurgeExpired deletes sessions that can no longer authenticate anything.
func (a *Auth) PurgeExpired(ctx context.Context) (int64, error) {
	res, err := a.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at < ?`, fmtTime(a.now().Add(-24*time.Hour)))
	if err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}
	return res.RowsAffected()
}

// ActiveSessions counts live sessions, shown on the health card.
func (a *Auth) ActiveSessions(ctx context.Context) (int, error) {
	var n int
	err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_sessions
		WHERE revoked_at IS NULL AND expires_at > ?`, fmtTime(a.now())).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return n, nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
