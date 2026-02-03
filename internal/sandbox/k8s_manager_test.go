package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestNewK8sManager(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		logger := zap.NewNop()
		config := DefaultManagerConfig()
		config.Namespace = "test-namespace"

		manager := NewK8sManager(client, config, logger)

		require.NotNil(t, manager)
		assert.Equal(t, client, manager.client)
		assert.Equal(t, "test-namespace", manager.namespace)
		assert.Equal(t, logger, manager.logger)
	})

	t.Run("without logger creates development logger", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		config := DefaultManagerConfig()

		manager := NewK8sManager(client, config, nil)

		require.NotNil(t, manager)
		assert.NotNil(t, manager.logger)
	})
}

func TestK8sManager_getResourcesByTier(t *testing.T) {
	config := DefaultManagerConfig()
	manager := NewK8sManager(fake.NewSimpleClientset(), config, zap.NewNop())

	tests := []struct {
		tier            string
		expectedRequest string
		expectedLimit   string
	}{
		{
			tier:            "free",
			expectedRequest: "500m",
			expectedLimit:   "1",
		},
		{
			tier:            "pro",
			expectedRequest: "1",
			expectedLimit:   "2",
		},
		{
			tier:            "enterprise",
			expectedRequest: "2",
			expectedLimit:   "4",
		},
		{
			tier:            "unknown",
			expectedRequest: "500m", // defaults to free
			expectedLimit:   "1",
		},
		{
			tier:            "",
			expectedRequest: "500m", // defaults to free
			expectedLimit:   "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			resources := manager.getResourcesByTier(tt.tier)
			assert.Equal(t, tt.expectedRequest, resources.RequestCPU)
			assert.Equal(t, tt.expectedLimit, resources.LimitCPU)
		})
	}
}

