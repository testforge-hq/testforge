package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// K8sManager manages sandbox pods in Kubernetes
type K8sManager struct {
	client    kubernetes.Interface
	namespace string
	config    ManagerConfig
	logger    *zap.Logger
}

// NewK8sManager creates a new Kubernetes sandbox manager
func NewK8sManager(client kubernetes.Interface, config ManagerConfig, logger *zap.Logger) *K8sManager {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
	}
	return &K8sManager{
		client:    client,
		namespace: config.Namespace,
		config:    config,
		logger:    logger,
	}
}

// RunTests creates a sandbox pod and waits for completion
func (m *K8sManager) RunTests(ctx context.Context, req SandboxRequest) (*SandboxResult, error) {
	startTime := time.Now()

	m.logger.Info("Starting K8s sandbox execution",
		zap.String("run_id", req.RunID),
		zap.String("tier", req.Tier),
		zap.String("target_url", req.TargetURL),
	)

	result := &SandboxResult{
		RunID:    req.RunID,
		TenantID: req.TenantID,
		Status:   SandboxStatusPending,
	}

	// Build pod spec
	pod, err := m.buildPodSpec(req)
	if err != nil {
		result.Status = SandboxStatusError
		result.Error = fmt.Sprintf("failed to build pod spec: %v", err)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Create pod
	m.logger.Info("Creating sandbox pod", zap.String("pod_name", pod.Name))
	created, err := m.client.CoreV1().Pods(m.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		result.Status = SandboxStatusError
		result.Error = fmt.Sprintf("failed to create pod: %v", err)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	podName := created.Name
	defer m.cleanupPod(context.Background(), podName)

	// Watch for completion
	result.Status = SandboxStatusRunning
	watchResult, err := m.waitForCompletion(ctx, podName, req.Timeout)
	if err != nil {
		result.Status = SandboxStatusError
		result.Error = err.Error()
		result.Duration = time.Since(startTime)
		return result, nil
	}

	result.Status = watchResult.Status
	result.ExitCode = watchResult.ExitCode
	result.Duration = time.Since(startTime)

	// Collect logs
	logs, err := m.getPodLogs(ctx, podName)
	if err != nil {
		m.logger.Warn("Failed to get pod logs", zap.Error(err))
	}
	result.Logs = logs

	// Parse test results from logs or MinIO
	m.parseTestResults(result)

	m.logger.Info("Sandbox execution completed",
		zap.String("status", string(result.Status)),
		zap.Int("exit_code", result.ExitCode),
		zap.Duration("duration", result.Duration),
	)

	return result, nil
}

// buildPodSpec creates the pod specification for a sandbox
func (m *K8sManager) buildPodSpec(req SandboxRequest) (*corev1.Pod, error) {
	// Get resource limits by tier
	resources := m.getResourcesByTier(req.Tier)

	// Parse scripts URI
	bucket, path := m.parseMinioURI(req.ScriptsURI)
	resultsBucket := m.config.MinIOBucket
	resultsPath := fmt.Sprintf("%s/%s/results", req.TenantID, req.RunID)

	// Build Playwright arguments
	playwrightArgs := m.buildPlaywrightArgs(req)

	// Timeout in seconds
	timeoutSeconds := int64(req.Timeout.Seconds())
	if timeoutSeconds <= 0 {
		timeoutSeconds = int64(m.config.DefaultTimeout.Seconds())
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("testforge-run-%s", req.RunID[:8]),
			Namespace: m.namespace,
			Labels: map[string]string{
				"app":     "testforge-sandbox",
				"tenant":  req.TenantID,
				"run":     req.RunID,
				"tier":    req.Tier,
				"project": req.ProjectID,
			},
			Annotations: map[string]string{
				"testforge.io/timeout":    req.Timeout.String(),
				"testforge.io/created-at": time.Now().UTC().Format(time.RFC3339),
				"testforge.io/target-url": req.TargetURL,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:         corev1.RestartPolicyNever,
			ActiveDeadlineSeconds: &timeoutSeconds,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr(true),
				RunAsUser:    ptr(int64(1000)),
				FSGroup:      ptr(int64(1000)),
			},
			InitContainers: []corev1.Container{
				m.buildInitContainer(bucket, path),
			},
			Containers: []corev1.Container{
				m.buildMainContainer(req, resources, resultsBucket, resultsPath, playwrightArgs),
			},
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "results",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "shm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium:    corev1.StorageMediumMemory,
							SizeLimit: resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
						},
					},
				},
			},
			// Tolerations for dedicated sandbox nodes
			Tolerations: []corev1.Toleration{
				{
					Key:      "testforge.io/sandbox",
					Operator: corev1.TolerationOpEqual,
					Value:    "true",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
	}

	return pod, nil
}

