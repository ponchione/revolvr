package main

import (
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/embedding"
	"revolvr/internal/embeddingeval"
)

func main() {
	dimensions, err := strconv.Atoi(os.Getenv("REVOLVR_EMBEDDING_DIMENSIONS"))
	if err != nil || dimensions <= 0 {
		log.Fatal("REVOLVR_EMBEDDING_DIMENSIONS must be positive")
	}
	model := embedding.EmbeddingModelInfo{
		SchemaVersion: embedding.ModelInfoSchemaVersion, ModelName: os.Getenv("REVOLVR_EMBEDDING_MODEL_NAME"),
		Revision: os.Getenv("REVOLVR_EMBEDDING_MODEL_REVISION"), Dimensions: dimensions,
		Pooling: os.Getenv("REVOLVR_EMBEDDING_POOLING"), Normalization: os.Getenv("REVOLVR_EMBEDDING_NORMALIZATION"),
		Quantization: os.Getenv("REVOLVR_EMBEDDING_QUANTIZATION"), ArtifactSHA256: os.Getenv("REVOLVR_EMBEDDING_ARTIFACT_SHA256"),
	}
	proxy, err := embeddingeval.New(embeddingeval.Config{
		BackendEndpoint: strings.TrimSpace(os.Getenv("REVOLVR_EVAL_BACKEND_ENDPOINT")),
		BackendModel:    strings.TrimSpace(os.Getenv("REVOLVR_EVAL_BACKEND_MODEL")), Model: model,
		DocumentPrefix: os.Getenv("REVOLVR_EVAL_DOCUMENT_PREFIX"), QueryPrefix: os.Getenv("REVOLVR_EVAL_QUERY_PREFIX"),
	})
	if err != nil {
		log.Fatal(err)
	}
	address := strings.TrimSpace(os.Getenv("REVOLVR_EVAL_LISTEN"))
	if address == "" {
		address = "127.0.0.1:18080"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	if !listener.Addr().(*net.TCPAddr).IP.IsLoopback() {
		_ = listener.Close()
		log.Fatal(errors.New("evaluation adapter must listen on loopback"))
	}
	server := &http.Server{Handler: proxy.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 65 * time.Second, WriteTimeout: 65 * time.Second, IdleTimeout: 30 * time.Second}
	log.Printf("listening on %s", listener.Addr())
	log.Fatal(server.Serve(listener))
}
