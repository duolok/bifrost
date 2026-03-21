package api_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"duolok/bifrost/gateway/internal/api"
	"duolok/bifrost/gateway/internal/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	m.Run()
}

func setupRouter(t *testing.T) http.Handler {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testPool = pool
	return api.NewRouter(pool, nil, nil, nil)
}

func doJSON(router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
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
	router := setupRouter(t)

	w := doJSON(router, "GET", "/health", nil)

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

func TestCreateProject(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "POST", "/api/v1/project", map[string]string{
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
	router := setupRouter(t)

	payload := map[string]string{
		"name":     "dup-project",
		"repo_url": "https://github.com/example/dup",
	}

	w1 := doJSON(router, "POST", "/api/v1/project", payload)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", w1.Code)
	}

	w2 := doJSON(router, "POST", "/api/v1/project", payload)
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
	router := setupRouter(t)

	w := doJSON(router, "POST", "/api/v1/project", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(router, "POST", "/api/v1/project", map[string]string{
		"name":     "bad-url",
		"repo_url": "not-a-url",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid URL, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListProjects(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "GET", "/api/v1/projects", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := parseJSON(t, w)
	projects := body["projects"].([]any)
	if len(projects) != 0 {
		t.Errorf("expected empty list, got %d", len(projects))
	}

	doJSON(router, "POST", "/api/v1/project", map[string]string{
		"name": "proj-a", "repo_url": "https://github.com/example/a",
	})

	doJSON(router, "POST", "/api/v1/project", map[string]string{
		"name": "proj-b", "repo_url": "https://github.com/example/b",
	})

	w = doJSON(router, "GET", "/api/v1/projects", nil)
	body = parseJSON(t, w)
	projects = body["projects"].([]any)
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestListProjectsExcludesArchived(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "POST", "/api/v1/project", map[string]string{
		"name": "to-archive", "repo_url": "https://github.com/example/archive",
	})
	body := parseJSON(t, w)
	projectID := body["id"].(string)

	doJSON(router, "DELETE", "/api/v1/project/"+projectID, nil)

	w = doJSON(router, "GET", "/api/v1/projects", nil)
	body = parseJSON(t, w)
	projects := body["projects"].([]any)
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after archive, got %d", len(projects))
	}
}

func TestGetProject(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "POST", "/api/v1/project", map[string]string{
		"name": "get-me", "repo_url": "https://github.com/example/get",
	})
	created := parseJSON(t, w)
	projectID := created["id"].(string)

	w = doJSON(router, "GET", "/api/v1/project/"+projectID, nil)
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
	router := setupRouter(t)

	w := doJSON(router, "GET", "/api/v1/project/00000000-0000-0000-0000-000000000000", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetProjectInvalidID(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "GET", "/api/v1/project/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteProject(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "POST", "/api/v1/project", map[string]string{
		"name": "delete-me", "repo_url": "https://github.com/example/del",
	})
	created := parseJSON(t, w)
	projectID := created["id"].(string)

	w = doJSON(router, "DELETE", "/api/v1/project/"+projectID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Should still be visible via GET but status=archived
	w = doJSON(router, "GET", "/api/v1/project/"+projectID, nil)
	body := parseJSON(t, w)
	if body["status"] != "archived" {
		t.Errorf("expected status archived, got %v", body["status"])
	}

	// Double delete should 404
	w = doJSON(router, "DELETE", "/api/v1/project/"+projectID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("double delete: expected 404, got %d", w.Code)
	}
}

func createTestProject(t *testing.T, router http.Handler) (projectID, webhookSecret string) {
	t.Helper()
	w := doJSON(router, "POST", "/api/v1/project", map[string]string{
		"name": "deploy-test-" + t.Name(), "repo_url": "https://github.com/example/" + t.Name(),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("setup: failed to create project: %d %s", w.Code, w.Body.String())
	}
	body := parseJSON(t, w)
	return body["id"].(string), body["webhook_secret"].(string)
}

func TestTriggerDeploy(t *testing.T) {
	router := setupRouter(t)
	projectID, _ := createTestProject(t, router)

	w := doJSON(router, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
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
	router := setupRouter(t)
	projectID, _ := createTestProject(t, router)

	payload := map[string]string{"commit_sha": "dup12345", "branch": "main"}
	w1 := doJSON(router, "POST", "/api/v1/projects/"+projectID+"/deploy", payload)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first deploy: expected 201, got %d", w1.Code)
	}

	w2 := doJSON(router, "POST", "/api/v1/projects/"+projectID+"/deploy", payload)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate deploy: expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestTriggerDeployProjectNotFound(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "POST", "/api/v1/projects/00000000-0000-0000-0000-000000000000/deploy", map[string]string{
		"commit_sha": "abc1234",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTriggerDeployArchivedProject(t *testing.T) {
	router := setupRouter(t)
	projectID, _ := createTestProject(t, router)

	doJSON(router, "DELETE", "/api/v1/project/"+projectID, nil)

	w := doJSON(router, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "abc1234",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for archived project, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDeployments(t *testing.T) {
	router := setupRouter(t)
	projectID, _ := createTestProject(t, router)

	// Empty list
	w := doJSON(router, "GET", "/api/v1/projects/"+projectID+"/deployments", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := parseJSON(t, w)
	deployments := body["deployments"].([]any)
	if len(deployments) != 0 {
		t.Errorf("expected empty list, got %d", len(deployments))
	}

	// Create two deployments
	doJSON(router, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "aaa1111", "branch": "main",
	})
	doJSON(router, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "bbb2222", "branch": "feat/x",
	})

	w = doJSON(router, "GET", "/api/v1/projects/"+projectID+"/deployments", nil)
	body = parseJSON(t, w)
	deployments = body["deployments"].([]any)
	if len(deployments) != 2 {
		t.Errorf("expected 2 deployments, got %d", len(deployments))
	}
}

func TestGetDeployment(t *testing.T) {
	router := setupRouter(t)
	projectID, _ := createTestProject(t, router)

	w := doJSON(router, "POST", "/api/v1/projects/"+projectID+"/deploy", map[string]string{
		"commit_sha": "get12345", "branch": "main",
	})
	created := parseJSON(t, w)
	deployID := created["id"].(string)

	w = doJSON(router, "GET", "/api/v1/deployments/"+deployID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := parseJSON(t, w)
	if body["commit_sha"] != "get12345" {
		t.Errorf("expected commit_sha get12345, got %v", body["commit_sha"])
	}
}

func TestGetDeploymentNotFound(t *testing.T) {
	router := setupRouter(t)

	w := doJSON(router, "GET", "/api/v1/deployments/00000000-0000-0000-0000-000000000000", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGitHubWebhook(t *testing.T) {
	router := setupRouter(t)
	projectID, secret := createTestProject(t, router)
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
	router.ServeHTTP(w, req)

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
	router := setupRouter(t)
	createTestProject(t, router)

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
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitHubWebhookDuplicate(t *testing.T) {
	router := setupRouter(t)
	_, secret := createTestProject(t, router)

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
		router.ServeHTTP(w, req)
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
	router := setupRouter(t)

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
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
