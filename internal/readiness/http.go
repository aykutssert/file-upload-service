package readiness

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type HTTPChecker struct {
	client *http.Client
	url    string
}

func NewHTTPChecker(client *http.Client, url string) HTTPChecker {
	return HTTPChecker{client: client, url: url}
}

func (c HTTPChecker) Ping(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode < http.StatusOK ||
		response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("health response status: %d", response.StatusCode)
	}
	return nil
}
