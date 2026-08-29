package jwks

import (
	"context"
	"log/slog"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
)

func NewProvider(ctx context.Context, jwksURL string) (keyfunc.Keyfunc, error) {
	storage, err := jwkset.NewStorageFromHTTP(jwksURL, jwkset.HTTPClientStorageOptions{
		Ctx:             ctx,
		RefreshInterval: 5 * time.Minute,
		RefreshErrorHandler: func(_ context.Context, err error) {
			// Keep serving cached keys on refresh failure, but make the
			// degradation visible — a silently stuck JWKS is otherwise
			// indistinguishable from healthy.
			slog.Error("jwks: refresh failed, serving cached keys", "error", err)
		},
	})
	if err != nil {
		return nil, err
	}

	k, err := keyfunc.New(keyfunc.Options{
		Ctx:     ctx,
		Storage: storage,
	})
	if err != nil {
		return nil, err
	}
	return k, nil
}