func TestK8sManager_parseMinioURI(t *testing.T) {
	config := DefaultManagerConfig()
	config.MinIOBucket = "default-bucket"
	manager := NewK8sManager(fake.NewSimpleClientset(), config, zap.NewNop())

	tests := []struct {
		name           string
		uri            string
		expectedBucket string
		expectedPath   string
	}{
		{
			name:           "simple bucket/path",
			uri:            "mybucket/path/to/file.zip",
			expectedBucket: "mybucket",
			expectedPath:   "path/to/file.zip",
		},
		{
			name:           "s3 prefix",
			uri:            "s3://mybucket/scripts.zip",
			expectedBucket: "mybucket",
			expectedPath:   "scripts.zip",
		},
		{
			name:           "minio prefix",
			uri:            "minio://mybucket/path/file.zip",
			expectedBucket: "mybucket",
			expectedPath:   "path/file.zip",
		},
		{
			name:           "path only - uses default bucket",
			uri:            "scripts.zip",
			expectedBucket: "default-bucket",
			expectedPath:   "scripts.zip",
		},
		{
			name:           "nested path",
			uri:            "bucket/tenant/project/run/scripts.zip",
			expectedBucket: "bucket",
			expectedPath:   "tenant/project/run/scripts.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, path := manager.parseMinioURI(tt.uri)
			assert.Equal(t, tt.expectedBucket, bucket)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}

func TestK8sManager_buildPlaywrightArgs(t *testing.T) {
	config := DefaultManagerConfig()
	manager := NewK8sManager(fake.NewSimpleClientset(), config, zap.NewNop())

	tests := []struct {
		name     string
		req      SandboxRequest
		expected []string
	}{
		{
			name: "default values",
			req:  SandboxRequest{},
			expected: []string{
				"--project=chromium",
				"--workers=2",
			},
		},
		{
			name: "custom browser and workers",
			req: SandboxRequest{
				Browser: "firefox",
				Workers: 4,
			},
			expected: []string{
				"--project=firefox",
				"--workers=4",
			},
		},
		{
			name: "with test filter",
			req: SandboxRequest{
				TestFilter: "@smoke",
			},
			expected: []string{
				"--project=chromium",
				"--workers=2",
				"--grep='@smoke'",
			},
		},
		{
			name: "with retries",
			req: SandboxRequest{
				Retries: 3,
			},
			expected: []string{
				"--project=chromium",
				"--workers=2",
				"--retries=3",
			},
		},
		{
			name: "with test files",
			req: SandboxRequest{
				TestFiles: []string{"login.spec.ts", "signup.spec.ts"},
			},
			expected: []string{
				"--project=chromium",
				"--workers=2",
				"login.spec.ts",
				"signup.spec.ts",
			},
		},
		{
			name: "full configuration",
			req: SandboxRequest{
				Browser:    "webkit",
				Workers:    8,
				TestFilter: "@regression",
				Retries:    2,
				TestFiles:  []string{"e2e.spec.ts"},
			},
			expected: []string{
				"--project=webkit",
				"--workers=8",
				"--grep='@regression'",
				"--retries=2",
				"e2e.spec.ts",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := manager.buildPlaywrightArgs(tt.req)
			for _, exp := range tt.expected {
				assert.Contains(t, args, exp)
			}
		})
	}
}

func TestK8sManager_buildPodSpec(t *testing.T) {
	config := DefaultManagerConfig()
	config.Namespace = "test-ns"
	config.MinIOBucket = "testforge"
	config.APIEndpoint = "http://api:8080"
	manager := NewK8sManager(fake.NewSimpleClientset(), config, zap.NewNop())

	req := SandboxRequest{
		RunID:       "run-12345678-abcd",
		TenantID:    "tenant-123",
		ProjectID:   "project-456",
		Tier:        "pro",
		ScriptsURI:  "bucket/scripts.zip",
		TargetURL:   "https://example.com",
		Environment: "staging",
		Timeout:     10 * time.Minute,
		Browser:     "chromium",
		Workers:     4,
	}

	pod, err := manager.buildPodSpec(req)
	require.NoError(t, err)
	require.NotNil(t, pod)

	// Verify pod metadata
	assert.Equal(t, "testforge-run-run-1234", pod.Name)
	assert.Equal(t, "test-ns", pod.Namespace)

	// Verify labels
	assert.Equal(t, "testforge-sandbox", pod.Labels["app"])
	assert.Equal(t, "tenant-123", pod.Labels["tenant"])
	assert.Equal(t, "run-12345678-abcd", pod.Labels["run"])
	assert.Equal(t, "pro", pod.Labels["tier"])
	assert.Equal(t, "project-456", pod.Labels["project"])

	// Verify annotations
	assert.Equal(t, "10m0s", pod.Annotations["testforge.io/timeout"])
	assert.Equal(t, "https://example.com", pod.Annotations["testforge.io/target-url"])

	// Verify pod spec
	assert.Equal(t, corev1.RestartPolicyNever, pod.Spec.RestartPolicy)
	assert.NotNil(t, pod.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, int64(600), *pod.Spec.ActiveDeadlineSeconds)

	// Verify security context
	assert.NotNil(t, pod.Spec.SecurityContext)
	assert.True(t, *pod.Spec.SecurityContext.RunAsNonRoot)
	assert.Equal(t, int64(1000), *pod.Spec.SecurityContext.RunAsUser)

	// Verify init container
	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, "init-scripts", pod.Spec.InitContainers[0].Name)
	assert.Equal(t, "minio/mc:latest", pod.Spec.InitContainers[0].Image)

	// Verify main container
	require.Len(t, pod.Spec.Containers, 1)
	assert.Equal(t, "playwright", pod.Spec.Containers[0].Name)
	assert.Equal(t, "mcr.microsoft.com/playwright:v1.40.0-jammy", pod.Spec.Containers[0].Image)
	assert.Equal(t, "/workspace", pod.Spec.Containers[0].WorkingDir)

	// Verify environment variables
	envMap := make(map[string]string)
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Value != "" {
			envMap[env.Name] = env.Value
		}
	}
	assert.Equal(t, "true", envMap["CI"])
	assert.Equal(t, "staging", envMap["TEST_ENV"])
	assert.Equal(t, "https://example.com", envMap["BASE_URL"])
	assert.Equal(t, "run-12345678-abcd", envMap["TESTFORGE_RUN_ID"])

	// Verify volumes
	assert.Len(t, pod.Spec.Volumes, 3)
	volumeNames := make([]string, len(pod.Spec.Volumes))
	for i, v := range pod.Spec.Volumes {
		volumeNames[i] = v.Name
	}
	assert.Contains(t, volumeNames, "workspace")
	assert.Contains(t, volumeNames, "results")
	assert.Contains(t, volumeNames, "shm")

	// Verify tolerations
	require.Len(t, pod.Spec.Tolerations, 1)
	assert.Equal(t, "testforge.io/sandbox", pod.Spec.Tolerations[0].Key)
}

func TestK8sManager_buildPodSpec_DefaultTimeout(t *testing.T) {
	config := DefaultManagerConfig()
	config.DefaultTimeout = 20 * time.Minute
	manager := NewK8sManager(fake.NewSimpleClientset(), config, zap.NewNop())

	req := SandboxRequest{
		RunID:    "run-12345678",
		TenantID: "tenant-123",
		Timeout:  0, // No timeout specified
	}

	pod, err := manager.buildPodSpec(req)
	require.NoError(t, err)

	// Should use default timeout
	assert.Equal(t, int64(1200), *pod.Spec.ActiveDeadlineSeconds)
}

