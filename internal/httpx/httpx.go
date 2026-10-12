// Package httpx sends HTTP requests that identify the application by a user agent.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Client sends requests with a user agent.
type Client struct {
	// HTTP is the client for requests. Nil means [http.DefaultClient].
	HTTP *http.Client
	// UserAgent identifies the application, as the services require.
	UserAgent string
}

// Do sends the request with the user agent.
func (c Client) Do(req *http.Request) (*http.Response, error) {
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// DoJSON sends the request, and decodes the JSON response into v.
// For a response with an error status, errMsg, if not nil, extracts the message of the service from the body.
func (c Client) DoJSON(req *http.Request, v any, errMsg func([]byte) string) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		if errMsg != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			if msg := errMsg(body); msg != "" {
				return errors.New(msg)
			}
		}
		return errors.New(resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
