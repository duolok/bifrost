package k8s

import (
	"context"
	"fmt"
	"log/slog"

	"duolok/bifrost/gateway/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type DeployConfig struct {
	Namespace  string
	InCluster  bool
	Kubeconfig string
}

type Deployer struct {
	client    kubernetes.Interface
	pool      *pgxpool.Pool
	namespace string
}

func NewDeployer(cfg DeployConfig, pool *pgxpool.Pool) (*Deployer, error) {
	var restConfig *rest.Config
	var err error

	if cfg.InCluster {
		restConfig, err = rest.InClusterConfig()
	} else {
		restConfig, err = clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	}
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}

	version, err := client.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("k8s connectivity check: %w", err)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = "bifrost-apps"
	}

	slog.Info("k8s deployer connected",
		"server_version", version.GitVersion,
		"namespace", ns,
	)

	return &Deployer{
		client:    client,
		pool:      pool,
		namespace: ns,
	}, nil
}

// NewDeployerWithClient creates a Deployer with a provided kubernetes client (for testing).
func NewDeployerWithClient(client kubernetes.Interface, pool *pgxpool.Pool, namespace string) *Deployer {
	if namespace == "" {
		namespace = "bifrost-apps"
	}
	return &Deployer{
		client:    client,
		pool:      pool,
		namespace: namespace,
	}
}

func (d *Deployer) Deploy(ctx context.Context, deployment models.Deployment, project models.Project) error {
	if !models.CanTransition(deployment.Status, models.StatusDeploying) {
		return fmt.Errorf("cannot deploy from status %s", deployment.Status)
	}

	// Transition to deploying
	if err := d.transitionStatus(ctx, deployment.ID, deployment.Status, models.StatusDeploying, "Applying K8s manifests"); err != nil {
		return err
	}

	// Generate and apply manifests
	if err := d.applyManifests(ctx, project, deployment); err != nil {
		msg := fmt.Sprintf("deploy failed: %v", err)
		_ = d.transitionStatus(ctx, deployment.ID, models.StatusDeploying, models.StatusFailed, msg)
		return err
	}

	// Transition to running
	if err := d.transitionStatus(ctx, deployment.ID, models.StatusDeploying, models.StatusRunning, "Manifests applied"); err != nil {
		return err
	}

	slog.Info("deployment applied to k8s",
		"deployment_id", deployment.ID,
		"project", project.Name,
		"namespace", d.namespace,
	)

	return nil
}

func (d *Deployer) Delete(ctx context.Context, projectName string) error {
	name := resourceName(projectName)
	propagation := metav1.DeletePropagationForeground
	opts := metav1.DeleteOptions{PropagationPolicy: &propagation}

	err := d.client.AppsV1().Deployments(d.namespace).Delete(ctx, name, opts)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete deployment: %w", err)
	}

	err = d.client.CoreV1().Services(d.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete service: %w", err)
	}

	return nil
}

func (d *Deployer) applyManifests(ctx context.Context, project models.Project, deployment models.Deployment) error {
	dep := BuildDeployment(project, deployment, d.namespace)
	if err := d.applyDeployment(ctx, dep); err != nil {
		return fmt.Errorf("apply deployment: %w", err)
	}

	svc := BuildService(project, deployment, d.namespace)
	if err := d.applyService(ctx, svc); err != nil {
		return fmt.Errorf("apply service: %w", err)
	}

	return nil
}

func (d *Deployer) applyDeployment(ctx context.Context, dep *appsv1.Deployment) error {
	client := d.client.AppsV1().Deployments(dep.Namespace)

	existing, err := client.Get(ctx, dep.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = client.Create(ctx, dep, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	dep.ResourceVersion = existing.ResourceVersion
	_, err = client.Update(ctx, dep, metav1.UpdateOptions{})
	return err
}

func (d *Deployer) applyService(ctx context.Context, svc *corev1.Service) error {
	client := d.client.CoreV1().Services(svc.Namespace)

	existing, err := client.Get(ctx, svc.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = client.Create(ctx, svc, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	svc.ResourceVersion = existing.ResourceVersion
	svc.Spec.ClusterIP = existing.Spec.ClusterIP // ClusterIP is immutable
	_, err = client.Update(ctx, svc, metav1.UpdateOptions{})
	return err
}

func (d *Deployer) transitionStatus(ctx context.Context, deployID uuid.UUID, from, to models.DeploymentStatus, message string) error {
	var query string
	switch to {
	case models.StatusDeploying:
		query = `UPDATE deployments SET status = $1, deploy_started_at = NOW(), status_message = $2 WHERE id = $3 AND status = $4`
	case models.StatusRunning:
		query = `UPDATE deployments SET status = $1, deploy_finished_at = NOW(), status_message = $2 WHERE id = $3 AND status = $4`
	case models.StatusFailed:
		query = `UPDATE deployments SET status = $1, deploy_finished_at = NOW(), status_message = $2 WHERE id = $3 AND status = $4`
	default:
		query = `UPDATE deployments SET status = $1, status_message = $2 WHERE id = $3 AND status = $4`
	}

	tag, err := d.pool.Exec(ctx, query, to, message, deployID, from)
	if err != nil {
		return fmt.Errorf("transition %s -> %s: %w", from, to, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("transition %s -> %s: deployment %s already moved", from, to, deployID)
	}
	return nil
}
