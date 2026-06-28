package bookmeta

import (
	"context"

	"github.com/banjuer/kompanion/internal/entity"
)

type LookupResult struct {
	Book  entity.Book
	Cover []byte
}

type Provider interface {
	LookupByISBN(ctx context.Context, isbn string) (LookupResult, error)
}

type CookieSource interface {
	Cookie(ctx context.Context, domain string) (string, error)
}

type StaticCookieSource struct {
	cookie string
}

func NewStaticCookieSource(cookie string) StaticCookieSource {
	return StaticCookieSource{cookie: cookie}
}

func (s StaticCookieSource) Cookie(context.Context, string) (string, error) {
	return s.cookie, nil
}

type DisabledProvider struct{}

func (DisabledProvider) LookupByISBN(context.Context, string) (LookupResult, error) {
	return LookupResult{}, ErrProviderDisabled
}
