package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/BLAZED-sh/rpc-test/pkg/rpctest"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Define command line flags
	socketPath := flag.String("socket", "", "Path to the Ethereum node Unix domain socket")
	wsURL := flag.String("ws", "", "WebSocket URL of the Ethereum node (ws:// or wss://)")
	httpURL := flag.String("http", "", "HTTP URL of the Ethereum node (http:// or https://)")
	outputFormat := flag.String("format", "text", "Output format: text, markdown, or json")
	// Removed timeout parameter
	runSubscriptions := flag.Bool("subscriptions", false, "Run subscription tests (may take longer)")
	outputPath := flag.String("output", "", "Path to write the report (optional, default is stdout)")
	shareClient := flag.Bool("share-client", false, "Use a shared client for subscription tests instead of creating fresh connections")

	// Logging related flags
	logLevel := flag.String("log-level", "info", "Log level: trace, debug, info, error")
	noColor := flag.Bool("no-color", false, "Disable colorized output")

	// Parse flags
	flag.Parse()

	// Configure zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.TimestampFunc = func() time.Time { return time.Now().UTC() }

	// Set up pretty logging by default
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	if *noColor {
		output.NoColor = true
	}

	// Set the global logger
	log.Logger = zerolog.New(output).With().Timestamp().Logger()

	// Set log level
	switch *logLevel {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Warn().Str("level", *logLevel).Msg("Unknown log level, defaulting to info")
	}

	// Validate endpoint - one of socket path, websocket URL, or HTTP URL is required
	endpoint := *socketPath
	endpointType := "socket"

	if endpoint == "" && *wsURL == "" && *httpURL == "" {
		log.Error().Msg("Either socket path, WebSocket URL, or HTTP URL is required")
		flag.Usage()
		os.Exit(1)
	}

	if endpoint == "" && *wsURL != "" {
		endpoint = *wsURL
		endpointType = "websocket"
	} else if endpoint == "" && *httpURL != "" {
		endpoint = *httpURL
		endpointType = "http"
	}

	// Create context without timeout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create test runner
	log.Info().Str("endpoint", endpoint).Str("type", endpointType).Msg("Creating test runner")
	runner, err := rpctest.NewTestRunner(endpoint)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create test runner")
	}
	defer runner.Close()

	log.Info().Str("endpoint", endpoint).Str("type", endpointType).Msg("Starting Ethereum RPC tests")

	// Run regular RPC tests
	log.Info().Msg("Running regular RPC tests")
	results := runner.RunTests(ctx)
	
	// Initialize allResults with the regular RPC test results
	allResults := results

	// Run subscription tests if requested
	if *runSubscriptions {
		// Give a warning if using HTTP for subscriptions (they won't work)
		if endpointType == "http" {
			log.Warn().Msg("Subscription tests were requested but you're using HTTP transport, which doesn't support subscriptions")
			log.Warn().Msg("Consider using WebSocket (--ws) or Unix socket (--socket) instead for subscription tests")
		}

		log.Info().
			Bool("share_client", *shareClient).
			Msg("Running subscription tests (this may take a while)")

		// Pass the shareClient flag to control whether to use fresh clients
		subResults := runner.RunSubscriptionTests(ctx, *shareClient)
		log.Info().Int("count", len(subResults)).Msg("Completed subscription tests")
		
		// Add subscription results to allResults
		allResults = append(allResults, subResults...)
	}

	// Generate report with all results
	log.Info().Msg("Generating report")
	reportGen := rpctest.NewReportGenerator(allResults)

	var report string
	switch *outputFormat {
	case "text":
		report = reportGen.GenerateTextReport()
	case "markdown":
		report = reportGen.GenerateMarkdownReport()
	case "json":
		report = reportGen.GenerateJSONReport()
	default:
		log.Fatal().Str("format", *outputFormat).Msg("Unknown output format")
	}

	// Output report
	if *outputPath == "" {
		// Print to stdout
		fmt.Println(report)
	} else {
		// Write to file
		err = os.WriteFile(*outputPath, []byte(report), 0644)
		if err != nil {
			log.Fatal().Err(err).Str("path", *outputPath).Msg("Failed to write report to file")
		}
		log.Info().Str("path", *outputPath).Msg("Report written to file")
	}
}