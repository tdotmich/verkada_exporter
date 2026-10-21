package verkada

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// A Verkada API token is valid for 30 minutes. We get a new one after 25 minutes,
	// so a token never expires in the middle of a scrape.
	tokenLifetime = 25 * time.Minute

	// Verkada returns no more than 200 cameras on each page.
	camerasPerPage = 200

	// maxCameraPages stops the page loop if the API keeps returning a next page token.
	maxCameraPages = 500
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string

	// Prometheus can scrape at the same time from two servers, so two goroutines can
	// use the client at once. The mutex protects the token fields.
	tokenMutex  sync.Mutex
	token       string
	tokenExpiry time.Time
}

type tokenResponse struct {
	Token string `json:"token"`
}

// NewClient returns a client for the Verkada API in one region, for example https://api.verkada.com.
func NewClient(baseURL string, apiKey string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,

			// Do not follow redirects. Go keeps custom headers such as x-api-key when it
			// follows a redirect, so a redirect to another host could send it our API key.
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

// getToken returns a valid API token. It only calls POST /token when there is no token
// or the token is old. The API key goes only to /token. All other calls use the token.
func (client *Client) getToken(ctx context.Context) (string, error) {
	client.tokenMutex.Lock()
	defer client.tokenMutex.Unlock()

	if client.token != "" && time.Now().Before(client.tokenExpiry) {
		return client.token, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/token", nil)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}

	request.Header.Set("accept", "application/json")
	request.Header.Set("x-api-key", client.apiKey)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("send token request: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"token request returned HTTP status %s: %s",
			response.Status,
			readErrorBody(response),
		)
	}

	var tokenData tokenResponse

	err = json.NewDecoder(response.Body).Decode(&tokenData)
	if err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	if tokenData.Token == "" {
		return "", fmt.Errorf("token response did not contain a token")
	}

	client.token = tokenData.Token
	client.tokenExpiry = time.Now().Add(tokenLifetime)

	return client.token, nil
}

// forgetToken removes the saved token, so the next call gets a new one.
func (client *Client) forgetToken() {
	client.tokenMutex.Lock()
	defer client.tokenMutex.Unlock()

	client.token = ""
}

// getJSON sends a GET request with the API token and decodes the JSON response into result.
// If Verkada says the token is not valid (HTTP 401), it gets a new token and tries once more.
func (client *Client) getJSON(ctx context.Context, requestURL string, result any) error {
	for attempt := 1; attempt <= 2; attempt++ {
		token, err := client.getToken(ctx)
		if err != nil {
			return err
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		request.Header.Set("accept", "application/json")
		request.Header.Set("x-verkada-auth", token)

		response, err := client.httpClient.Do(request)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}

		if response.StatusCode == http.StatusUnauthorized && attempt == 1 {
			response.Body.Close()
			client.forgetToken()

			continue
		}

		if response.StatusCode != http.StatusOK {
			errorBody := readErrorBody(response)
			response.Body.Close()

			return fmt.Errorf("verkada API returned HTTP status %s: %s", response.Status, errorBody)
		}

		err = json.NewDecoder(response.Body).Decode(result)
		response.Body.Close()

		if err != nil {
			return fmt.Errorf("decode response: %w", err)
		}

		return nil
	}

	return fmt.Errorf("verkada API rejected a new token (HTTP 401)")
}

// readErrorBody returns the start of an error response. Verkada puts the reason for
// the error in the body (for example "Token expired"), so we add it to the error message.
func readErrorBody(response *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	if err != nil {
		return "(could not read response body)"
	}

	return string(body)
}
