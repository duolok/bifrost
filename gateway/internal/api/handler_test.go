package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"duolok/bifrost/gateway/internal/api"
	"duolok/bifrost/gateway/internal/auth"
	"duolok/bifrost/gateway/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testJWTSecret = []byte("test-secret-key-for-testing-only")

func TestMain(m *testing.M) {
	m.Run()
}

type testEnv struct {
	router http.Handler
	pool   *pgxpool.Pool
	token  string
	teamID uuid.UUID
	userID uuid.UUID
}

func setupRouter(t *testing.T) *testEnv {
	t.Helper()
	pool := testutil.SetupTestDB(t)

	// Create a test user and team
	ctx := context.Background()
	var teamID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO teams (name) VALUES ('test-team') RETURNING id`,
	).Scan(&teamID)
	if err != nil {
		t.Fatalf("failed to create test team: %v", err)
	}

	hash, _ := auth.HashPassword("testpassword")
	var userID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name) VALUES ('test@bifrost.dev', $1, 'Test User') RETURNING id`,
		hash,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'admin')`,
		teamID, userID,
	)
	if err != nil {
		t.Fatalf("failed to add team member: %v", err)
	}

	token, err := auth.GenerateJWT(userID, teamID, "admin", testJWTSecret)
	if err != nil {
		t.Fatalf("failed to generate test JWT: %v", err)
	}

	router := api.NewRouter(api.HandlerDeps{
		Pool:      pool,
		JWTSecret: testJWTSecret,
	}, testJWTSecret, pool)

	return &testEnv{
		router: router,
		pool:   pool,
		token:  token,
		teamID: teamID,
		userID: userID,
	}
}

func doJSON(env *testEnv, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	return w
}

// doJSONNoAuth makes a request without auth header (for public routes/webhook tests)
func doJSONNoAuth(router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response JSON: %v\nbody: %s", err, w.Body.String())
	}
	return result
}

func signPayload(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestCheckHealth(t *testing.T) {
	env := setupRouter(t)

	// Health is a public endpoint — no auth needed
	w := doJSONNoAuth(env.router, "GET", "/health", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := parseJSON(t, w)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
	db, ok := body["database"].(map[string]any)
	if !ok {
		t.Fatal("expected database object in health response")
	}
	if db["status"] != "connected" {
		t.Errorf("expected database connected, got %v", db["status"])
	}
}

func TestProtectedRouteRequiresAuth(t *testing.T) {
	env := setupRouter(t)

	w := doJSONNoAuth(env.router, "GET", "/api/v1/projects", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateProject(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name":     "test-project",
		"repo_url": "https://github.com/example/repo",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	body := parseJSON(t, w)
	if body["name"] != "test-project" {
		t.Errorf("expected name test-project, got %v", body["name"])
	}

	if body["status"] != "active" {
		t.Errorf("expected status active, got %v", body["status"])
	}

	if body["default_branch"] != "main" {
		t.Errorf("expected default_branch main, got %v", body["default_branch"])
	}

	if body["webhook_secret"] == nil || body["webhook_secret"] == "" {
		t.Error("expected webhook_secret in creation response")
	}

	if body["id"] == nil {
		t.Error("expected id in response")
	}
}

func TestCreateProjectDuplicate(t *testing.T) {
	env := setupRouter(t)

	payload := map[string]string{
		"name":     "dup-project",
		"repo_url": "https://github.com/example/dup",
	}

	w1 := doJSON(env, "POST", "/api/v1/project", payload)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", w1.Code)
	}

	w2 := doJSON(env, "POST", "/api/v1/project", payload)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate create: expected 409, got %d: %s", w2.Code, w2.Body.String())
	}

	body := parseJSON(t, w2)
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "ALREADY_EXISTS" {
		t.Errorf("expected ALREADY_EXISTS, got %v", errObj["code"])
	}
}

func TestCreateProjectValidation(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "POST", "/api/v1/project", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name":     "bad-url",
		"repo_url": "not-a-url",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid URL, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListProjects(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "GET", "/api/v1/projects", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := parseJSON(t, w)
	projects := body["projects"].([]any)
	if len(projects) != 0 {
		t.Errorf("expected empty list, got %d", len(projects))
	}

	doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name": "proj-a", "repo_url": "https://github.com/example/a",
	})

	doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name": "proj-b", "repo_url": "https://github.com/example/b",
	})

	w = doJSON(env, "GET", "/api/v1/projects", nil)
	body = parseJSON(t, w)
	projects = body["projects"].([]any)
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestListProjectsExcludesArchived(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name": "to-archive", "repo_url": "https://github.com/example/archive",
	})
	body := parseJSON(t, w)
	projectID := body["id"].(string)

	doJSON(env, "DELETE", "/api/v1/project/"+projectID, nil)

	w = doJSON(env, "GET", "/api/v1/projects", nil)
	body = parseJSON(t, w)
	projects := body["projects"].([]any)
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after archive, got %d", len(projects))
	}
}

func TestGetProject(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name": "get-me", "repo_url": "https://github.com/example/get",
	})
	created := parseJSON(t, w)
	projectID := created["id"].(string)

	w = doJSON(env, "GET", "/api/v1/project/"+projectID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := parseJSON(t, w)
	if body["name"] != "get-me" {
		t.Errorf("expected name get-me, got %v", body["name"])
	}
	// webhook_secret should NOT be in GET response
	if body["webhook_secret"] != nil {
		t.Error("webhook_secret should not be exposed in GET")
	}
}

func TestGetProjectNotFound(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "GET", "/api/v1/project/00000000-0000-0000-0000-000000000000", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetProjectInvalidID(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "GET", "/api/v1/project/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteProject(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name": "delete-me", "repo_url": "https://github.com/example/del",
	})
	created := parseJSON(t, w)
	projectID := created["id"].(string)

	w = doJSON(env, "DELETE", "/api/v1/project/"+projectID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Should still be visible via GET but status=archived
	w = doJSON(env, "GET", "/api/v1/project/"+projectID, nil)
	body := parseJSON(t, w)
	if body["status"] != "archived" {
		t.Errorf("expected status archived, got %v", body["status"])
	}

	// Double delete should 404
	w = doJSON(env, "DELETE", "/api/v1/project/"+projectID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("double delete: expected 404, got %d", w.Code)
	}
}

func createTestProject(t *testing.T, env *testEnv) (projectID, webhookSecret string) {
	t.Helper()
	w := doJSON(env, "POST", "/api/v1/project", map[string]string{
		"name": "deploy-test-" + t.Name(), "repo_url": "https://github.com/example/" + t.Name(),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("setup: failed to create project: %d %s", w.Code, w.Body.String())
	}
	body := parseJSON(t, w)
	return body["id"].(string), body["webhook_secret"].(string)
}

func TestTriggerDeploy(t *testing.T) {
	env := setupRouter(t)
	projectID, _ := createTestProject(t, env)

	w := doJSON(env, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "abc1234",
		"branch":     "main",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	body := parseJSON(t, w)
	if body["status"] != "queued" {
		t.Errorf("expected status queued, got %v", body["status"])
	}
	if body["commit_sha"] != "abc1234" {
		t.Errorf("expected commit_sha abc1234, got %v", body["commit_sha"])
	}
	if body["triggered_by"] != "api" {
		t.Errorf("expected triggered_by api, got %v", body["triggered_by"])
	}
}

func TestTriggerDeployDuplicate(t *testing.T) {
	env := setupRouter(t)
	projectID, _ := createTestProject(t, env)

	payload := map[string]string{"commit_sha": "dup12345", "branch": "main"}
	w1 := doJSON(env, "POST", "/api/v1/projects/"+projectID+"/deploy", payload)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first deploy: expected 201, got %d", w1.Code)
	}

	w2 := doJSON(env, "POST", "/api/v1/projects/"+projectID+"/deploy", payload)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate deploy: expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestTriggerDeployProjectNotFound(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "POST", "/api/v1/projects/00000000-0000-0000-0000-000000000000/deploy", map[string]string{
		"commit_sha": "abc1234",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTriggerDeployArchivedProject(t *testing.T) {
	env := setupRouter(t)
	projectID, _ := createTestProject(t, env)

	doJSON(env, "DELETE", "/api/v1/project/"+projectID, nil)

	w := doJSON(env, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "abc1234",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for archived project, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDeployments(t *testing.T) {
	env := setupRouter(t)
	projectID, _ := createTestProject(t, env)

	// Empty list
	w := doJSON(env, "GET", "/api/v1/projects/"+projectID+"/deployments", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := parseJSON(t, w)
	deployments := body["deployments"].([]any)
	if len(deployments) != 0 {
		t.Errorf("expected empty list, got %d", len(deployments))
	}

	// Create two deployments
	doJSON(env, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "aaa1111", "branch": "main",
	})
	doJSON(env, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "bbb2222", "branch": "feat/x",
	})

	w = doJSON(env, "GET", "/api/v1/projects/"+projectID+"/deployments", nil)
	body = parseJSON(t, w)
	deployments = body["deployments"].([]any)
	if len(deployments) != 2 {
		t.Errorf("expected 2 deployments, got %d", len(deployments))
	}
}

func TestGetDeployment(t *testing.T) {
	env := setupRouter(t)
	projectID, _ := createTestProject(t, env)

	w := doJSON(env, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "get12345", "branch": "main",
	})
	created := parseJSON(t, w)
	deployID := created["id"].(string)

	w = doJSON(env, "GET", "/api/v1/deployments/"+deployID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := parseJSON(t, w)
	if body["commit_sha"] != "get12345" {
		t.Errorf("expected commit_sha get12345, got %v", body["commit_sha"])
	}
}

func TestGetDeploymentNotFound(t *testing.T) {
	env := setupRouter(t)

	w := doJSON(env, "GET", "/api/v1/deployments/00000000-0000-0000-0000-000000000000", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRegisterAndLogin(t *testing.T) {
	env := setupRouter(t)

	// Register
	w := doJSONNoAuth(env.router, "POST", "/api/v1/auth/register", map[string]string{
		"email":    "new@bifrost.dev",
		"password": "securepass123",
		"name":     "New User",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	body := parseJSON(t, w)
	if body["token"] == nil || body["token"] == "" {
		t.Error("expected token in register response")
	}
	if body["role"] != "admin" {
		t.Errorf("expected role admin, got %v", body["role"])
	}

	// Login
	w = doJSONNoAuth(env.router, "POST", "/api/v1/auth/login", map[string]string{
		"email":    "new@bifrost.dev",
		"password": "securepass123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body = parseJSON(t, w)
	if body["token"] == nil || body["token"] == "" {
		t.Error("expected token in login response")
	}

	// Wrong password
	w = doJSONNoAuth(env.router, "POST", "/api/v1/auth/login", map[string]string{
		"email":    "new@bifrost.dev",
		"password": "wrongpassword",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: expected 401, got %d", w.Code)
	}
}

func TestRBACViewerCannotDeploy(t *testing.T) {
	env := setupRouter(t)

	// Create a viewer-scoped token
	viewerToken, _ := auth.GenerateJWT(env.userID, env.teamID, "viewer", testJWTSecret)

	// Create project with admin token first
	projectID, _ := createTestProject(t, env)

	// Try to deploy with viewer token
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]string{
		"commit_sha": "abc1234",
		"branch":     "main",
	})
	req := httptest.NewRequest("POST", "/api/v1/projects/"+projectID+"/deploy", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer deploy: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitHubWebhook(t *testing.T) {
	env := setupRouter(t)
	projectID, secret := createTestProject(t, env)
	_ = projectID

	// Get the repo_url we used
	repoURL := "https://github.com/example/" + t.Name()

	payload := map[string]any{
		"ref":   "refs/heads/main",
		"after": "deadbeef1234567",
		"repository": map[string]string{
			"clone_url": repoURL,
			"html_url":  repoURL,
		},
		"pusher": map[string]string{
			"name": "testuser",
		},
	}
	body, _ := json.Marshal(payload)
	sig := signPayload(secret, body)

	req := httptest.NewRequest("POST", "/api/v1/webhook/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := parseJSON(t, w)
	if resp["status"] != "accepted" {
		t.Errorf("expected status accepted, got %v", resp["status"])
	}
	if resp["deployment_id"] == nil {
		t.Error("expected deployment_id in response")
	}
}

func TestGitHubWebhookInvalidSignature(t *testing.T) {
	env := setupRouter(t)
	createTestProject(t, env)

	repoURL := "https://github.com/example/" + t.Name()

	payload := map[string]any{
		"ref":   "refs/heads/main",
		"after": "deadbeef1234567",
		"repository": map[string]string{
			"clone_url": repoURL,
		},
		"pusher": map[string]string{"name": "attacker"},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/v1/webhook/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=0000000000000000000000000000000000000000000000000000000000000000")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitHubWebhookDuplicate(t *testing.T) {
	env := setupRouter(t)
	_, secret := createTestProject(t, env)

	repoURL := "https://github.com/example/" + t.Name()

	payload := map[string]any{
		"ref":   "refs/heads/main",
		"after": "samecommit12345",
		"repository": map[string]string{
			"clone_url": repoURL,
		},
		"pusher": map[string]string{"name": "dev"},
	}
	body, _ := json.Marshal(payload)
	sig := signPayload(secret, body)

	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/webhook/github", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hub-Signature-256", sig)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		return w
	}

	w1 := send()
	if w1.Code != http.StatusOK {
		t.Fatalf("first webhook: expected 200, got %d", w1.Code)
	}

	w2 := send()
	if w2.Code != http.StatusOK {
		t.Fatalf("duplicate webhook: expected 200, got %d", w2.Code)
	}
	resp := parseJSON(t, w2)
	if resp["status"] != "already_processed" {
		t.Errorf("expected already_processed, got %v", resp["status"])
	}
}

func TestGitHubWebhookUnknownRepo(t *testing.T) {
	env := setupRouter(t)

	payload := map[string]any{
		"ref":   "refs/heads/main",
		"after": "abc1234",
		"repository": map[string]string{
			"clone_url": "https://github.com/unknown/repo",
		},
		"pusher": map[string]string{"name": "x"},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/v1/webhook/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=fake")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
