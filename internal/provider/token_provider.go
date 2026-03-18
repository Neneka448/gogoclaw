package provider

// TokenProvider supplies Codex authentication credentials at request time.
type TokenProvider interface {
	GetToken() (accessToken string, accountID string, err error)
}
