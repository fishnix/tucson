package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fishnix/tucson/internal/srv"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

type origins map[string]*srv.Origin
type matchers []*srv.Matcher

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "starts the tucson server",
	RunE: func(cmd *cobra.Command, args []string) error {
		return serve()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().String("listen", "0.0.0.0:8000", "address to listen on")
	viperBindFlag("listen", serveCmd.Flags().Lookup("listen"))

	serveCmd.Flags().String("default-origin", "default", "name of the default origin")
	viperBindFlag("default-origin", serveCmd.Flags().Lookup("default-origin"))
	viperBindEnv("default-origin")

	serveCmd.Flags().StringP("signing-key", "k", "", "signing key for token authentication")
	viperBindFlag("signing-key", serveCmd.Flags().Lookup("signing-key"))
	viperBindEnv("signing-key")

	serveCmd.Flags().String("oidc-issuer", "", "oidc issuer url")
	viperBindFlag("oidc.issuer", serveCmd.Flags().Lookup("oidc-issuer"))
	viperBindEnv("oidc.issuer")

	serveCmd.Flags().String("oidc-client-id", "", "oidc client id")
	viperBindFlag("oidc.client-id", serveCmd.Flags().Lookup("oidc-client-id"))
	viperBindEnv("oidc.client-id")

	serveCmd.Flags().String("oidc-client-secret", "", "oidc client secret")
	viperBindFlag("oidc.client-secret", serveCmd.Flags().Lookup("oidc-client-secret"))
	viperBindEnv("oidc.client-secret")

	serveCmd.Flags().String("oidc-redirect-url", "http://localhost:8000/auth/callback", "oidc callback/redirect url")
	viperBindFlag("oidc.redirect-url", serveCmd.Flags().Lookup("oidc-redirect-url"))
	viperBindEnv("oidc.redirect-url")
}

func serve() error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-c
		cancel()
	}()

	o := origins{}
	if err := viper.UnmarshalKey("origins", &o); err != nil {
		panic(err)
	}

	for k, v := range o {
		logger.Debugw("adding origin", zap.String("name", k), zap.Any("origin", v))
	}

	do, ok := o[viper.GetString("default-origin")]
	if !ok {
		panic("default origin not found!")
	}

	m := matchers{}
	if err := viper.UnmarshalKey("matchers", &m); err != nil {
		panic(err)
	}

	for _, v := range m {
		logger.Debugw("adding matcher", zap.Any("matcher", v))
	}

	u, _ := uuid.NewUUID()
	sk := u.String()
	if viper.IsSet("signing-key") {
		sk = viper.GetString("signing-key")
	}

	provider, err := newOidcProvider(ctx)
	if err != nil {
		panic(err)
	}

	server := srv.New(
		srv.WithDebug(viper.GetBool("logging.debug")),
		srv.WithLogger(logger.Desugar()),
		srv.WithListen(viper.GetString("listen")),
		srv.WithDefaultOrigin(do),
		srv.WithOrigins(o),
		srv.WithMatchers(m),
		srv.WithSigningKey(sk),
		srv.WithOidcProvider(provider),
		srv.WithOauth2Config(newOauth2Config(provider)),
	)

	logger.Infow("starting server", "address", viper.GetString("listen"))

	if err := server.Run(ctx); err != nil {
		logger.Fatalw("failed starting server", "error", err)
	}

	return nil
}

func newOidcProvider(ctx context.Context) (*oidc.Provider, error) {
	return oidc.NewProvider(ctx, viper.GetString("oidc.issuer"))
}

func newOauth2Config(provider *oidc.Provider) oauth2.Config {
	return oauth2.Config{
		ClientID:     viper.GetString("oidc.client-id"),
		ClientSecret: viper.GetString("oidc.client-secret"),
		RedirectURL:  viper.GetString("oidc.redirect-url"),
		Endpoint:     provider.Endpoint(),

		// "openid" is a required scope for OpenID Connect flows.
		// TODO: make this configurable
		Scopes: []string{oidc.ScopeOpenID},
	}
}