// buildInitContainer creates the init container for downloading scripts
func (m *K8sManager) buildInitContainer(bucket, path string) corev1.Container {
	return corev1.Container{
		Name:  "init-scripts",
		Image: "minio/mc:latest",
		Command: []string{
			"/bin/sh",
			"-c",
			`
mc alias set storage $MINIO_ENDPOINT $MINIO_ACCESS_KEY $MINIO_SECRET_KEY --api S3v4
mc cp storage/$SCRIPTS_BUCKET/$SCRIPTS_PATH /workspace/scripts.zip
cd /workspace && unzip -o scripts.zip
ls -la /workspace
`,
		},
		Env: []corev1.EnvVar{
			{
				Name: "MINIO_ENDPOINT",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "testforge-minio"},
						Key:                  "endpoint",
					},
				},
			},
			{
				Name: "MINIO_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "testforge-minio"},
						Key:                  "access-key",
					},
				},
			},
			{
				Name: "MINIO_SECRET_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "testforge-minio"},
						Key:                  "secret-key",
					},
				},
			},
			{Name: "SCRIPTS_BUCKET", Value: bucket},
			{Name: "SCRIPTS_PATH", Value: path},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workspace", MountPath: "/workspace"},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	}
}

// buildMainContainer creates the main Playwright container
func (m *K8sManager) buildMainContainer(req SandboxRequest, resources ResourceLimits, resultsBucket, resultsPath, playwrightArgs string) corev1.Container {
	return corev1.Container{
		Name:       "playwright",
		Image:      "mcr.microsoft.com/playwright:v1.40.0-jammy",
		WorkingDir: "/workspace",
		Command: []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(`
cd /workspace
npm ci --prefer-offline
npx playwright install chromium

# Run tests with JSON reporter
npx playwright test \
  --reporter=json,list \
  --output=/results \
  %s

EXIT_CODE=$?

# Save results
cp -r test-results/* /results/ 2>/dev/null || true
cp playwright-report/* /results/ 2>/dev/null || true

# Upload results to MinIO
mc alias set storage $MINIO_ENDPOINT $MINIO_ACCESS_KEY $MINIO_SECRET_KEY --api S3v4
mc cp --recursive /results/ storage/$RESULTS_BUCKET/$RESULTS_PATH/ || echo "Upload failed but continuing"

exit $EXIT_CODE
`, playwrightArgs),
		},
		Env: []corev1.EnvVar{
			{Name: "CI", Value: "true"},
			{Name: "TEST_ENV", Value: req.Environment},
			{Name: "BASE_URL", Value: req.TargetURL},
			{Name: "TESTFORGE_RUN_ID", Value: req.RunID},
			{Name: "TESTFORGE_API_ENDPOINT", Value: m.config.APIEndpoint},
			{Name: "RESULTS_BUCKET", Value: resultsBucket},
			{Name: "RESULTS_PATH", Value: resultsPath},
			{
				Name: "MINIO_ENDPOINT",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "testforge-minio"},
						Key:                  "endpoint",
					},
				},
			},
			{
				Name: "MINIO_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "testforge-minio"},
						Key:                  "access-key",
					},
				},
			},
			{
				Name: "MINIO_SECRET_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "testforge-minio"},
						Key:                  "secret-key",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workspace", MountPath: "/workspace"},
			{Name: "results", MountPath: "/results"},
			{Name: "shm", MountPath: "/dev/shm"},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(resources.RequestCPU),
				corev1.ResourceMemory: resource.MustParse(resources.RequestMemory),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(resources.LimitCPU),
				corev1.ResourceMemory: resource.MustParse(resources.LimitMemory),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}

