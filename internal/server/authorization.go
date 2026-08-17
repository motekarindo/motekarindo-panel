package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/rbac"
)

type PermissionAuthorizer interface {
	Authorize(ctx context.Context, userID, permission string) error
}

type sessionPrincipalContextKey struct{}

func (s *Server) RequirePermission(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.setAuthResponseHeaders(w)
		if s.sessions == nil || s.authorization == nil || strings.TrimSpace(permission) == "" || next == nil {
			writeAuthorizationError(w, http.StatusInternalServerError, "internal_error", "Unable to authorize request.")
			return
		}

		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writeAuthorizationError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required.")
			return
		}
		principal, err := s.sessions.Validate(r.Context(), cookie.Value)
		if errors.Is(err, auth.ErrInvalidSession) {
			writeAuthorizationError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required.")
			return
		}
		if err != nil {
			writeAuthorizationError(w, http.StatusInternalServerError, "internal_error", "Unable to authorize request.")
			return
		}
		if err := s.authorization.Authorize(r.Context(), principal.UserID, permission); errors.Is(err, rbac.ErrForbidden) {
			writeAuthorizationError(w, http.StatusForbidden, "forbidden", "Permission denied.")
			return
		} else if err != nil {
			writeAuthorizationError(w, http.StatusInternalServerError, "internal_error", "Unable to authorize request.")
			return
		}

		ctx := context.WithValue(r.Context(), sessionPrincipalContextKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SessionPrincipalFromContext(ctx context.Context) (auth.SessionPrincipal, bool) {
	principal, ok := ctx.Value(sessionPrincipalContextKey{}).(auth.SessionPrincipal)
	return principal, ok
}

func writeAuthorizationError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
