package riot

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	riotAuthURL   = "https://auth.riotgames.com/api/v1/authorization"
	riotUserAgent = "RiotClient/96.0.0.0 rso-auth (Windows;10;;Professional, x64)"
)

type sessionCookies struct {
	ssid string
	csid string
	clid string
	sub  string
	asid string
	ccid string
	tdid string
}

// refreshLiveSession parses cookies out of the live RiotGamesPrivateSettings.yaml,
// asks auth.riotgames.com to mint fresh tokens with them, and writes any rotated
// cookies back to the file. Returns (changed, err); changed=false with err=nil
// means the server accepted the cookies but did not rotate any of them.
//
// This mirrors what production Valorant tools do — they don't trust file-swap
// alone, because Riot's RSO rotates ssid on each successful auth and may treat
// a profile's saved cookies as superseded after another profile has logged in
// from the same install. 2FA accounts are notably stricter about this.
func refreshLiveSession() (bool, error) {
	yamlPath, err := localPath(`Riot Games\Riot Client\Data\RiotGamesPrivateSettings.yaml`)("")
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		return false, err
	}
	cookies := parseSessionCookies(string(raw))
	if cookies.ssid == "" {
		return false, errors.New("ssid cookie missing from RiotGamesPrivateSettings.yaml")
	}
	refreshed, err := requestRiotReauth(cookies)
	if err != nil {
		return false, err
	}
	updated := applySessionCookies(string(raw), refreshed)
	if updated == string(raw) {
		return false, nil
	}
	tmp := yamlPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0600); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, yamlPath); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
}

func parseSessionCookies(yaml string) sessionCookies {
	var s sessionCookies

	cookiePat := regexp.MustCompile(`(?m)name:\s*"?(\w+)"?\s*\n(?:\s+\w+:[^\n]*\n)*?\s+value:\s*"([^"]*)"`)
	for _, m := range cookiePat.FindAllStringSubmatch(yaml, -1) {
		assignCookie(&s, m[1], m[2])
	}

	tdidPat := regexp.MustCompile(`(?m)rso-authenticator:\s*\n\s+tdid:\s*\n(?:\s+\w+:[^\n]*\n)*?\s+value:\s*"([^"]*)"`)
	if m := tdidPat.FindStringSubmatch(yaml); len(m) >= 2 {
		s.tdid = m[1]
	}
	return s
}

func assignCookie(s *sessionCookies, name, value string) {
	switch name {
	case "ssid":
		s.ssid = value
	case "csid":
		s.csid = value
	case "clid":
		s.clid = value
	case "sub":
		s.sub = value
	case "asid":
		s.asid = value
	case "ccid":
		s.ccid = value
	case "tdid":
		s.tdid = value
	}
}

func requestRiotReauth(c sessionCookies) (sessionCookies, error) {
	nonce, err := randomNonce()
	if err != nil {
		return sessionCookies{}, err
	}
	body, err := json.Marshal(map[string]any{
		"client_id":     "riot-client",
		"nonce":         nonce,
		"redirect_uri":  "http://localhost/redirect",
		"response_type": "token id_token",
		"scope":         "account openid",
	})
	if err != nil {
		return sessionCookies{}, err
	}
	req, err := http.NewRequest(http.MethodPost, riotAuthURL, strings.NewReader(string(body)))
	if err != nil {
		return sessionCookies{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", riotUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", buildCookieHeader(c))

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return sessionCookies{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return sessionCookies{}, fmt.Errorf("auth server returned %s: %s", resp.Status, strings.TrimSpace(string(preview)))
	}

	var data struct {
		Type     string `json:"type"`
		Response struct {
			Parameters struct {
				URI string `json:"uri"`
			} `json:"parameters"`
		} `json:"response"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return sessionCookies{}, err
	}
	if !strings.EqualFold(data.Type, "response") {
		if data.Error != "" {
			return sessionCookies{}, fmt.Errorf("auth server rejected cookies: %s", data.Error)
		}
		return sessionCookies{}, fmt.Errorf("auth server returned type %q (saved session is no longer valid)", data.Type)
	}

	refreshed := c
	for _, ck := range resp.Cookies() {
		assignCookie(&refreshed, ck.Name, ck.Value)
	}
	return refreshed, nil
}

func buildCookieHeader(c sessionCookies) string {
	var parts []string
	add := func(name, value string) {
		if value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	add("ssid", c.ssid)
	add("clid", c.clid)
	add("sub", c.sub)
	add("csid", c.csid)
	add("tdid", c.tdid)
	add("asid", c.asid)
	add("ccid", c.ccid)
	return strings.Join(parts, "; ")
}

func applySessionCookies(yaml string, c sessionCookies) string {
	result := yaml

	replaceCookie := func(name, value string) {
		if value == "" {
			return
		}
		pat := regexp.MustCompile(fmt.Sprintf(`(?m)(name:\s*"?%s"?\s*\n(?:\s+\w+:[^\n]*\n)*?\s+value:\s*)"[^"]*"`, regexp.QuoteMeta(name)))
		result = pat.ReplaceAllString(result, `${1}"`+escapeRegexReplacement(value)+`"`)
	}
	replaceCookie("ssid", c.ssid)
	replaceCookie("clid", c.clid)
	replaceCookie("csid", c.csid)
	replaceCookie("sub", c.sub)
	replaceCookie("tdid", c.tdid)
	replaceCookie("asid", c.asid)
	replaceCookie("ccid", c.ccid)

	if c.tdid != "" {
		pat := regexp.MustCompile(`(?m)(rso-authenticator:\s*\n\s+tdid:\s*\n(?:\s+\w+:[^\n]*\n)*?\s+value:\s*)"[^"]*"`)
		result = pat.ReplaceAllString(result, `${1}"`+escapeRegexReplacement(c.tdid)+`"`)
	}
	return result
}

func escapeRegexReplacement(s string) string {
	return strings.ReplaceAll(s, "$", "$$")
}

func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
