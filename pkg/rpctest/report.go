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

	// Sort results by test name
	sort.Slice(rg.Results, func(i, j int) bool {
		return rg.Results[i].TestName < rg.Results[j].TestName
	})

	var sb strings.Builder
	sb.WriteString("ETHEREUM RPC BENCHMARK RESULTS\n")
	sb.WriteString("============================\n\n")

	// Calculate overall stats
	total := len(rg.Results)
	successful := 0
	var totalDuration time.Duration
	var totalReqSize, totalRespSize int

	for _, result := range rg.Results {
		if result.Success {
			successful++
		}
		totalDuration += result.Duration
		totalReqSize += result.RequestSize
		totalRespSize += result.ResponseSize
	}

	// Write summary
	sb.WriteString(fmt.Sprintf("Total tests:     %d\n", total))
	sb.WriteString(fmt.Sprintf("Successful:      %d (%.1f%%)\n", successful, float64(successful)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("Failed:          %d (%.1f%%)\n", total-successful, float64(total-successful)/float64(total)*100))
	sb.WriteString(fmt.Sprintf("Total duration:  %s\n", totalDuration))
	sb.WriteString(fmt.Sprintf("Avg. duration:   %s\n", totalDuration/time.Duration(total)))
	sb.WriteString(fmt.Sprintf("Total request:   %d bytes\n", totalReqSize))
	sb.WriteString(fmt.Sprintf("Total response:  %d bytes\n", totalRespSize))
	sb.WriteString("\n")

	// Write detailed results
	sb.WriteString("DETAILED RESULTS\n")
	sb.WriteString("================\n\n")

	for _, result := range rg.Results {
		// Test name and status
		if result.Success {
			sb.WriteString(fmt.Sprintf("✅ %s - SUCCESS\n", result.TestName))
		} else {
			sb.WriteString(fmt.Sprintf("❌ %s - FAILED\n", result.TestName))
		}

		// Metrics
		sb.WriteString(fmt.Sprintf("   Duration:     %s\n", result.Duration))
		sb.WriteString(fmt.Sprintf("   Request size: %d bytes\n", result.RequestSize))
		sb.WriteString(fmt.Sprintf("   Response size: %d bytes\n", result.ResponseSize))

		// Status message or error
		if result.Success {
			sb.WriteString(fmt.Sprintf("   Result: %s\n", result.StatusMessage))
		} else {
			sb.WriteString(fmt.Sprintf("   Error: %s\n", result.Error))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// GenerateMarkdownReport generates a markdown report
func (rg *ReportGenerator) GenerateMarkdownReport() string {
	if len(rg.Results) == 0 {
		return "# Ethereum RPC Benchmark Results\n\nNo benchmark results to report."
	}

	// Sort results by test name
	sort.Slice(rg.Results, func(i, j int) bool {
		return rg.Results[i].TestName < rg.Results[j].TestName
	})

	var sb strings.Builder
	sb.WriteString("# Ethereum RPC Benchmark Results\n\n")

	// Calculate overall stats
	total := len(rg.Results)
	successful := 0
	var totalDuration time.Duration
	var totalReqSize, totalRespSize int

	for _, result := range rg.Results {
		if result.Success {
			successful++
		}
		totalDuration += result.Duration
		totalReqSize += result.RequestSize
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
	sb.WriteString(fmt.Sprintf("| Total request size | %d bytes |\n", totalReqSize))
	sb.WriteString(fmt.Sprintf("| Total response size | %d bytes |\n", totalRespSize))
	sb.WriteString("\n")

	// Write detailed results in table format
	sb.WriteString("## Detailed Results\n\n")
	sb.WriteString("| Test | Status | Duration | Request | Response | Message |\n")
	sb.WriteString("|------|--------|----------|---------|----------|--------|\n")

	for _, result := range rg.Results {
		status := "✅"
		message := result.StatusMessage
		if !result.Success {
			status = "❌"
			message = result.Error
		}

		// Escape pipe characters in the message
		message = strings.ReplaceAll(message, "|", "\\|")
		
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d B | %d B | %s |\n",
			result.TestName,
			status,
			result.Duration,
			result.RequestSize,
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
	var totalReqSize, totalRespSize int

	for _, result := range rg.Results {
		if result.Success {
			successful++
		}
		totalDuration += result.Duration
		totalReqSize += result.RequestSize
		totalRespSize += result.ResponseSize
	}

	sb.WriteString(fmt.Sprintf("    \"totalTests\": %d,\n", total))
	sb.WriteString(fmt.Sprintf("    \"successfulTests\": %d,\n", successful))
	sb.WriteString(fmt.Sprintf("    \"failedTests\": %d,\n", total-successful))
	sb.WriteString(fmt.Sprintf("    \"totalDurationMs\": %d,\n", totalDuration.Milliseconds()))
	sb.WriteString(fmt.Sprintf("    \"avgDurationMs\": %d,\n", totalDuration.Milliseconds()/int64(total)))
	sb.WriteString(fmt.Sprintf("    \"totalRequestBytes\": %d,\n", totalReqSize))
	sb.WriteString(fmt.Sprintf("    \"totalResponseBytes\": %d\n", totalRespSize))
	sb.WriteString("  },\n")
	
	sb.WriteString("  \"tests\": [\n")
	
	for i, result := range rg.Results {
		sb.WriteString("    {\n")
		sb.WriteString(fmt.Sprintf("      \"name\": \"%s\",\n", result.TestName))
		sb.WriteString(fmt.Sprintf("      \"success\": %t,\n", result.Success))
		sb.WriteString(fmt.Sprintf("      \"durationMs\": %d,\n", result.Duration.Milliseconds()))
		sb.WriteString(fmt.Sprintf("      \"requestBytes\": %d,\n", result.RequestSize))
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