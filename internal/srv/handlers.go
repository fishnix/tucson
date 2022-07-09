package srv

import (
	"errors"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fishnix/tucson/internal/token"
	"go.uber.org/zap"
	"gopkg.in/square/go-jose.v2/jwt"
)

// writeHTTPResponse writes the http response and panics on write errors
func writeHTTPResponse(w http.ResponseWriter, payload []byte) {
	if _, err := w.Write(payload); err != nil {
		panic(err)
	}
}

// livenessCheck ensures that the server is up and responding
func (s *Server) livenessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeHTTPResponse(w, []byte(`{"status":"UP"}`))
}

// readinessCheck ensures that the server is up and that we are able to process requests.
func (s *Server) readinessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeHTTPResponse(w, []byte(`{"status":"UP"}`))
}

func (s *Server) proxyOriginHandler(o *Origin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.logger.Info("inside proxy origin handler func!",
			zap.String("req.url", r.URL.String()),
			zap.String("http.method", r.Method),
		)
		prox := s.newProxy(o, s.logger)
		prox.proxyRequest(w, r)
	}
}

func (s *Server) handleOAuth2Callback(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("handling OIDC callback, exchanging code for token")

	// Verify state and errors.
	oauth2Token, err := s.oauth2Config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.logger.Error("error exchanging code from token", zap.Error(err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	s.logger.Debug("exchanged code for id_token", zap.Any("token", oauth2Token))

	// Extract the ID Token from OAuth2 token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		s.logger.Error("missing token token", zap.Error(errors.New("missing token")))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	verifier := s.oidcProvider.Verifier(&oidc.Config{ClientID: s.oauth2Config.ClientID})

	// Parse and verify ID Token payload.
	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.logger.Error("error verifying token", zap.Error(err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	s.logger.Debug("token verified, got oidc token, parsing claims", zap.Any("token", idToken))

	claims := struct {
		Email      string `json:"email"`
		Name       string `json:"name"`
		UniqueName string `json:"unique_name"`
	}{}

	// decode JWT token without verifying the signature (verified above)
	parsedToken, err := jwt.ParseSigned(oauth2Token.AccessToken)
	if err != nil {
		s.logger.Error("error parsing signed token", zap.Error(err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := parsedToken.UnsafeClaimsWithoutVerification(&claims); err != nil {
		s.logger.Error("error marshalling claims fromtoken", zap.Error(err))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	s.logger.Debug("parsed claims from token", zap.Any("claims", claims))

	// generate the node token for requesting secrets from the scaler
	rawToken, err := token.New(
		token.WithKey(s.signingKey),
		token.WithSubject(claims.Email),
		token.WithNotBefore(time.Now()),
		token.WithExpire(time.Now().Add(5*time.Minute)),
		token.WithPrivate(
			struct {
				Name       string `json:"name"`
				UniqueName string `json:"unique_name"`
			}{claims.Name, claims.UniqueName},
		),
	)
	if err != nil {
		s.logger.Error("failed to generate token", zap.Error(err))

	}

	addCookie(w, "jwt", rawToken, 60*time.Minute)

	http.Redirect(w, r, "/", http.StatusFound)
}

// addCookie will apply a new cookie to the response of a http request
// with the key/value specified.
func addCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	expire := time.Now().Add(ttl)
	cookie := http.Cookie{
		Name:    name,
		Value:   value,
		Expires: expire,
		Path:    "/",
	}
	http.SetCookie(w, &cookie)
}
