package alks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Client represents an ALKS client and contains the account info and base url.
type Client struct {
	Credentials    AuthInjecter
	AccountDetails AccountDetails
	BaseURL        string

	http      *http.Client
	userAgent string
}

// LoginRoleResponse represents the response from ALKS containing information about a login role
type LoginRoleResponse struct {
	BaseResponse
	LoginRole LoginRole `json:"loginRole"`
}

// LoginRole represents information about a login role
type LoginRole struct {
	Account        string `json:"account"`
	IamKeyActive   bool   `json:"iamKeyActive"`
	MaxKeyDuration int    `json:"maxKeyDuration"`
	Role           string `json:"role"`
}

// NewClient will create a new instance of the ALKS Client. If you don't yet know the account/role
// pass them as nil and then invoke GetAccounts().
//
// Options (see ClientOption) configure the proxy, the User-Agent and the HTTP client.
func NewClient(url string, username string, password string, account string, role string, opts ...ClientOption) (*Client, error) {
	cfg, err := buildConfig(opts)
	if err != nil {
		return nil, err
	}
	creds := Basic{Username: username, Password: password}

	client := Client{
		Credentials:    &creds,
		AccountDetails: AccountDetails{Account: account, Role: role},
		BaseURL:        url,
		http:           newHTTPClient(cfg),
		userAgent:      cfg.userAgent,
	}

	return &client, nil
}

// NewSTSClient will create a new instance of the ALKS Client using STS tokens.
//
// The constructor makes one request (GET /loginRoles/id/me) to learn the account
// and role. Options, including WithUserAgent and the proxy options, already apply
// to that request.
func NewSTSClient(url string, accessKey string, secretKey string, token string, opts ...ClientOption) (*Client, error) {
	cfg, err := buildConfig(opts)
	if err != nil {
		return nil, err
	}
	creds := STS{AccessKey: accessKey, SecretKey: secretKey, SessionToken: token}
	client := Client{
		Credentials: &creds,
		BaseURL:     url,
		http:        newHTTPClient(cfg),
		userAgent:   cfg.userAgent,
	}

	// Fetch the current login role, and try to populate the account details object.  If we fail, just ignore
	// GetMyLoginRole returns a *AlksError; keep it out of the error variable above,
	// or a nil *AlksError would become a non-nil error interface.
	loginRole, lrErr := client.GetMyLoginRole()
	if lrErr == nil {
		client.AccountDetails.Account = loginRole.LoginRole.Account
		client.AccountDetails.Role = loginRole.LoginRole.Role
	}

	return &client, nil
}

// NewBearerTokenClient will create a new instance of the ALKS Client using Okta Bearer Token auth.
//
// Options (see ClientOption) configure the proxy, the User-Agent and the HTTP client.
func NewBearerTokenClient(url string, bearerToken string, account string, role string, opts ...ClientOption) (*Client, error) {
	cfg, err := buildConfig(opts)
	if err != nil {
		return nil, err
	}
	creds := Bearer{Token: bearerToken}

	client := Client{
		Credentials:    &creds,
		AccountDetails: AccountDetails{Account: account, Role: role},
		BaseURL:        url,
		http:           newHTTPClient(cfg),
		userAgent:      cfg.userAgent,
	}

	return &client, nil
}

// SetUserAgent sets the client user agent in order to report tool details to ALKS
func (c *Client) SetUserAgent(userAgent string) {
	if userAgent == "" {
		return
	}

	c.userAgent = userAgent
}

// IsUsingSTSCredentials returns a boolean indicating if the client was configured using AWS STS Credentials for authentication
func (c *Client) IsUsingSTSCredentials() bool {
	switch c.Credentials.(type) {
	case *STS:
		return true
	default:
		return false
	}
}

// NewRequest will create a new request object for API requests.
func (c *Client) NewRequest(json []byte, method string, endpoint string) (*http.Request, error) {
	u, err := url.Parse(c.BaseURL + endpoint)

	if err != nil {
		return nil, fmt.Errorf("Error parsing base URL: %s", err)
	}

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(json))

	if err != nil {
		return nil, fmt.Errorf("Error creating request: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	err = c.Credentials.InjectAuth(req)

	if err != nil {
		return nil, fmt.Errorf("Error adding configuring authentication: %s", err)
	}

	// Never log headers or bodies: they carry credentials (Constitution IV).
	debugf("request %s %s", method, u.Path)

	return req, nil
}

// decodeBody will convert a http.Response object to a JSON object.
func decodeBody(resp *http.Response, out interface{}) error {
	// Never log bodies: session responses carry credentials (Constitution IV).
	debugf("response HTTP %d", resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(body, &out); err != nil {
		if resp.StatusCode >= 300 {
			return fmt.Errorf("HTTP Status (%d): %s", resp.StatusCode, err)
		}
		return err
	}

	return nil
}

// Durations will provide the valid session durations
func (c *Client) Durations() ([]int, error) {
	log.Printf("[INFO] Requesting allowed durations from ALKS")

	// Use .../me endpoint for getting durations if using STS credentials
	var path string
	if len(strings.TrimSpace(c.AccountDetails.Account)) > 0 {
		accountID := c.AccountDetails.Account[:12]
		path = fmt.Sprintf("/loginRoles/id/%v/%v", accountID, c.AccountDetails.Role)
	} else {
		path = "/loginRoles/id/me"
	}

	req, err := c.NewRequest(nil, "GET", path)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		durationErr := new(AlksResponseError)
		err = decodeBody(resp, &durationErr)
		if err != nil {
			if reqID := GetRequestID(resp); reqID != "" {
				return nil, fmt.Errorf(ParseErrorReqId, reqID, err)
			}

			return nil, fmt.Errorf(ParseError, err)
		}

		if durationErr.Errors != nil {
			if reqID := GetRequestID(resp); reqID != "" {
				return nil, fmt.Errorf(ErrorStringFull, reqID, resp.StatusCode, durationErr.Errors)
			}

			return nil, fmt.Errorf(ErrorStringNoReqId, resp.StatusCode, durationErr.Errors)
		}

		if reqID := GetRequestID(resp); reqID != "" {
			return nil, fmt.Errorf(ErrorStringOnlyCodeAndReqId, reqID, resp.StatusCode)
		}

		return nil, fmt.Errorf(ErrorStringOnlyCode, resp.StatusCode)
	}

	lrr := new(LoginRoleResponse)
	err = decodeBody(resp, &lrr)
	if err != nil {
		if reqID := GetRequestID(resp); reqID != "" {
			return nil, fmt.Errorf("Error parsing LoginRole response: [%s] %s", reqID, err)
		}

		return nil, fmt.Errorf("Error parsing LoginRole response: %s", err)
	}

	if lrr.RequestFailed() {
		return nil, fmt.Errorf("Error fetching role information: [%s] %s", lrr.BaseResponse.RequestID, strings.Join(lrr.GetErrors(), ", "))
	}

	maxDuration := lrr.LoginRole.MaxKeyDuration
	durations := make([]int, maxDuration)
	for i := 0; i < maxDuration; i++ {
		durations[i] = i + 1
	}
	return durations, nil
}
