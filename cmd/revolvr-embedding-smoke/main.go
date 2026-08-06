package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/embedding"
)

const smokeSchemaVersion = "revolvr-embedding-smoke-v1"

type smokeReport struct {
	SchemaVersion    string                           `json:"schema_version"`
	Status           embedding.ServiceStatus          `json:"status"`
	Model            embedding.EmbeddingModelInfo     `json:"model"`
	Space            embedding.EmbeddingSpaceIdentity `json:"space"`
	VectorDimensions int                              `json:"vector_dimensions"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, output io.Writer) error {
	flags := flag.NewFlagSet("revolvr-embedding-smoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	text := flags.String("text", "where is task scheduling implemented?", "non-sensitive smoke query")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("embedding smoke: positional arguments are not supported")
	}
	dimensions, err := positiveIntEnvironment(getenv, "REVOLVR_EMBEDDING_DIMENSIONS")
	if err != nil {
		return err
	}
	timeout := embedding.DefaultTimeout
	if value := strings.TrimSpace(getenv("REVOLVR_EMBEDDING_TIMEOUT")); value != "" {
		timeout, err = time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return errors.New("embedding smoke: REVOLVR_EMBEDDING_TIMEOUT must be a positive Go duration")
		}
	}
	model := embedding.EmbeddingModelInfo{
		SchemaVersion:  embedding.ModelInfoSchemaVersion,
		ModelName:      strings.TrimSpace(getenv("REVOLVR_EMBEDDING_MODEL_NAME")),
		Revision:       strings.TrimSpace(getenv("REVOLVR_EMBEDDING_MODEL_REVISION")),
		Dimensions:     dimensions,
		Pooling:        strings.TrimSpace(getenv("REVOLVR_EMBEDDING_POOLING")),
		Normalization:  strings.TrimSpace(getenv("REVOLVR_EMBEDDING_NORMALIZATION")),
		Quantization:   strings.TrimSpace(getenv("REVOLVR_EMBEDDING_QUANTIZATION")),
		ArtifactSHA256: strings.TrimSpace(getenv("REVOLVR_EMBEDDING_ARTIFACT_SHA256")),
	}
	client, err := embedding.NewClient(embedding.Config{
		Endpoint:      strings.TrimSpace(getenv("REVOLVR_EMBEDDING_ENDPOINT")),
		ExpectedModel: model,
		Timeout:       timeout,
	})
	if err != nil {
		return fmt.Errorf("embedding smoke: %w", err)
	}
	status, err := client.Health(ctx)
	if err != nil {
		return fmt.Errorf("embedding smoke health: %w", err)
	}
	result, err := client.EmbedQuery(ctx, *text)
	if err != nil {
		return fmt.Errorf("embedding smoke query: %w", err)
	}
	if len(result.Value) != model.Dimensions {
		return fmt.Errorf("embedding smoke: vector dimension %d does not match pinned dimension %d", len(result.Value), model.Dimensions)
	}
	report := smokeReport{
		SchemaVersion:    smokeSchemaVersion,
		Status:           status,
		Model:            model,
		Space:            result.Space,
		VectorDimensions: len(result.Value),
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("embedding smoke: encode report: %w", err)
	}
	return nil
}

func positiveIntEnvironment(getenv func(string) string, name string) (int, error) {
	value := strings.TrimSpace(getenv(name))
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("embedding smoke: %s must be a positive integer", name)
	}
	return parsed, nil
}
