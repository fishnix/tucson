package token

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/square/go-jose.v2"
)

func TestNew(t *testing.T) {
	testValidNbf, _ := time.Parse(time.RFC3339, "2021-01-02T15:04:05Z")
	testValidExp, _ := time.Parse(time.RFC3339, "2021-01-02T16:04:05Z")

	tests := []struct {
		name    string
		alg     jose.SignatureAlgorithm
		key     string
		sub     string
		nbf     time.Time
		exp     time.Time
		want    string
		wantErr bool
	}{
		{
			name: "example token",
			key:  "secret",
			sub:  "subject",
			nbf:  testValidNbf,
			exp:  testValidExp,
			want: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2MDk2MDM0NDUsIm5iZiI6MTYwOTU5OTg0NSwic3ViIjoic3ViamVjdCJ9.CL9Lnu8gKOjdkjVivxOxpBPv8KS4pQazkCRoDIbuf5o",
		},
		{
			name:    "error empty key",
			key:     "",
			sub:     "subject",
			nbf:     testValidNbf,
			exp:     testValidExp,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(
				WithKey(tt.key),
				WithSubject(tt.sub),
				WithNotBefore(tt.nbf),
				WithExpire(tt.exp),
			)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
