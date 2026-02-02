package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/vjepa"
)

func main() {
	// Flags
	address := flag.String("address", "localhost:50051", "V-JEPA service address")
	baseline := flag.String("baseline", "", "Path to baseline image")
	actual := flag.String("actual", "", "Path to actual image")
	healthOnly := flag.Bool("health", false, "Only check health")
	flag.Parse()

	// Logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create client
	client, err := vjepa.NewClient(vjepa.ClientConfig{
		Address: *address,
		Timeout: 30 * time.Second,
		Logger:  logger,
	})
	if err != nil {
		logger.Fatal("Failed to create client", zap.Error(err))
	}
	defer client.Close()

	ctx := context.Background()

	// Health check
	fmt.Println("Checking V-JEPA service health...")
	health, err := client.HealthCheck(ctx)
	if err != nil {
		logger.Fatal("Health check failed", zap.Error(err))
	}

	fmt.Println("\n=== V-JEPA Service Health ===")
	fmt.Printf("Healthy:        %t\n", health.Healthy)
	fmt.Printf("Model:          %s\n", health.ModelLoaded)
	fmt.Printf("Device:         %s\n", health.Device)
	fmt.Printf("Memory:         %d MB / %d MB\n", health.MemoryUsedMB, health.MemoryTotalMB)
	fmt.Printf("Avg Inference:  %.2f ms\n", health.AvgInferenceMS)

	if *healthOnly {
		return
	}

	// Compare frames if provided
	if *baseline != "" && *actual != "" {
		fmt.Printf("\nComparing frames:\n  Baseline: %s\n  Actual:   %s\n", *baseline, *actual)

		baselineData, err := os.ReadFile(*baseline)
		if err != nil {
			logger.Fatal("Failed to read baseline", zap.Error(err))
		}

		actualData, err := os.ReadFile(*actual)
		if err != nil {
			logger.Fatal("Failed to read actual", zap.Error(err))
		}

		start := time.Now()
		result, err := client.CompareFrames(ctx, baselineData, actualData, "test comparison")
		if err != nil {
			logger.Fatal("Comparison failed", zap.Error(err))
		}
		duration := time.Since(start)

		fmt.Println("\n=== Comparison Result ===")
		fmt.Printf("Similarity:     %.2f%%\n", result.SimilarityScore*100)
		fmt.Printf("Semantic Match: %t\n", result.SemanticMatch)
		fmt.Printf("Confidence:     %.2f%%\n", result.Confidence*100)
		fmt.Printf("Duration:       %s\n", duration.Round(time.Millisecond))
		fmt.Printf("Analysis:       %s\n", result.Analysis)

		if len(result.ChangedRegions) > 0 {
			fmt.Printf("\nChanged Regions: %d\n", len(result.ChangedRegions))
			for i, r := range result.ChangedRegions {
				fmt.Printf("  %d. (%d,%d) %dx%d - %s (%.2f)\n",
					i+1, r.X, r.Y, r.Width, r.Height, r.ChangeType, r.Significance)
			}
		}
	} else if !*healthOnly {
		fmt.Println("\nTo compare images, use:")
		fmt.Println("  --baseline <path> --actual <path>")
	}

	fmt.Println("\nV-JEPA integration test complete!")
}
