package jwks

import (
	"context"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
)

func NewProvider(ctx context.Context, jwksURL string) (keyfunc.Keyfunc, error) {
	storage, err := jwkset.NewStorageFromHTTP(jwksURL, jwkset.HTTPClientStorageOptions{
		Ctx:               ctx,
		RefreshInterval:   5 * time.Minute,
		RefreshErrorHandler: func(_ context.Context, err error) {
			// Continue using cached keys on refresh failure
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