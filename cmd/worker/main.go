package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/testforge/testforge/internal/activities/automation"
	"github.com/testforge/testforge/internal/activities/discovery"
	"github.com/testforge/testforge/internal/activities/execution"
	"github.com/testforge/testforge/internal/activities/healing"
	"github.com/testforge/testforge/internal/activities/reporting"
	"github.com/testforge/testforge/internal/activities/testdesign"
	"github.com/testforge/testforge/internal/config"
	"github.com/testforge/testforge/internal/workflows"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := initLogger(cfg.App.Environment)
	defer logger.Sync()

	logger.Info("Starting TestForge Worker",
		zap.String("version", cfg.App.Version),
		zap.String("environment", cfg.App.Environment),
		zap.String("temporal_address", cfg.Temporal.Address()),
		zap.String("namespace", cfg.Temporal.Namespace),
		zap.String("task_queue", cfg.Temporal.TaskQueue),
	)

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.Address(),
		Namespace: cfg.Temporal.Namespace,
	})
	if err != nil {
		logger.Fatal("Failed to create Temporal client", zap.Error(err))
	}
	defer c.Close()

	logger.Info("Connected to Temporal server")

	// Create worker
	w := worker.New(c, cfg.Temporal.TaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     cfg.Temporal.WorkerCount,
		MaxConcurrentWorkflowTaskExecutionSize: cfg.Temporal.WorkerCount,
	})

	// Register workflows
	w.RegisterWorkflow(workflows.MasterOrchestrationWorkflow)

	// Create and register activities
	discoveryActivity := discovery.NewActivity()
	testDesignActivity := testdesign.NewActivity()
	automationActivity := automation.NewActivity()

	// Create execution activity with sandbox configuration
	execCfg := execution.Config{
		LocalMode:      cfg.App.Environment == "development" || !cfg.K8s.InCluster,
		LocalWorkDir:   "/tmp/testforge-sandboxes",
		Namespace:      cfg.K8s.Namespace + "-sandboxes",
		DefaultTimeout: cfg.K8s.SandboxTimeout,
		Kubeconfig:     cfg.K8s.Kubeconfig,
		MinIOEndpoint:  cfg.S3.Endpoint,
		MinIOAccessKey: cfg.S3.AccessKeyID,
		MinIOSecretKey: cfg.S3.SecretAccessKey,
		MinIOBucket:    cfg.S3.Bucket,
		APIEndpoint:    fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port),
	}

	executionActivity, err := execution.NewActivity(execCfg, nil, logger)
	if err != nil {
		logger.Fatal("Failed to create execution activity", zap.Error(err))
	}

	// Create healing activity with Claude and V-JEPA configuration
	healingCfg := healing.Config{
		ClaudeAPIKey:        cfg.Claude.APIKey,
		ClaudeModel:         cfg.Claude.Model,
		VJEPAEndpoint:       cfg.VJEPA.Address(),
		SimilarityThreshold: 0.85,
		MaxAttempts:         3,
		TimeoutSeconds:      60,
		EnableVisualHealing: cfg.VJEPA.Host != "",
	}

	healingActivity, err := healing.NewActivity(healingCfg, logger)
	if err != nil {
		logger.Warn("Failed to create healing activity, healing will be disabled", zap.Error(err))
	}

	reportingActivity := reporting.NewActivity()

	w.RegisterActivityWithOptions(discoveryActivity.Execute, activity.RegisterOptions{
		Name: workflows.DiscoveryActivityName,
	})
	w.RegisterActivityWithOptions(testDesignActivity.Execute, activity.RegisterOptions{
		Name: workflows.TestDesignActivityName,
	})
	w.RegisterActivityWithOptions(automationActivity.Execute, activity.RegisterOptions{
		Name: workflows.AutomationActivityName,
	})
	w.RegisterActivityWithOptions(executionActivity.Execute, activity.RegisterOptions{
		Name: workflows.ExecutionActivityName,
	})
	if healingActivity != nil {
		w.RegisterActivityWithOptions(healingActivity.Heal, activity.RegisterOptions{
			Name: workflows.HealingActivityName,
		})
	}
	w.RegisterActivityWithOptions(reportingActivity.Execute, activity.RegisterOptions{
		Name: workflows.ReportActivityName,
	})

	logger.Info("Registered workflows and activities",
		zap.Int("activity_count", 6),
		zap.Int("workflow_count", 1),
	)

	// Start worker in goroutine
	workerErrors := make(chan error, 1)
	go func() {
		workerErrors <- w.Run(worker.InterruptCh())
	}()

	logger.Info("Worker started successfully",
		zap.String("task_queue", cfg.Temporal.TaskQueue),
	)

	// Wait for shutdown signal or worker error
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-workerErrors:
		if err != nil {
			logger.Fatal("Worker error", zap.Error(err))
		}

	case sig := <-shutdown:
		logger.Info("Shutdown signal received", zap.String("signal", sig.String()))
		w.Stop()
		logger.Info("Worker stopped gracefully")
	}
}

func initLogger(env string) *zap.Logger {
	var config zap.Config
	if env == "production" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := config.Build()
	if err != nil {
		logger, _ = zap.NewProduction()
	}

	return logger
}
