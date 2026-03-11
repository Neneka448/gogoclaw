package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	codexClientID          = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexAuthorizeURL      = "https://auth.openai.com/oauth/authorize"
	codexTokenURL          = "https://auth.openai.com/oauth/token"
	codexRedirectURL       = "http://localhost:1455/auth/callback"
	codexScope             = "openid profile email offline_access"
	codexJWTClaimPath      = "https://api.openai.com/auth"
	codexAccountIDClaim    = "chatgpt_account_id"
	codexDefaultOriginator = "gogoclaw"
	tokenRefreshLeeway     = 60 * time.Second
)

type CodexOAuthToken struct {
	Access    string `json:"access"`
	Refresh   string `json:"refresh"`
	Expires   int64  `json:"expires"`
	AccountID string `json:"account_id,omitempty"`
}

func AuthCodex() (string, error) {
	if token, err := loadCodexToken(); err == nil && token.Access != "" && token.Expires > time.Now().Add(tokenRefreshLeeway).UnixMilli() {
		return token.Access, nil
	}

	verifier, challenge, err := generatePKCE()
	if err != nil {
		return "", err
	}
	state, err := randomBase64URL(16)
	if err != nil {
		return "", err
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	server := startLocalCallbackServer(state, codeCh, errCh)
	defer shutdownServer(server)

	authURL := buildCodexAuthorizationURL(state, challenge)
	fmt.Println("Open this URL in your browser to authenticate:")
	fmt.Println(authURL)
	openBrowser(authURL)

	code, err := awaitAuthorizationCode(codeCh, errCh, state)
	if err != nil {
		return "", err
	}

	token, err := exchangeCodeForToken(code, verifier)
	if err != nil {
		return "", err
	}
	if err := saveCodexToken(token); err != nil {
		return "", err
	}

	return token.Access, nil
}

func GetCodexToken() (*CodexOAuthToken, error) {
	token, err := loadCodexToken()
	if err != nil {
		return nil, err
	}
	if token.Expires > time.Now().Add(tokenRefreshLeeway).UnixMilli() {
		return token, nil
	}

	refreshed, err := refreshCodexToken(token.Refresh)
	if err != nil {
		latest, reloadErr := loadCodexToken()
		if reloadErr == nil && latest.Expires > time.Now().UnixMilli() {
			return latest, nil
		}
		return nil, err
	}
	if err := saveCodexToken(refreshed); err != nil {
		return nil, err
	}

	return refreshed, nil
}

func loadCodexToken() (*CodexOAuthToken, error) {
	tokenPath, err := codexTokenPath()
	if err != nil {
		return nil, err
	}
	if token, err := readTokenFile(tokenPath); err == nil && token != nil {
		return token, nil
	}

	imported, err := importCodexCLIToken(tokenPath)
	if err != nil {
		return nil, err
	}
	if imported != nil {
		return imported, nil
	}

	return nil, errors.New("codex credentials not found; run `gogoclaw auth --provider codex`")
}

func saveCodexToken(token *CodexOAuthToken) error {
	tokenPath, err := codexTokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tokenPath, encoded, 0600); err != nil {
		return err
	}
	return nil
}

func refreshCodexToken(refreshToken string) (*CodexOAuthToken, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", codexClientID)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return parseOAuthTokenResponse(resp.Body)
}

func exchangeCodeForToken(code string, verifier string) (*CodexOAuthToken, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", codexClientID)
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", codexRedirectURL)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return parseOAuthTokenResponse(resp.Body)
}

func parseOAuthTokenResponse(body io.Reader) (*CodexOAuthToken, error) {
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.AccessToken == "" || payload.RefreshToken == "" || payload.ExpiresIn <= 0 {
		return nil, errors.New("token response missing fields")
	}
	accountID, err := decodeAccountID(payload.AccessToken)
	if err != nil {
		return nil, err
	}

	return &CodexOAuthToken{
		Access:    payload.AccessToken,
		Refresh:   payload.RefreshToken,
		Expires:   time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UnixMilli(),
		AccountID: accountID,
	}, nil
}

