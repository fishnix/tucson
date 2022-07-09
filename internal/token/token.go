package token

import (
	"errors"
	"time"

	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

var (
	// ErrSecretKeyEmpty is the error returned when the secret key is empty
	ErrSecretKeyEmpty = errors.New("secret key cannot be empty")
)

// Token is an authentication token
type Token struct {
	alg     jose.SignatureAlgorithm
	exp     time.Time
	key     string
	nbf     time.Time
	subject string
	private []interface{}
}

// Option is a functional configuration option
type Option func(t *Token)

// New generates and returns a new signed token
func New(opts ...Option) (string, error) {
	t := Token{
		alg: jose.HS256,
	}

	for _, o := range opts {
		o(&t)
	}

	if err := t.preFlight(); err != nil {
		return "", err
	}

	return t.newSigned()
}

// WithKey sets the secret key used for signing
func WithKey(k string) Option {
	return func(t *Token) {
		t.key = k
	}
}

// WithAlgorithm sets the algorithm used for signing
func WithAlgorithm(a jose.SignatureAlgorithm) Option {
	return func(t *Token) {
		t.alg = a
	}
}

// WithNotBefore sets the jwt nbf
func WithNotBefore(d time.Time) Option {
	return func(t *Token) {
		t.nbf = d
	}
}

// WithExpire sets the jwt exp
func WithExpire(d time.Time) Option {
	return func(t *Token) {
		t.exp = d
	}
}

// WithSubject sets the jwt subject
func WithSubject(s string) Option {
	return func(t *Token) {
		t.subject = s
	}
}

// WithPrivate sets private claims
func WithPrivate(c interface{}) Option {
	return func(t *Token) {
		if t.private == nil {
			t.private = []interface{}{}
		}

		t.private = append(t.private, c)
	}
}

// preFlight validates we aren't doing anything too foolish
func (t *Token) preFlight() error {
	if t.key == "" {
		return ErrSecretKeyEmpty
	}

	return nil
}

func (t *Token) newSigned() (string, error) {
	signingKey := jose.SigningKey{
		Algorithm: t.alg,
		Key:       []byte(t.key),
	}

	opts := &jose.SignerOptions{}

	sig, err := jose.NewSigner(signingKey, opts.WithType("JWT"))
	if err != nil {
		return "", err
	}

	cl := jwt.Claims{
		Subject:   t.subject,
		NotBefore: jwt.NewNumericDate(t.nbf.UTC()),
		Expiry:    jwt.NewNumericDate(t.exp.UTC()),
	}

	builder := jwt.Signed(sig).Claims(cl)
	for _, p := range t.private {
		builder = builder.Claims(p)
	}

	return builder.CompactSerialize()
}
