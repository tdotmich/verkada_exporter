package verkada

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeVerkada is a fake Verkada API. It counts the calls, so the tests can check them.
type fakeVerkada struct {
	t               *testing.T
	tokenCalls      int
	expireFirstCall bool
}

func (fake *fakeVerkada) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/token":
		if request.Method != http.MethodPost || request.Header.Get("x-api-key") != "test-api-key" {
			http.Error(writer, `{"message":"bad api key"}`, http.StatusUnauthorized)
			return
		}

		fake.tokenCalls++
		fmt.Fprintf(writer, `{"token":"token-%d"}`, fake.tokenCalls)

	case "/cameras/v1/devices":
		// The API key must only go to /token.
		if request.Header.Get("x-api-key") != "" {
			fake.t.Errorf("the API key was sent to %s", request.URL.Path)
		}

		if fake.expireFirstCall && request.Header.Get("x-verkada-auth") == "token-1" {
			http.Error(writer, `{"message":"Token expired"}`, http.StatusUnauthorized)
			return
		}

		// Page 1 points to page 2. Page 2 is the last page.
		if request.URL.Query().Get("page_token") == "" {
			fmt.Fprint(writer, `{"cameras":[{"camera_id":"cam-1","name":"Front Door","status":"Live"}],"next_page_token":"page-2"}`)
			return
		}

		fmt.Fprint(writer, `{"cameras":[{"camera_id":"cam-2","name":"Car Park","status":"Offline"}],"next_page_token":""}`)

	default:
		http.NotFound(writer, request)
	}
}

func TestListCamerasReadsAllPages(t *testing.T) {
	fake := &fakeVerkada{t: t}
	server := httptest.NewServer(fake)
	defer server.Close()

	client := NewClient(server.URL, "test-api-key", 5*time.Second)

	cameras, err := client.ListCameras(context.Background())
	if err != nil {
		t.Fatalf("ListCameras returned an unexpected error: %v", err)
	}

	if len(cameras) != 2 {
		t.Fatalf("expected 2 cameras, got %d", len(cameras))
	}

	if !cameras[0].IsOnline() || cameras[1].IsOnline() {
		t.Errorf("expected cam-1 online and cam-2 offline, got %+v", cameras)
	}
}

func TestTokenIsReused(t *testing.T) {
	fake := &fakeVerkada{t: t}
	server := httptest.NewServer(fake)
	defer server.Close()

	client := NewClient(server.URL, "test-api-key", 5*time.Second)

	for i := 0; i < 3; i++ {
		_, err := client.ListCameras(context.Background())
		if err != nil {
			t.Fatalf("ListCameras returned an unexpected error: %v", err)
		}
	}

	if fake.tokenCalls != 1 {
		t.Errorf("expected 1 token request for 3 scrapes, got %d", fake.tokenCalls)
	}
}

func TestExpiredTokenIsReplaced(t *testing.T) {
	fake := &fakeVerkada{t: t, expireFirstCall: true}
	server := httptest.NewServer(fake)
	defer server.Close()

	client := NewClient(server.URL, "test-api-key", 5*time.Second)

	_, err := client.ListCameras(context.Background())
	if err != nil {
		t.Fatalf("ListCameras returned an unexpected error: %v", err)
	}

	if fake.tokenCalls != 2 {
		t.Errorf("expected a second token request after HTTP 401, got %d token requests", fake.tokenCalls)
	}
}

func TestWrongAPIKey(t *testing.T) {
	server := httptest.NewServer(&fakeVerkada{t: t})
	defer server.Close()

	client := NewClient(server.URL, "wrong-key", 5*time.Second)

	_, err := client.ListCameras(context.Background())
	if err == nil {
		t.Fatal("expected an error for a wrong API key")
	}
}

// TestRedirectDoesNotSendAPIKey checks that a redirect cannot send the API key to another server.
func TestRedirectDoesNotSendAPIKey(t *testing.T) {
	otherServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "" {
			t.Error("the API key was sent to another server after a redirect")
		}
	}))
	defer otherServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, otherServer.URL+"/token", http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-api-key", 5*time.Second)

	_, err := client.ListCameras(context.Background())
	if err == nil {
		t.Fatal("expected an error for a redirect")
	}
}
