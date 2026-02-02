package reporting

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/testforge/testforge/internal/workflows"
)

// Activity implements the reporting activity
type Activity struct {
	// Dependencies will be added here (S3 client, template engine, etc.)
}

// NewActivity creates a new reporting activity
func NewActivity() *Activity {
	return &Activity{}
}

// Execute generates test reports and uploads them to storage
func (a *Activity) Execute(ctx context.Context, input workflows.ReportInput) (*workflows.ReportOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting reporting activity",
		"test_run_id", input.TestRunID.String(),
		"total_results", len(input.Results),
	)

	activity.RecordHeartbeat(ctx, "Generating report...")

	// TODO: Implement actual report generation with templates
	// For now, return mock report URL

	// Simulate report generation
	time.Sleep(1 * time.Second)

	// In production, this would:
	// 1. Generate HTML report from template
	// 2. Generate PDF version
	// 3. Upload to S3/MinIO
	// 4. Return the public URL

	reportURL := fmt.Sprintf(
		"https://minio:9000/testforge/reports/%s/%s/report.html",
		input.TenantID.String(),
		input.TestRunID.String(),
	)

	duration := time.Since(startTime)

	output := &workflows.ReportOutput{
		ReportURL: reportURL,
		Duration:  duration,
	}

	logger.Info("Reporting activity completed",
		"report_url", reportURL,
		"duration", duration,
	)

	return output, nil
}
