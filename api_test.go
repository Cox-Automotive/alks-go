package alks

import (
	"testing"
)

func makeClient(t *testing.T) *Client {
	client, err := NewClient("http://foo.bar.com", "brian", "pass", "acct", "role")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if client.BaseURL != "http://foo.bar.com" {
		t.Fatalf("base url not set on client: %s", client.BaseURL)
	}

	if client.Credentials == nil {
		t.Fatalf("credentials not set on client")
	}

	if client.AccountDetails.Account != "acct" {
		t.Fatalf("account account not set on client: %s", client.AccountDetails.Account)
	}

	if client.AccountDetails.Role != "role" {
		t.Fatalf("account role not set on client: %s", client.AccountDetails.Role)
	}

	return client
}

func TestClient_NewRequest(t *testing.T) {
	c := makeClient(t)
	c.SetUserAgent("test-value")

	json := []byte(`{"fooz":"barz"}`)

	req, err := c.NewRequest(json, "POST", "/endpointfun")
	if err != nil {
		t.Fatalf("bad: %v", err)
	}

	if req.URL.String() != "http://foo.bar.com/endpointfun" {
		t.Fatalf("bad base url: %v", req.URL.String())
	}

	if req.UserAgent() != "test-value" {
		t.Fatalf("bad user-agent: %v", req.UserAgent())
	}

	if req.Method != "POST" {
		t.Fatalf("bad method: %v", req.Method)
	}

	if _, _, ok := req.BasicAuth(); !ok {
		t.Fatalf("basic auth header missing")
	}
}

// TODO: tests for STS functionality