func TestK8sManager_buildInitContainer(t *testing.T) {
	config := DefaultManagerConfig()
	manager := NewK8sManager(fake.NewSimpleClientset(), config, zap.NewNop())

	container := manager.buildInitContainer("mybucket", "path/to/scripts.zip")

	assert.Equal(t, "init-scripts", container.Name)
	assert.Equal(t, "minio/mc:latest", container.Image)

	// Check environment variables
	envMap := make(map[string]string)
	for _, env := range container.Env {
		if env.Value != "" {
			envMap[env.Name] = env.Value
		}
	}
	assert.Equal(t, "mybucket", envMap["SCRIPTS_BUCKET"])
	assert.Equal(t, "path/to/scripts.zip", envMap["SCRIPTS_PATH"])

	// Check volume mounts
	require.Len(t, container.VolumeMounts, 1)
	assert.Equal(t, "workspace", container.VolumeMounts[0].Name)
	assert.Equal(t, "/workspace", container.VolumeMounts[0].MountPath)
}

func TestK8sManager_parseTestResults(t *testing.T) {
	config := DefaultManagerConfig()
	manager := NewK8sManager(fake.NewSimpleClientset(), config, zap.NewNop())

	tests := []struct {
		name           string
		logs           string
		expectedPassed int
		expectedFailed int
		expectedSkip   int
		expectedTotal  int
	}{
		{
			name:           "all passed",
			logs:           "Running tests\n5 passed (3.2s)",
			expectedPassed: 5,
			expectedFailed: 0,
			expectedSkip:   0,
			expectedTotal:  5,
		},
		{
			name:           "mixed results",
			logs:           "Running tests\n3 passed\n2 failed\n1 skipped (5.5s)",
			expectedPassed: 3,
			expectedFailed: 2,
			expectedSkip:   1,
			expectedTotal:  6,
		},
		{
			name:           "empty logs",
			logs:           "",
			expectedPassed: 0,
			expectedFailed: 0,
			expectedSkip:   0,
			expectedTotal:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &SandboxResult{Logs: tt.logs}
			manager.parseTestResults(result)

			assert.Equal(t, tt.expectedPassed, result.TestsPassed)
			assert.Equal(t, tt.expectedFailed, result.TestsFailed)
			assert.Equal(t, tt.expectedSkip, result.TestsSkipped)
			assert.Equal(t, tt.expectedTotal, result.TotalTests)
		})
	}
}

func TestK8sManager_Cleanup(t *testing.T) {
	client := fake.NewSimpleClientset()
	config := DefaultManagerConfig()
	config.Namespace = "test-ns"
	manager := NewK8sManager(client, config, zap.NewNop())

	// Create a pod to delete
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-run-123",
			Namespace: "test-ns",
		},
	}
	_, err := client.CoreV1().Pods("test-ns").Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	// Cleanup
	err = manager.Cleanup("run-123")
	assert.NoError(t, err)
}

func TestK8sManager_RunTests_PodCreationFailed(t *testing.T) {
	client := fake.NewSimpleClientset()
	// Make pod creation fail
	client.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})

	config := DefaultManagerConfig()
	config.Namespace = "test-ns"
	manager := NewK8sManager(client, config, zap.NewNop())

	req := SandboxRequest{
		RunID:      "run-12345678",
		TenantID:   "tenant-123",
		ScriptsURI: "bucket/scripts.zip",
		Timeout:    5 * time.Minute,
	}

	result, err := manager.RunTests(context.Background(), req)
	require.NoError(t, err) // RunTests returns result with error, not error
	assert.Equal(t, SandboxStatusError, result.Status)
	assert.Contains(t, result.Error, "failed to create pod")
}

func TestK8sManager_cleanupPod(t *testing.T) {
	client := fake.NewSimpleClientset()
	config := DefaultManagerConfig()
	config.Namespace = "test-ns"
	manager := NewK8sManager(client, config, zap.NewNop())

	// Create a pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
		},
	}
	_, err := client.CoreV1().Pods("test-ns").Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)

	// Cleanup
	err = manager.cleanupPod(context.Background(), "test-pod")
	assert.NoError(t, err)

	// Verify pod is deleted
	_, err = client.CoreV1().Pods("test-ns").Get(context.Background(), "test-pod", metav1.GetOptions{})
	assert.Error(t, err)
}

func TestPtr(t *testing.T) {
	// Test the ptr helper function
	intVal := 42
	ptrInt := ptr(intVal)
	assert.Equal(t, &intVal, ptrInt)
	assert.Equal(t, 42, *ptrInt)

	boolVal := true
	ptrBool := ptr(boolVal)
	assert.Equal(t, &boolVal, ptrBool)
	assert.True(t, *ptrBool)

	strVal := "test"
	ptrStr := ptr(strVal)
	assert.Equal(t, &strVal, ptrStr)
	assert.Equal(t, "test", *ptrStr)
}
