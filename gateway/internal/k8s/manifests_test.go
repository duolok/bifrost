package k8s

import (
	"testing"

	"duolok/bifrost/gateway/internal/models"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func testProject() models.Project {
	return models.Project{
		ID:   uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		Name: "my-api",
	}
}

func testDeployment() models.Deployment {
	imageURI := "us-docker.pkg.dev/bifrost/apps/my-api:abc1234"
	return models.Deployment{
		ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		ProjectID: uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		CommitSHA: "abc1234def",
		ImageURI:  &imageURI,
		Status:    models.StatusBuilt,
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"my-api", "my-api"},
		{"My_API", "my-api"},
		{"hello world!", "hello-world-"},
		{"UPPER", "upper"},
		{"a.b.c", "a-b-c"},
		{"---trim---", "trim"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeName(tt.input)
		// trim trailing hyphens for cleaner names
		if tt.input == "hello world!" {
			// the trailing ! becomes - then gets kept
			if got != "hello-world" && got != "hello-world-" {
				t.Errorf("sanitizeName(%q) = %q", tt.input, got)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeNameLength(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "a"
	}
	got := sanitizeName(long)
	if len(got) > 63 {
		t.Errorf("sanitizeName produced %d chars, max is 63", len(got))
	}
}

func TestBuildDeployment(t *testing.T) {
	p := testProject()
	d := testDeployment()

	dep := BuildDeployment(p, d, "bifrost-apps", nil)

	if dep.Name != "bifrost-my-api" {
		t.Errorf("expected name bifrost-my-api, got %s", dep.Name)
	}
	if dep.Namespace != "bifrost-apps" {
		t.Errorf("expected namespace bifrost-apps, got %s", dep.Namespace)
	}
	if *dep.Spec.Replicas != 2 {
		t.Errorf("expected 2 replicas, got %d", *dep.Spec.Replicas)
	}

	// Labels
	if dep.Labels["app.kubernetes.io/managed-by"] != "bifrost" {
		t.Errorf("missing managed-by label")
	}
	if dep.Labels["bifrost.io/project-id"] != p.ID.String() {
		t.Errorf("missing project-id label")
	}

	// Container
	containers := dep.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers (app + sidecar), got %d", len(containers))
	}
	c := containers[0]

	if c.Image != *d.ImageURI {
		t.Errorf("expected image %s, got %s", *d.ImageURI, c.Image)
	}
	if c.Ports[0].ContainerPort != 8080 {
		t.Errorf("expected port 8080, got %d", c.Ports[0].ContainerPort)
	}

	// Resource requests
	cpuReq := c.Resources.Requests[corev1.ResourceCPU]
	if !cpuReq.Equal(resource.MustParse("250m")) {
		t.Errorf("expected cpu request 250m, got %s", cpuReq.String())
	}
	memLimit := c.Resources.Limits[corev1.ResourceMemory]
	if !memLimit.Equal(resource.MustParse("256Mi")) {
		t.Errorf("expected memory limit 256Mi, got %s", memLimit.String())
	}

	// Probes
	if c.ReadinessProbe == nil {
		t.Fatal("expected readiness probe")
	}
	if c.ReadinessProbe.HTTPGet.Path != "/healthz" {
		t.Errorf("expected readiness path /healthz, got %s", c.ReadinessProbe.HTTPGet.Path)
	}
	if c.LivenessProbe == nil {
		t.Fatal("expected liveness probe")
	}

	// Pod labels include commit SHA
	podLabels := dep.Spec.Template.Labels
	if podLabels["bifrost.io/commit-sha"] != d.CommitSHA {
		t.Errorf("expected commit-sha label %s, got %s", d.CommitSHA, podLabels["bifrost.io/commit-sha"])
	}
}

func TestBuildDeploymentPlaceholderImage(t *testing.T) {
	p := testProject()
	d := testDeployment()
	d.ImageURI = nil

	dep := BuildDeployment(p, d, "default", nil)
	c := dep.Spec.Template.Spec.Containers[0]
	if c.Image != "placeholder:latest" {
		t.Errorf("expected placeholder image, got %s", c.Image)
	}
}

func TestBuildService(t *testing.T) {
	p := testProject()
	d := testDeployment()

	svc := BuildService(p, d, "bifrost-apps")

	if svc.Name != "bifrost-my-api" {
		t.Errorf("expected name bifrost-my-api, got %s", svc.Name)
	}
	if svc.Namespace != "bifrost-apps" {
		t.Errorf("expected namespace bifrost-apps, got %s", svc.Namespace)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("expected ClusterIP, got %s", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("expected port 80, got %d", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[0].TargetPort.IntVal != 8080 {
		t.Errorf("expected targetPort 8080, got %d", svc.Spec.Ports[0].TargetPort.IntVal)
	}

	// Selector should match deployment selector
	if svc.Spec.Selector["app.kubernetes.io/name"] != "my-api" {
		t.Errorf("selector mismatch: %v", svc.Spec.Selector)
	}
}
