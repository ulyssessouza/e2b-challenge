package service

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"

    "e2b-challenge/internal/config"
    "e2b-challenge/internal/db"
)

type AuthService struct {
    q          *db.Queries
    cfg        *config.Config
    httpClient *http.Client
}

func NewAuthService(q *db.Queries, cfg *config.Config) *AuthService {
    return &AuthService{
        q:          q,
        cfg:        cfg,
        httpClient: &http.Client{},
    }
}

type tokenResponse struct {
    AccessToken string `json:"access_token"`
    IDToken     string `json:"id_token"`
}

func (s *AuthService) HydraPublicURL() string  { return s.cfg.HydraPublicURL }
func (s *AuthService) OAuthClientID() string     { return s.cfg.OAuthClientID }
func (s *AuthService) OAuthRedirectURI() string  { return s.cfg.OAuthRedirectURI }

func (s *AuthService) ExchangeCode(ctx context.Context, code string) (string, error) {
    data := url.Values{
        "grant_type":   {"authorization_code"},
        "code":         {code},
        "redirect_uri": {s.cfg.OAuthRedirectURI},
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        s.cfg.HydraPublicURL+"/oauth2/token",
        strings.NewReader(data.Encode()))
    if err != nil {
        return "", fmt.Errorf("creating token request: %w", err)
    }

    req.SetBasicAuth(s.cfg.OAuthClientID, s.cfg.OAuthClientSecret)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("token exchange request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("reading token response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
    }

    var tr tokenResponse
    if err := json.Unmarshal(body, &tr); err != nil {
        return "", fmt.Errorf("parsing token response: %w", err)
    }

    return tr.AccessToken, nil
}

func (s *AuthService) FindOrCreateUser(ctx context.Context, email string) (*db.User, error) {
    user, err := s.q.GetUserByEmail(ctx, email)
    if err == sql.ErrNoRows {
        user, err = s.q.CreateUser(ctx, db.CreateUserParams{
            Email: email,
            Name:  email,
        })
        if err != nil {
            return nil, fmt.Errorf("creating user: %w", err)
        }
        return &user, nil
    }
    if err != nil {
        return nil, fmt.Errorf("looking up user: %w", err)
    }
    return &user, nil
}