func decodeAccountID(accessToken string) (string, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid jwt token")
	}
	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return "", err
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", err
	}
	claim, _ := payload[codexJWTClaimPath].(map[string]any)
	accountID, _ := claim[codexAccountIDClaim].(string)
	if accountID == "" {
		return "", errors.New("account id not found in token")
	}
	return accountID, nil
}

func buildCodexAuthorizationURL(state string, challenge string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", codexClientID)
	params.Set("redirect_uri", codexRedirectURL)
	params.Set("scope", codexScope)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")
	params.Set("originator", codexDefaultOriginator)

	return codexAuthorizeURL + "?" + params.Encode()
}

func awaitAuthorizationCode(codeCh <-chan string, errCh <-chan error, state string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	manualCh := make(chan string, 1)
	go func() {
		fmt.Println("Paste the callback URL or authorization code if the browser does not return automatically:")
		line, _ := reader.ReadString('\n')
		manualCh <- strings.TrimSpace(line)
	}()

	for {
		select {
		case code := <-codeCh:
			return code, nil
		case err := <-errCh:
			return "", err
		case raw := <-manualCh:
			code, parsedState := parseAuthorizationInput(raw)
			if parsedState != "" && parsedState != state {
				return "", errors.New("state validation failed")
			}
			if code == "" {
				return "", errors.New("authorization code not found")
			}
			return code, nil
		}
	}
}

func parseAuthorizationInput(raw string) (string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", ""
	}
	if parsedURL, err := url.Parse(value); err == nil {
		query := parsedURL.Query()
		if code := query.Get("code"); code != "" {
			return code, query.Get("state")
		}
	}
	if strings.Contains(value, "#") {
		parts := strings.SplitN(value, "#", 2)
		return parts[0], parts[1]
	}
	if strings.Contains(value, "code=") {
		query, err := url.ParseQuery(value)
		if err == nil {
			return query.Get("code"), query.Get("state")
		}
	}
	return value, ""
}

func startLocalCallbackServer(state string, codeCh chan<- string, errCh chan<- error) *http.Server {
	listener, err := netListen("tcp", "127.0.0.1:1455")
	if err != nil {
		errCh <- fmt.Errorf("local callback server failed to start: %w", err)
		return nil
	}

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}
	var once sync.Once
	mux.HandleFunc("/auth/callback", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("state") != state {
			http.Error(writer, "State mismatch", http.StatusBadRequest)
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(writer, "Missing code", http.StatusBadRequest)
			return
		}
		once.Do(func() { codeCh <- code })
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte("<html><body><p>Authentication successful. Return to your terminal to continue.</p></body></html>"))
	})

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	return server
}

func shutdownServer(server *http.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func openBrowser(target string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	_ = command.Start()
}

func generatePKCE() (string, string, error) {
	verifier, err := randomBase64URL(32)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomBase64URL(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func decodeBase64URL(value string) ([]byte, error) {
	if mod := len(value) % 4; mod != 0 {
		value += strings.Repeat("=", 4-mod)
	}
	return base64.URLEncoding.DecodeString(value)
}

func codexTokenPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("OAUTH_CLI_KIT_TOKEN_PATH")); override != "" {
		return override, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "gogoclaw", "auth", "codex.json"), nil
}

func importCodexCLIToken(targetPath string) (*CodexOAuthToken, error) {
	codexPath, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(codexPath, ".codex", "auth.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var payload struct {
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, nil
	}
	if payload.Tokens.AccessToken == "" || payload.Tokens.RefreshToken == "" || payload.Tokens.AccountID == "" {
		return nil, nil
	}
	info, err := os.Stat(filepath.Join(codexPath, ".codex", "auth.json"))
	if err != nil {
		return nil, err
	}
	token := &CodexOAuthToken{
		Access:    payload.Tokens.AccessToken,
		Refresh:   payload.Tokens.RefreshToken,
		AccountID: payload.Tokens.AccountID,
		Expires:   info.ModTime().Add(time.Hour).UnixMilli(),
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(targetPath, encoded, 0600); err != nil {
		return nil, err
	}
	return token, nil
}

func readTokenFile(path string) (*CodexOAuthToken, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token CodexOAuthToken
	if err := json.Unmarshal(content, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

var netListen = net.Listen
