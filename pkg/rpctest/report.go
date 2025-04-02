package rpctest

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ReportGenerator handles generating reports from benchmark results
type ReportGenerator struct {
	Results []BenchmarkResult
}

// NewReportGenerator creates a new report generator with the given results
func NewReportGenerator(results []BenchmarkResult) *ReportGenerator {
	return &ReportGenerator{
		Results: results,
	}
}

// GenerateTextReport generates a plain text report
func (rg *ReportGenerator) GenerateTextReport() string {
	if len(rg.Results) == 0 {
		return "No benchmark results to report."
	}

	// Sort results: standard RPC tests first, then subscription tests
	sort.Slice(rg.Results, func(i, j int) bool {
		// If one is a subscription test and the other isn't, put standard tests first
		isSub1 := strings.HasPrefix(rg.Results[i].TestName, "eth_") && strings.Contains(rg.Results[i].TestName, "logs")
		isSub2 := strings.HasPrefix(rg.Results[j].TestName, "eth_") && strings.Contains(rg.Results[j].TestName, "logs")

		if isSub1 && !isSub2 {
			return false // i is subscription, j is not, so j comes first
		}
		if !isSub1 && isSub2 {
			return true // i is not subscription, j is, so i comes first
		}

		// Otherwise sort by name
		return rg.Results[i].TestName < rg.Results[j].TestName
	})

	var sb strings.Builder
	sb.WriteString("ETHEREUM RPC BENCHMARK RESULTS\n")
	sb.WriteString("============================\n\n")

	// Calculate overall stats
	total := len(rg.Results)
	successful := 0
	var totalDuration time.Duration
	var totalRespSize int

	for _, result := range rg.Results {
		if result.Success {
			successful++
		}
		totalDuration += result.Duration
		totalRespSize += result.ResponseSize
	}

	// Write summary
	sb.WriteString(fmt.Sprintf("Total tests:     %d\n", total))
	sb.WriteString(fmt.Sprintf("Successful:      %d (%.1f%%)\n", successful, float64(successful)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("Failed:          %d (%.1f%%)\n", total-successful, float64(total-successful)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("Total duration:  %s\n", totalDuration))
	sb.WriteString(fmt.Sprintf("Avg. duration:   %s\n", totalDuration/time.Duration(total)))
	sb.WriteString(fmt.Sprintf("Total response:  %d bytes\n", totalRespSize))
	sb.WriteString("\n")

	// Write detailed results
	sb.WriteString("DETAILED RESULTS\n")
	sb.WriteString("================\n\n")

	// First write standard RPC tests
	sb.WriteString("Standard RPC Tests\n")
	sb.WriteString("-----------------\n\n")

	standardTestCount := 0
	for _, result := range rg.Results {
		// Skip subscription tests
		if strings.HasPrefix(result.TestName, "eth_") &&
			(strings.Contains(result.TestName, "logs_") || strings.Contains(result.TestName, "newHeads")) {
			continue
		}

		standardTestCount++

		// Test name and status
		if result.Success {
			sb.WriteString(fmt.Sprintf("✅ %s - SUCCESS\n", result.TestName))
		} else {
			sb.WriteString(fmt.Sprintf("❌ %s - FAILED\n", result.TestName))
		}

		// Metrics
		sb.WriteString(fmt.Sprintf("   Duration:     %s\n", result.Duration))
		sb.WriteString(fmt.Sprintf("   Response size: %d bytes\n", result.ResponseSize))

		// Status message or error
		if result.Success {
			sb.WriteString(fmt.Sprintf("   Result: %s\n", result.StatusMessage))
		} else {
			sb.WriteString(fmt.Sprintf("   Error: %s\n", result.Error))
		}

		sb.WriteString("\n")
	}

	// Then write subscription tests
	sb.WriteString("Subscription Tests\n")
	sb.WriteString("-----------------\n\n")

	subscriptionTestCount := 0
	for _, result := range rg.Results {
		// Only include subscription tests
		if !strings.HasPrefix(result.TestName, "eth_") ||
			(!strings.Contains(result.TestName, "logs_") && !strings.Contains(result.TestName, "newHeads")) {
			continue
		}

		subscriptionTestCount++

		// Test name and status
		if result.Success {
			sb.WriteString(fmt.Sprintf("✅ %s - SUCCESS\n", result.TestName))
		} else {
			sb.WriteString(fmt.Sprintf("❌ %s - FAILED\n", result.TestName))
		}

		// Metrics
		sb.WriteString(fmt.Sprintf("   Duration:     %s\n", result.Duration))

		// Status message or error
		if result.Success {
			sb.WriteString(fmt.Sprintf("   Result: %s\n", result.StatusMessage))
		} else {
			sb.WriteString(fmt.Sprintf("   Error: %s\n", result.Error))
		}

		sb.WriteString("\n")
	}

	// If no subscription tests were found, mention it
	if subscriptionTestCount == 0 {
		sb.WriteString("No subscription tests run.\n\n")
	}

	return sb.String()
}

// GenerateMarkdownReport generates a markdown report
func (rg *ReportGenerator) GenerateMarkdownReport() string {
	if len(rg.Results) == 0 {
		return "# Ethereum RPC Benchmark Results\n\nNo benchmark results to report."
	}

	// Sort results: standard RPC tests first, then subscription tests
	sort.Slice(rg.Results, func(i, j int) bool {
		// If one is a subscription test and the other isn't, put standard tests first
		isSub1 := strings.HasPrefix(rg.Results[i].TestName, "eth_") && strings.Contains(rg.Results[i].TestName, "logs")
		isSub2 := strings.HasPrefix(rg.Results[j].TestName, "eth_") && strings.Contains(rg.Results[j].TestName, "logs")

		if isSub1 && !isSub2 {
			return false // i is subscription, j is not, so j comes first
		}
		if !isSub1 && isSub2 {
			return true // i is not subscription, j is, so i comes first
		}

		// Otherwise sort by name
		return rg.Results[i].TestName < rg.Results[j].TestName
	})

	var sb strings.Builder
	sb.WriteString("# Ethereum RPC Benchmark Results\n\n")

	// Calculate overall stats
	total := len(rg.Results)
	successful := 0
	var totalDuration time.Duration
	var totalRespSize int

	for _, result := range rg.Results {
		if result.Success {
			successful++
		}
		totalDuration += result.Duration
		totalRespSize += result.ResponseSize
	}

	// Write summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total tests | %d |\n", total))
	sb.WriteString(fmt.Sprintf("| Successful | %d (%.1f%%) |\n", successful, float64(successful)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("| Failed | %d (%.1f%%) |\n", total-successful, float64(total-successful)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("| Total duration | %s |\n", totalDuration))
	sb.WriteString(fmt.Sprintf("| Avg. duration | %s |\n", totalDuration/time.Duration(total)))
	sb.WriteString(fmt.Sprintf("| Total response size | %d bytes |\n", totalRespSize))
	sb.WriteString("\n")

	// Write detailed results in table format
	sb.WriteString("## Detailed Results\n\n")
	sb.WriteString("| Test | Status | Duration | Response | Message |\n")
	sb.WriteString("|------|--------|--------------------|--------|\n")

	for _, result := range rg.Results {
		status := "✅"
		message := result.StatusMessage
		if !result.Success {
			status = "❌"
			message = result.Error
		}

		// Escape pipe characters in the message
		message = strings.ReplaceAll(message, "|", "\\|")

		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d B | %s |\n",
			result.TestName,
			status,
			result.Duration,
			result.ResponseSize,
			message,
		))
	}

	return sb.String()
}

// GenerateJSONReport generates a JSON report (returns a string with JSON content)
func (rg *ReportGenerator) GenerateJSONReport() string {
	// This would normally use json.Marshal, but for simplicity we're just creating a formatted string
	var sb strings.Builder
	sb.WriteString("{\n")
	sb.WriteString("  \"summary\": {\n")

	total := len(rg.Results)
	successful := 0
	var totalDuration time.Duration
	var totalRespSize int

	for _, result := range rg.Results {
		if result.Success {
			successful++
		}
		totalDuration += result.Duration
		totalRespSize += result.ResponseSize
	}

	sb.WriteString(fmt.Sprintf("    \"totalTests\": %d,\n", total))
	sb.WriteString(fmt.Sprintf("    \"successfulTests\": %d,\n", successful))
	sb.WriteString(fmt.Sprintf("    \"failedTests\": %d,\n", total-successful))
	sb.WriteString(fmt.Sprintf("    \"totalDurationMs\": %d,\n", totalDuration.Milliseconds()))
	sb.WriteString(fmt.Sprintf("    \"avgDurationMs\": %d,\n", totalDuration.Milliseconds()/int64(total)))
	sb.WriteString(fmt.Sprintf("    \"totalResponseBytes\": %d\n", totalRespSize))
	sb.WriteString("  },\n")

	sb.WriteString("  \"tests\": [\n")

	for i, result := range rg.Results {
		sb.WriteString("    {\n")
		sb.WriteString(fmt.Sprintf("      \"name\": \"%s\",\n", result.TestName))
		sb.WriteString(fmt.Sprintf("      \"success\": %t,\n", result.Success))
		sb.WriteString(fmt.Sprintf("      \"durationMs\": %d,\n", result.Duration.Milliseconds()))
		sb.WriteString(fmt.Sprintf("      \"responseBytes\": %d,\n", result.ResponseSize))

		if result.Success {
			sb.WriteString(fmt.Sprintf("      \"message\": \"%s\"\n", escapeJSON(result.StatusMessage)))
		} else {
			sb.WriteString(fmt.Sprintf("      \"error\": \"%s\"\n", escapeJSON(result.Error)))
		}

		if i < len(rg.Results)-1 {
			sb.WriteString("    },\n")
		} else {
			sb.WriteString("    }\n")
		}
	}

	sb.WriteString("  ]\n")
	sb.WriteString("}")

	return sb.String()
}

// escapeJSON escapes special characters in JSON strings
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
