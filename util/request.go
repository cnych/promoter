package util

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/prometheus/common/version"
)

var UserAgentHeader = fmt.Sprintf("Promoter/%s", version.Version)

// RedactURL removes the URL part from an error of *url.Error type.
func RedactURL(err error) error {
	e, ok := err.(*url.Error)
	if !ok {
		return err
	}
	e.URL = "<redacted>"
	return e
}

func request(ctx context.Context, client *http.Client, method string, url string, bodyType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgentHeader)
	if bodyType != "" {
		req.Header.Set("Content-Type", bodyType)
	}
	return client.Do(req.WithContext(ctx))
}

// Get sends a GET request to the given URL
func Get(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	return request(ctx, client, http.MethodGet, url, "", nil)
}

// PostJSON sends a POST request with JSON payload to the given URL.
func PostJSON(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
	//b, _ := io.ReadAll(body)
	return post(ctx, client, url, "application/json", body)
}

// PostText sends a POST request with text payload to the given URL.
func PostText(ctx context.Context, client *http.Client, url string, body io.Reader) (*http.Response, error) {
	return post(ctx, client, url, "text/plain", body)
}

func post(ctx context.Context, client *http.Client, url string, bodyType string, body io.Reader) (*http.Response, error) {
	return request(ctx, client, http.MethodPost, url, bodyType, body)
}

// Drain consumes and closes the response's body to make sure that the
// HTTP client can reuse existing connections.
func Drain(r *http.Response) {
	io.Copy(ioutil.Discard, r.Body)
	r.Body.Close()
}
