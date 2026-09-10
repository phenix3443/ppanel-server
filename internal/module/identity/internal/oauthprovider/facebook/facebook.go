package facebook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/perfect-panel/server/pkg/logger"
	"golang.org/x/oauth2"
)

// Endpoint pins a current Graph API version; the constants shipped with
// golang.org/x/oauth2/facebook still point at the retired v3.2 dialog.
var Endpoint = oauth2.Endpoint{
	AuthURL:  "https://www.facebook.com/v22.0/dialog/oauth",
	TokenURL: "https://graph.facebook.com/v22.0/oauth/access_token",
}

// userInfoURL is a variable so tests can point the client at a stub server.
var userInfoURL = "https://graph.facebook.com/v22.0/me"

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type Client struct {
	*oauth2.Config
}

// UserInfo is the subset of the Graph API "me" node the login flow needs.
type UserInfo struct {
	OpenID  string
	Name    string
	Email   string
	Picture string
}

func New(config *Config) *Client {
	return &Client{
		&oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RedirectURL:  config.RedirectURL,
			Scopes:       []string{"email", "public_profile"},
			Endpoint:     Endpoint,
		},
	}
}

// GetUserInfo fetches the user profile from the Graph API. Facebook only
// returns the email field when the account has a confirmed address and the
// user granted the email permission, so a non-empty value is verified.
func (c *Client) GetUserInfo(token string) (*UserInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := url.Values{}
	query.Set("fields", "id,name,email,picture.type(large)")
	// appsecret_proof is mandatory when the app enables "Require App
	// Secret" and harmless otherwise.
	query.Set("appsecret_proof", c.appSecretProof(token))

	client := c.Config.Client(ctx, &oauth2.Token{AccessToken: token})
	resp, err := client.Get(userInfoURL + "?" + query.Encode())
	if err != nil {
		logger.Error("[Facebook OAuth 2.0] Get User Info", logger.Field("error", err.Error()))
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("[Facebook OAuth 2.0] Get User Info unexpected status",
			logger.Field("status", resp.StatusCode))
		return nil, fmt.Errorf("facebook graph api returned status %d", resp.StatusCode)
	}

	var raw struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Email   string `json:"email"`
		Picture struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		logger.Error("[Facebook OAuth 2.0] Decode User Info", logger.Field("error", err.Error()))
		return nil, err
	}
	if raw.ID == "" {
		return nil, fmt.Errorf("facebook graph api returned no user id")
	}

	return &UserInfo{
		OpenID:  raw.ID,
		Name:    raw.Name,
		Email:   raw.Email,
		Picture: raw.Picture.Data.URL,
	}, nil
}

// appSecretProof signs the access token with the app secret as required by
// Graph API calls from servers.
// Ref: https://developers.facebook.com/docs/graph-api/securing-requests
func (c *Client) appSecretProof(token string) string {
	mac := hmac.New(sha256.New, []byte(c.ClientSecret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
