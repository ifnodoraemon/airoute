package controlplane

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// exchangeGitHubOAuth exchanges an authorization code for GitHub user profile info.
func exchangeGitHubOAuth(code, clientID, clientSecret string) (string, string, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)

	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", err
	}
	if tokenResp.Error != "" {
		return "", "", fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return "", "", fmt.Errorf("empty access token returned")
	}

	userReq, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userReq.Header.Set("User-Agent", "Airoute-Gateway")

	userResp, err := client.Do(userReq)
	if err != nil {
		return "", "", err
	}
	defer userResp.Body.Close()

	var ghUser struct {
		Login string `json:"login"`
		Email string `json:"email"`
		ID    int64  `json:"id"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ghUser); err != nil {
		return "", "", err
	}

	if userResp.StatusCode != http.StatusOK || ghUser.ID <= 0 {
		return "", "", fmt.Errorf("invalid GitHub identity")
	}
	username := fmt.Sprintf("%d", ghUser.ID)
	// Prefer the primary verified email: the /user public email may be unverified,
	// which would let an attacker register a victim's address.
	email := fetchGitHubVerifiedEmail(client, tokenResp.AccessToken)
	if email == "" {
		email = ghUser.Email
	}
	if email == "" {
		email = fmt.Sprintf("%s@github.oauth.local", username)
	}

	return username, email, nil
}

// fetchGitHubVerifiedEmail returns the account's primary verified email from
// /user/emails, or "" when none is available (the caller then falls back).
func fetchGitHubVerifiedEmail(client *http.Client, token string) string {
	req, err := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Airoute-Gateway")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if json.NewDecoder(resp.Body).Decode(&emails) != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary && e.Verified && e.Email != "" {
			return e.Email
		}
	}
	return ""
}

// exchangeGoogleOAuth exchanges an authorization code for Google user profile info.
func exchangeGoogleOAuth(code, clientID, clientSecret, redirectURI string) (string, string, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", err
	}
	if tokenResp.AccessToken == "" {
		return "", "", fmt.Errorf("empty access token returned: %s", tokenResp.ErrorDesc)
	}

	userReq, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	userResp, err := client.Do(userReq)
	if err != nil {
		return "", "", err
	}
	defer userResp.Body.Close()

	var ggUser struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ggUser); err != nil {
		return "", "", err
	}

	if userResp.StatusCode != http.StatusOK || ggUser.ID == "" {
		return "", "", fmt.Errorf("invalid Google identity")
	}
	return ggUser.ID, ggUser.Email, nil
}
