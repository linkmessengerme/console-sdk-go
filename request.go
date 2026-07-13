package link

import (
	"strings"

	"github.com/linkmessengerme/console-sdk-go/internal/idempotency"
	"github.com/linkmessengerme/console-sdk-go/internal/transport"
)

type RequestOption func(*requestOptions)

type requestOptions struct {
	idempotencyKey string
}

func WithIdempotencyKey(key string) RequestOption {
	return func(o *requestOptions) {
		o.idempotencyKey = strings.TrimSpace(key)
	}
}

func NewIdempotencyKey() (string, error) {
	return idempotency.NewKey()
}

func buildRequestOptions(opts []RequestOption) (transport.RequestOptions, error) {
	var ro requestOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&ro)
		}
	}
	if ro.idempotencyKey == "" {
		key, err := idempotency.NewKey()
		if err != nil {
			return transport.RequestOptions{}, err
		}
		ro.idempotencyKey = key
	}
	if err := idempotency.Validate(ro.idempotencyKey); err != nil {
		return transport.RequestOptions{}, err
	}
	return transport.RequestOptions{IdempotencyKey: ro.idempotencyKey}, nil
}
