package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"duolok/bifrost/gateway/internal/auth"
	appErrors "duolok/bifrost/gateway/internal/errors"
	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/pkg/apiutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	oauthGithub "golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	CallbackBaseURL    string
	UIBaseURL          string
}

func (o *OAuthConfig) GoogleEnabled() bool {
	return o.GoogleClientID != "" && o.GoogleClientSecret != ""
}

func (o *OAuthConfig) GitHubEnabled() bool {
	return o.GitHubClientID != "" && o.GitHubClientSecret != ""
}

func (h *Handler) googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.oauth.GoogleClientID,
		ClientSecret: h.oauth.GoogleClientSecret,
		RedirectURL:  h.oauth.CallbackBaseURL + "/api/v1/auth/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

func (h *Handler) githubOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.oauth.GitHubClientID,
		ClientSecret: h.oauth.GitHubClientSecret,
		RedirectURL:  h.oauth.CallbackBaseURL + "/api/v1/auth/github/callback",
		Scopes:       []string{"user:email", "read:user"},
		Endpoint:     oauthGithub.Endpoint,
	}
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) GoogleRedirect(c *gin.Context) {
	if !h.oauth.GoogleEnabled() {
		apiutil.RespondError(c, appErrors.InvalidInput("Google OAuth not configured"))
		return
	}

	state := generateState()
	c.SetCookie("oauth_state", state, 600, "/", "", false, true)

	url := h.googleOAuthConfig().AuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GoogleCallback handles the redirect from Google after the user approves.
func (h *Handler) GoogleCallback(c *gin.Context) {
	if !h.oauth.GoogleEnabled() {
		apiutil.RespondError(c, appErrors.InvalidInput("Google OAuth not configured"))
		return
	}

	// Verify state
	state, err := c.Cookie("oauth_state")
	if err != nil || state != c.Query("state") {
		apiutil.RespondError(c, appErrors.Unauthorized("invalid OAuth state"))
		return
	}

	// Exchange code for token
	code := c.Query("code")
	if code == "" {
		apiutil.RespondError(c, appErrors.InvalidInput("missing authorization code"))
		return
	}

	token, err := h.googleOAuthConfig().Exchange(c.Request.Context(), code)
	if err != nil {
		apiutil.RespondError(c, appErrors.Unauthorized("failed to exchange code: "+err.Error()))
		return
	}

	// Get user info
	client := h.googleOAuthConfig().Client(c.Request.Context(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to get user info", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var googleUser struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &googleUser); err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to parse user info", err))
		return
	}

	if googleUser.Email == "" {
		apiutil.RespondError(c, appErrors.Unauthorized("could not get email from Google"))
		return
	}

	h.completeOAuthLogin(c, googleUser.Email, googleUser.Name, "google")
}

func (h *Handler) GitHubRedirect(c *gin.Context) {
	if !h.oauth.GitHubEnabled() {
		apiutil.RespondError(c, appErrors.InvalidInput("GitHub OAuth not configured"))
		return
	}

	state := generateState()
	c.SetCookie("oauth_state", state, 600, "/", "", false, true)

	url := h.githubOAuthConfig().AuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GitHubCallback handles the redirect from GitHub after the user approves.
func (h *Handler) GitHubCallback(c *gin.Context) {
	if !h.oauth.GitHubEnabled() {
		apiutil.RespondError(c, appErrors.InvalidInput("GitHub OAuth not configured"))
		return
	}

	// Verify state
	state, err := c.Cookie("oauth_state")
	if err != nil || state != c.Query("state") {
		apiutil.RespondError(c, appErrors.Unauthorized("invalid OAuth state"))
		return
	}

	code := c.Query("code")
	if code == "" {
		apiutil.RespondError(c, appErrors.InvalidInput("missing authorization code"))
		return
	}

	token, err := h.githubOAuthConfig().Exchange(c.Request.Context(), code)
	if err != nil {
		apiutil.RespondError(c, appErrors.Unauthorized("failed to exchange code: "+err.Error()))
		return
	}

	client := h.githubOAuthConfig().Client(c.Request.Context(), token)

	// Get user profile
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to get GitHub user", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var ghUser struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	json.Unmarshal(body, &ghUser)

	// GitHub email might be private — fetch from /user/emails
	if ghUser.Email == "" {
		ghUser.Email, err = h.fetchGitHubEmail(c.Request.Context(), client)
		if err != nil {
			apiutil.RespondError(c, appErrors.Unauthorized("could not get email from GitHub"))
			return
		}
	}

	name := ghUser.Name
	if name == "" {
		name = ghUser.Login
	}

	h.completeOAuthLogin(c, ghUser.Email, name, "github")
}

func (h *Handler) fetchGitHubEmail(ctx context.Context, client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	json.Unmarshal(body, &emails)

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no verified email found")
}

// completeOAuthLogin finds or creates a user from an OAuth provider, then returns a JWT.
func (h *Handler) completeOAuthLogin(c *gin.Context, email, name, provider string) {
	ctx := c.Request.Context()

	var userID uuid.UUID
	var teamID uuid.UUID
	var role string

	err := h.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, email,
	).Scan(&userID)

	if err != nil {
		err = h.pool.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name) VALUES ($1, '', $2)
			 ON CONFLICT (email) DO UPDATE SET name = COALESCE(NULLIF(users.name, ''), EXCLUDED.name)
			 RETURNING id`,
			email, name,
		).Scan(&userID)
		if err != nil {
			apiutil.RespondError(c, appErrors.Internal("failed to create user", err))
			return
		}
	}

	err = h.pool.QueryRow(ctx,
		`SELECT team_id, role FROM team_members WHERE user_id = $1 LIMIT 1`,
		userID,
	).Scan(&teamID, &role)

	if err != nil {
		// No team — create one
		teamName := fmt.Sprintf("%s's team", name)
		err = h.pool.QueryRow(ctx,
			`INSERT INTO teams (name) VALUES ($1) RETURNING id`,
			teamName,
		).Scan(&teamID)
		if err != nil {
			teamName = fmt.Sprintf("%s's team (%d)", name, time.Now().Unix())
			err = h.pool.QueryRow(ctx,
				`INSERT INTO teams (name) VALUES ($1) RETURNING id`,
				teamName,
			).Scan(&teamID)
			if err != nil {
				apiutil.RespondError(c, appErrors.Internal("failed to create team", err))
				return
			}
		}

		role = string(models.RoleAdmin)
		_, err = h.pool.Exec(ctx,
			`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)`,
			teamID, userID, role,
		)
		if err != nil {
			apiutil.RespondError(c, appErrors.Internal("failed to add team member", err))
			return
		}

		h.audit(c, auditUserRegister, "user", userID, gin.H{"email": email, "provider": provider})
	}

	jwtToken, err := auth.GenerateJWT(userID, teamID, role, h.jwtSecret)
	if err != nil {
		apiutil.RespondError(c, appErrors.Internal("failed to generate token", err))
		return
	}

	// Redirect to UI with token if UI URL is configured
	if h.oauth.UIBaseURL != "" {
		c.Redirect(http.StatusTemporaryRedirect, h.oauth.UIBaseURL+"/?token="+jwtToken)
		return
	}

	// If there's a redirect_uri query param (for CLI flow), redirect with token
	if redirectURI := c.Query("redirect_uri"); redirectURI != "" {
		c.Redirect(http.StatusTemporaryRedirect, redirectURI+"?token="+jwtToken)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":    jwtToken,
		"user_id":  userID,
		"team_id":  teamID,
		"role":     role,
		"provider": provider,
	})
}