// buildPlaywrightArgs builds the Playwright CLI arguments
func (m *K8sManager) buildPlaywrightArgs(req SandboxRequest) string {
	var args []string

	// Browser project
	browser := req.Browser
	if browser == "" {
		browser = "chromium"
	}
	args = append(args, fmt.Sprintf("--project=%s", browser))

	// Workers
	workers := req.Workers
	if workers <= 0 {
		workers = 2
	}
	args = append(args, fmt.Sprintf("--workers=%d", workers))

	// Test filter
	if req.TestFilter != "" {
		args = append(args, fmt.Sprintf("--grep='%s'", req.TestFilter))
	}

	// Retries
	if req.Retries > 0 {
		args = append(args, fmt.Sprintf("--retries=%d", req.Retries))
	}

	// Specific test files
	for _, file := range req.TestFiles {
		args = append(args, file)
	}

	return strings.Join(args, " ")
}

// getResourcesByTier returns resource limits for a given tier
func (m *K8sManager) getResourcesByTier(tier string) ResourceLimits {
	switch tier {
	case "enterprise":
		return m.config.EnterpriseTier
	case "pro":
		return m.config.ProTier
	default:
		return m.config.FreeTier
	}
}

// waitForCompletion watches the pod until it completes or times out
func (m *K8sManager) waitForCompletion(ctx context.Context, podName string, timeout time.Duration) (*SandboxResult, error) {
	watcher, err := m.client.CoreV1().Pods(m.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", podName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch pod: %w", err)
	}
	defer watcher.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-ctx.Done():
			return &SandboxResult{Status: SandboxStatusError}, ctx.Err()

		case <-timeoutCh:
			return &SandboxResult{
				Status: SandboxStatusTimeout,
			}, nil

		case event := <-watcher.ResultChan():
			if event.Type == watch.Deleted {
				return &SandboxResult{Status: SandboxStatusError}, fmt.Errorf("pod deleted")
			}

			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				return &SandboxResult{
					Status:   SandboxStatusSucceeded,
					ExitCode: 0,
				}, nil

			case corev1.PodFailed:
				exitCode := 1
				if len(pod.Status.ContainerStatuses) > 0 {
					if term := pod.Status.ContainerStatuses[0].State.Terminated; term != nil {
						exitCode = int(term.ExitCode)
					}
				}
				return &SandboxResult{
					Status:   SandboxStatusFailed,
					ExitCode: exitCode,
				}, nil
			}
		}
	}
}

// getPodLogs retrieves logs from the pod
func (m *K8sManager) getPodLogs(ctx context.Context, podName string) (string, error) {
	req := m.client.CoreV1().Pods(m.namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: "playwright",
	})

	logs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer logs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, logs)
	return buf.String(), err
}

// parseTestResults extracts test results from logs
func (m *K8sManager) parseTestResults(result *SandboxResult) {
	// Try to parse from logs
	if result.Logs != "" {
		lines := strings.Split(result.Logs, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "passed") {
				fmt.Sscanf(line, "%d passed", &result.TestsPassed)
			}
			if strings.Contains(line, "failed") {
				fmt.Sscanf(line, "%d failed", &result.TestsFailed)
			}
			if strings.Contains(line, "skipped") {
				fmt.Sscanf(line, "%d skipped", &result.TestsSkipped)
			}
		}
	}
	result.TotalTests = result.TestsPassed + result.TestsFailed + result.TestsSkipped
}

// cleanupPod deletes the sandbox pod
func (m *K8sManager) cleanupPod(ctx context.Context, podName string) error {
	m.logger.Info("Cleaning up pod", zap.String("pod_name", podName))
	return m.client.CoreV1().Pods(m.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
}

// parseMinioURI parses a MinIO URI into bucket and path
func (m *K8sManager) parseMinioURI(uri string) (bucket, path string) {
	// Format: bucket/path/to/file
	uri = strings.TrimPrefix(uri, "s3://")
	uri = strings.TrimPrefix(uri, "minio://")

	parts := strings.SplitN(uri, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return m.config.MinIOBucket, uri
}

// Cleanup removes any resources associated with a run
func (m *K8sManager) Cleanup(runID string) error {
	ctx := context.Background()
	podName := fmt.Sprintf("sandbox-%s", runID)
	return m.cleanupPod(ctx, podName)
}

// Helper function
func ptr[T any](v T) *T {
	return &v
}
