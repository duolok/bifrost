package k8s

import (
	"context"
	"testing"

	"duolok/bifrost/gateway/internal/models"
	"duolok/bifrost/gateway/internal/testutil"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDeploySuccess(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO projects (name, repo_url) VALUES ($1, $2) RETURNING id`,
		"deploy-ok", "https://github.com/example/deploy-ok",
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	imageURI := "us-docker.pkg.dev/bifrost/apps/deploy-ok:abc1234"
	var deployID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO deployments (project_id, commit_sha, branch, triggered_by, status, image_uri)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		projectID, "abc1234def", "main", "test", models.StatusBuilt, imageURI,
	).Scan(&deployID)
	if err != nil {
		t.Fatalf("seed deployment: %v", err)
	}

	fakeClient := fake.NewSimpleClientset()
	deployer := NewDeployerWithClient(fakeClient, pool, "bifrost-apps")

	project := models.Project{ID: projectID, Name: "deploy-ok"}
	deployment := models.Deployment{
		ID:        deployID,
		ProjectID: projectID,
		CommitSHA: "abc1234def",
		ImageURI:  &imageURI,
		Status:    models.StatusBuilt,
	}

	if err := deployer.Deploy(ctx, deployment, project); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	// Verify DB status transitioned to running
	var status models.DeploymentStatus
	var statusMessage *string
	err = pool.QueryRow(ctx,
		`SELECT status, status_message FROM deployments WHERE id = $1`, deployID,
	).Scan(&status, &statusMessage)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != models.StatusRunning {
		t.Errorf("expected status running, got %s", status)
	}
	if statusMessage == nil || *statusMessage != "Manifests applied" {
		t.Errorf("unexpected status message: %v", statusMessage)
	}

	// Verify deploy timestamps are set
	var deployStarted, deployFinished bool
	err = pool.QueryRow(ctx,
		`SELECT deploy_started_at IS NOT NULL, deploy_finished_at IS NOT NULL FROM deployments WHERE id = $1`, deployID,
	).Scan(&deployStarted, &deployFinished)
	if err != nil {
		t.Fatalf("query timestamps: %v", err)
	}
	if !deployStarted {
		t.Error("deploy_started_at should be set")
	}
	if !deployFinished {
		t.Error("deploy_finished_at should be set")
	}

	// Verify K8s resources were created
	dep, err := fakeClient.AppsV1().Deployments("bifrost-apps").Get(ctx, "bifrost-deploy-ok", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("k8s deployment not found: %v", err)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != imageURI {
		t.Errorf("expected image %s, got %s", imageURI, dep.Spec.Template.Spec.Containers[0].Image)
	}

	svc, err := fakeClient.CoreV1().Services("bifrost-apps").Get(ctx, "bifrost-deploy-ok", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("k8s service not found: %v", err)
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("expected service port 80, got %d", svc.Spec.Ports[0].Port)
	}
}

func TestDeployInvalidTransition(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	fakeClient := fake.NewSimpleClientset()
	deployer := NewDeployerWithClient(fakeClient, pool, "bifrost-apps")

	deployment := models.Deployment{
		ID:     uuid.New(),
		Status: models.StatusQueued,
	}
	project := models.Project{Name: "test"}

	err := deployer.Deploy(ctx, deployment, project)
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}
}

func TestDeployUpdatesExistingResources(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	ctx := context.Background()

	var projectID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO projects (name, repo_url) VALUES ($1, $2) RETURNING id`,
		"redeploy", "https://github.com/example/redeploy",
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	imageURI1 := "registry/app:v1"
	imageURI2 := "registry/app:v2"

	var deployID1 uuid.UUID
	pool.QueryRow(ctx,
		`INSERT INTO deployments (project_id, commit_sha, status, image_uri) VALUES ($1, $2, $3, $4) RETURNING id`,
		projectID, "commit1", models.StatusBuilt, imageURI1,
	).Scan(&deployID1)

	fakeClient := fake.NewSimpleClientset()
	deployer := NewDeployerWithClient(fakeClient, pool, "default")

	project := models.Project{ID: projectID, Name: "redeploy"}

	d1 := models.Deployment{ID: deployID1, ProjectID: projectID, CommitSHA: "commit1", ImageURI: &imageURI1, Status: models.StatusBuilt}
	if err := deployer.Deploy(ctx, d1, project); err != nil {
		t.Fatalf("first deploy: %v", err)
	}

	var deployID2 uuid.UUID
	pool.QueryRow(ctx,
		`INSERT INTO deployments (project_id, commit_sha, status, image_uri) VALUES ($1, $2, $3, $4) RETURNING id`,
		projectID, "commit2", models.StatusBuilt, imageURI2,
	).Scan(&deployID2)

	d2 := models.Deployment{ID: deployID2, ProjectID: projectID, CommitSHA: "commit2", ImageURI: &imageURI2, Status: models.StatusBuilt}
	if err := deployer.Deploy(ctx, d2, project); err != nil {
		t.Fatalf("second deploy: %v", err)
	}

	dep, _ := fakeClient.AppsV1().Deployments("default").Get(ctx, "bifrost-redeploy", metav1.GetOptions{})
	if dep.Spec.Template.Spec.Containers[0].Image != imageURI2 {
		t.Errorf("expected image %s after update, got %s", imageURI2, dep.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestDeleteIdempotent(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	deployer := NewDeployerWithClient(fakeClient, nil, "default")

	err := deployer.Delete(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("expected no error for deleting nonexistent resource, got: %v", err)
	}
}
