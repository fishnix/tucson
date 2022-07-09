package srv

import (
	"errors"
	"net/http"

	"github.com/go-chi/jwtauth/v5"
	"github.com/lestrrat-go/jwx/jwt"
)

func (s *Server) Authenticator(ja *jwtauth.JWTAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		hfn := func(w http.ResponseWriter, r *http.Request) {
			tokenCookie, err := r.Cookie("jwt")
			if err != nil {
				s.logger.Debug("token not found in cookies")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			token, err := VerifyToken(ja, tokenCookie.Value)
			if err != nil {
				s.logger.Debug("error validating token")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			if token == nil {
				s.logger.Debug("token is nil")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			if jwt.Validate(token) != nil {
				s.logger.Debug("token is not valid")
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			// Token is authenticated, pass it through
			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(hfn)
	}
}

func VerifyToken(ja *jwtauth.JWTAuth, tokenString string) (jwt.Token, error) {
	// Decode & verify the token
	token, err := ja.Decode(tokenString)
	if err != nil {
		return token, err
	}

	if token == nil {
		return nil, errors.New("Unauthorized")
	}

	if err := jwt.Validate(token); err != nil {
		return token, err
	}

	// Valid!
	return token, nil
}
