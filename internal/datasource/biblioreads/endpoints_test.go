package biblioreads_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/svera/coreander/v5/internal/datasource/biblioreads"
)

type unexpectedRequestTransport struct{ t *testing.T }

func (transport unexpectedRequestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.t.Errorf("unexpected network request to %s", request.URL)
	return nil, errors.New("network requests must require explicit endpoints")
}

func TestClientWithoutEndpointsMakesNoRequests(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts []biblioreads.Option
	}{
		{name: "default"},
		{name: "empty", opts: []biblioreads.Option{biblioreads.WithInstances(nil)}},
		{name: "explicitly cleared", opts: []biblioreads.Option{
			biblioreads.WithInstances([]string{"http://localhost:8080"}),
			biblioreads.WithInstances(nil),
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := biblioreads.NewClient(append(tt.opts, biblioreads.WithHTTPClient(
				&http.Client{Transport: unexpectedRequestTransport{t: t}},
			))...)
			ctx := context.Background()
			_, searchErr := client.SearchBooks(ctx, "Book")
			_, bookErr := client.Book(ctx, "123")
			_, authorErr := client.Author(ctx, "123")
			_, reviewsErr := client.Reviews(ctx, "work", 123, "", "en")
			for _, err := range []error{searchErr, bookErr, authorErr, reviewsErr} {
				if !errors.Is(err, biblioreads.ErrUnavailable) {
					t.Errorf("expected explicit unavailable error without endpoints, got %v", err)
				}
			}
		})
	}
}
