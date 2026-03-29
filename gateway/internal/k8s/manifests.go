package k8s

import (
	"regexp"
	"strings"

	"duolok/bifrost/gateway/internal/models"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	managedBy        = "bifrost"
	defaultReplicas  = int32(2)
	defaultPort      = int32(8080)
	healthPath       = "/healthz"
	sidecarImage     = "europe-central2-docker.pkg.dev/bifrost-platform/bifrost-platform/healthcheck:latest"
	sidecarInterval  = "10"
	gatewayService   = "bifrost-gateway"
	gatewayPort      = "8080"
)

var invalidDNS = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizeName converts a project name to a valid K8s DNS label.
func sanitizeName(name string) string {
	s := strings.ToLower(name)
	s = invalidDNS.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// resourceName returns the K8s resource name for a project.
func resourceName(projectName string) string {
	return "bifrost-" + sanitizeName(projectName)
}

func labels(project models.Project, deployment models.Deployment) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       sanitizeName(project.Name),
		"app.kubernetes.io/managed-by": managedBy,
		"bifrost.io/project-id":        project.ID.String(),
		"bifrost.io/deployment-id":     deployment.ID.String(),
	}
}

func selectorLabels(project models.Project) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": sanitizeName(project.Name),
	}
}

func BuildDeployment(project models.Project, deployment models.Deployment, namespace string, secrets map[string]string) *appsv1.Deployment {
	replicas := defaultReplicas
	name := resourceName(project.Name)
	allLabels := labels(project, deployment)
	selLabels := selectorLabels(project)

	podLabels := make(map[string]string)
	for k, v := range allLabels {
		podLabels[k] = v
	}
	podLabels["bifrost.io/commit-sha"] = deployment.CommitSHA

	imageURI := "placeholder:latest"
	if deployment.ImageURI != nil {
		imageURI = *deployment.ImageURI
	}

	// Build env vars from project secrets
	var envVars []corev1.EnvVar
	for key, val := range secrets {
		envVars = append(envVars, corev1.EnvVar{Name: key, Value: val})
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    allLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  sanitizeName(project.Name),
							Image: imageURI,
							Env:   envVars,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: defaultPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("250m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: healthPath,
										Port: intstr.FromInt32(defaultPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: healthPath,
										Port: intstr.FromInt32(defaultPort),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
						},
						{
							Name:  "healthcheck",
							Image: sidecarImage,
							Env: []corev1.EnvVar{
								{Name: "BF_DEPLOY_ID", Value: deployment.ID.String()},
								{Name: "BF_PROBE_HOST", Value: "localhost"},
								{Name: "BF_PROBE_PORT", Value: "8080"},
								{Name: "BF_PROBE_PATH", Value: healthPath},
								{Name: "BF_GATEWAY_HOST", Value: gatewayService},
								{Name: "BF_GATEWAY_PORT", Value: gatewayPort},
								{Name: "BF_INTERVAL_S", Value: sidecarInterval},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("16Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildService(project models.Project, deployment models.Deployment, namespace string) *corev1.Service {
	name := resourceName(project.Name)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels(project, deployment),
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels(project),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(defaultPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}
