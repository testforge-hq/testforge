package temporal

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/config"
)

// Client wraps the Temporal SDK client with additional functionality
type Client struct {
	client.Client
	logger    *zap.Logger
	namespace string
	taskQueue string
}

// NewClient creates a new Temporal client
func NewClient(cfg config.TemporalConfig, logger *zap.Logger) (*Client, error) {
	// Create Temporal client options
	options := client.Options{
		HostPort:  cfg.Address(),
		Namespace: cfg.Namespace,
		Logger:    NewZapAdapter(logger),
	}

	// Connect to Temporal
	c, err := client.Dial(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create Temporal client: %w", err)
	}

	return &Client{
		Client:    c,
		logger:    logger,
		namespace: cfg.Namespace,
		taskQueue: cfg.TaskQueue,
	}, nil
}

// TaskQueue returns the configured task queue name
func (c *Client) TaskQueue() string {
	return c.taskQueue
}

// Namespace returns the configured namespace
func (c *Client) Namespace() string {
	return c.namespace
}

// StartWorkflow starts a workflow with standard options
func (c *Client) StartWorkflow(ctx context.Context, workflowID string, workflow interface{}, input interface{}) (client.WorkflowRun, error) {
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: c.taskQueue,
	}

	return c.ExecuteWorkflow(ctx, options, workflow, input)
}

// GetWorkflowStatus returns the current status of a workflow
func (c *Client) GetWorkflowStatus(ctx context.Context, workflowID, runID string) (*WorkflowStatus, error) {
	desc, err := c.DescribeWorkflowExecution(ctx, workflowID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to describe workflow: %w", err)
	}

	info := desc.WorkflowExecutionInfo
	status := &WorkflowStatus{
		WorkflowID: info.Execution.WorkflowId,
		RunID:      info.Execution.RunId,
		Status:     info.Status.String(),
		StartTime:  info.StartTime.AsTime(),
	}

	if info.CloseTime != nil {
		closeTime := info.CloseTime.AsTime()
		status.CloseTime = &closeTime
	}

	return status, nil
}

// CancelWorkflow cancels a running workflow
func (c *Client) CancelWorkflow(ctx context.Context, workflowID, runID string) error {
	return c.Client.CancelWorkflow(ctx, workflowID, runID)
}

// WorkflowStatus represents the status of a workflow execution
type WorkflowStatus struct {
	WorkflowID string
	RunID      string
	Status     string
	StartTime  time.Time
	CloseTime  *time.Time
}

// IsRunning returns true if the workflow is still running
func (s *WorkflowStatus) IsRunning() bool {
	return s.Status == "Running" || s.Status == "WORKFLOW_EXECUTION_STATUS_RUNNING"
}

// IsCompleted returns true if the workflow completed successfully
func (s *WorkflowStatus) IsCompleted() bool {
	return s.Status == "Completed" || s.Status == "WORKFLOW_EXECUTION_STATUS_COMPLETED"
}

// IsFailed returns true if the workflow failed
func (s *WorkflowStatus) IsFailed() bool {
	return s.Status == "Failed" || s.Status == "WORKFLOW_EXECUTION_STATUS_FAILED"
}

// IsCanceled returns true if the workflow was canceled
func (s *WorkflowStatus) IsCanceled() bool {
	return s.Status == "Canceled" || s.Status == "WORKFLOW_EXECUTION_STATUS_CANCELED"
}

// ZapAdapter adapts zap.Logger to Temporal's log interface
type ZapAdapter struct {
	logger *zap.Logger
}

// NewZapAdapter creates a new Temporal logger adapter
func NewZapAdapter(logger *zap.Logger) *ZapAdapter {
	return &ZapAdapter{logger: logger.Named("temporal")}
}

func (z *ZapAdapter) Debug(msg string, keyvals ...interface{}) {
	z.logger.Debug(msg, toZapFields(keyvals)...)
}

func (z *ZapAdapter) Info(msg string, keyvals ...interface{}) {
	z.logger.Info(msg, toZapFields(keyvals)...)
}

func (z *ZapAdapter) Warn(msg string, keyvals ...interface{}) {
	z.logger.Warn(msg, toZapFields(keyvals)...)
}

func (z *ZapAdapter) Error(msg string, keyvals ...interface{}) {
	z.logger.Error(msg, toZapFields(keyvals)...)
}

func toZapFields(keyvals []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(keyvals)/2)
	for i := 0; i < len(keyvals)-1; i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		fields = append(fields, zap.Any(key, keyvals[i+1]))
	}
	return fields
}
