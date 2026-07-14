package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MobasirSarkar/MediaEngine/internal/config"
	sqlc "github.com/MobasirSarkar/MediaEngine/internal/db/sqlc"
	"github.com/MobasirSarkar/MediaEngine/internal/events"
	"github.com/MobasirSarkar/MediaEngine/internal/hub"
	"github.com/MobasirSarkar/MediaEngine/internal/jobs"
	logpkg "github.com/MobasirSarkar/MediaEngine/internal/log"
	"github.com/MobasirSarkar/MediaEngine/internal/server"
	"github.com/MobasirSarkar/MediaEngine/internal/storage"
	st "github.com/MobasirSarkar/MediaEngine/internal/store"
	"github.com/MobasirSarkar/MediaEngine/internal/upload"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/compress"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/metadata"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/thumbnail"
	"github.com/MobasirSarkar/MediaEngine/internal/workers/transcode"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPipelineE2E(t *testing.T) {
	// 1. Load config and skip if no DSN is provided (e.g. running in empty environment)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	logger, err := logpkg.New("info", "console", "dev")
	if err != nil {
		t.Fatalf("failed to initialize logger: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = logpkg.With(ctx, logger)

	// Connect to postgres pool
	pool, err := pgxpool.New(ctx, cfg.DB.DSN)
	if err != nil {
		t.Skipf("skipping test; postgres database connection failed: %v", err)
		return
	}
	defer pool.Close()
	q := sqlc.New(pool)

	// Clean tables before test run to ensure a clean slate
	_, _ = pool.Exec(ctx, "DELETE FROM tasks")
	_, _ = pool.Exec(ctx, "DELETE FROM jobs")
	_, _ = pool.Exec(ctx, "DELETE FROM chunks")
	_, _ = pool.Exec(ctx, "DELETE FROM uploads")

	// Connect to NATS
	bus, err := events.New(ctx, events.Config{URL: cfg.NATS.URL, Stream: cfg.NATS.Stream})
	if err != nil {
		t.Skipf("skipping test; NATS connection failed: %v", err)
		return
	}
	defer func() { _ = bus.Close(ctx) }()
	_ = bus.EnsureStream(ctx)

	// Connect to S3 Storage
	store, err := storage.NewS3(ctx, storage.Config{
		Endpoint:       cfg.S3.Endpoint,
		PublicEndpoint: cfg.S3.PublicEndpoint,
		Region:         cfg.S3.Region,
		AccessKey:      cfg.S3.AccessKey,
		SecretKey:      cfg.S3.SecretKey,
		BucketUploads:  cfg.S3.BucketUploads,
		BucketMedia:    cfg.S3.BucketMedia,
		PresignTTL:     cfg.S3.PresignTTL,
	})
	if err != nil {
		t.Fatalf("failed S3 storage setup: %v", err)
	}
	_ = store.EnsureBuckets(ctx, cfg.S3.BucketUploads, cfg.S3.BucketMedia)

	// 2. Start Job Orchestrator & Workers
	jobsRepo := st.NewJobs(q)
	jobSvc := jobs.NewService(jobsRepo, bus)
	h := hub.New(64)

	orch := jobs.NewOrchestrator(jobSvc, bus, h)
	go func() { _ = orch.Run(ctx) }()

	// Start workers programmatically
	go func() { _ = metadata.New(metadata.Config{Bucket: cfg.S3.BucketUploads}, jobsRepo, bus, store).Run(ctx) }()
	go func() { _ = thumbnail.Run(ctx, jobsRepo, bus, store, cfg.S3.BucketUploads, cfg.S3.BucketMedia) }()
	go func() { _ = transcode.Run(ctx, jobsRepo, bus, store, cfg.S3.BucketUploads, cfg.S3.BucketMedia) }()
	go func() { _ = compress.Run(ctx, jobsRepo, bus) }()

	// 3. Set up Gin engine and API endpoints
	uploadSvc := upload.NewService(
		upload.Config{SessionTTL: cfg.Up.SessionTTL, Bucket: cfg.S3.BucketUploads},
		st.NewUploads(q), store, bus,
	)
	uploadH := upload.NewHandlers(uploadSvc)
	jobH := jobs.NewHandlers(jobSvc, h)

	engine := gin.New()
	server.Routes(engine, server.Deps{
		Logger:  logger,
		Upload:  uploadH,
		Jobs:    jobH,
		Storage: store,
		Config:  cfg,
	})

	// 4. Generate a valid PNG image and pad it to 5MB (which is required by the chunk-size validation)
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var imgBuf bytes.Buffer
	if err := png.Encode(&imgBuf, img); err != nil {
		t.Fatalf("failed to encode mock image: %v", err)
	}
	pngBytes := imgBuf.Bytes()
	chunkSize := 5 * 1024 * 1024 // 5 MB minimum size
	payloadBytes := make([]byte, chunkSize)
	copy(payloadBytes, pngBytes) // Write valid PNG bytes at the start, trailing elements remain as zero padding

	// Compute SHA-256 checksum of the chunk
	checksum := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // default hash value fallback
	// Natively calculate SHA-256 for real verification
	importSha := func() string {
		importCrypto := func() []byte {
			// Using basic sum for checksum hashing
			h := sha256Sum(payloadBytes)
			return h
		}
		return fmt.Sprintf("%x", importCrypto())
	}()
	checksum = importSha

	// 5. Execute HTTP E2E Pipeline
	// Step A: Create Upload Session
	initReqBody, _ := json.Marshal(map[string]any{
		"owner_id":     "e2e_tester",
		"filename":     "mock_file.png",
		"content_type": "image/png",
		"total_size":   len(payloadBytes),
		"chunk_size":   len(payloadBytes),
	})
	w := performRequest(engine, "POST", "/uploads", bytes.NewBuffer(initReqBody))
	if w.Code != 201 {
		t.Fatalf("expected upload creation 201, got: %d (body: %s)", w.Code, w.Body.String())
	}

	var initResp struct {
		Success bool `json:"success"`
		Data    struct {
			UploadID  string   `json:"upload_id"`
			ChunkURLs []string `json:"chunk_urls"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &initResp)
	uploadIDStr := initResp.Data.UploadID

	// Step B: Upload Chunk to S3 pre-signed URL (using HTTP PUT directly)
	putURL := initResp.Data.ChunkURLs[0]
	// Replace localhost with s3 endpoint host if running inside Docker environment,
	// but since our test runs in host network, localhost should connect natively to MinIO.
	putReq, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(payloadBytes))
	if err != nil {
		t.Fatalf("failed to create PUT request: %v", err)
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("failed S3 PUT upload: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != 200 && putResp.StatusCode != 204 {
		t.Fatalf("expected S3 PUT success, got: %d", putResp.StatusCode)
	}

	// Step C: Register Chunk with API
	regReqBody, _ := json.Marshal(map[string]any{
		"size":     len(payloadBytes),
		"checksum": checksum,
	})
	w = performRequest(engine, "POST", fmt.Sprintf("/uploads/%s/chunks/0", uploadIDStr), bytes.NewBuffer(regReqBody))
	if w.Code != 200 {
		t.Fatalf("expected chunk registration success 200, got: %d (body: %s)", w.Code, w.Body.String())
	}

	// Step D: Finalize Upload Composition
	w = performRequest(engine, "POST", fmt.Sprintf("/uploads/%s/complete", uploadIDStr), bytes.NewBuffer([]byte("{}")))
	if w.Code != 200 {
		t.Fatalf("expected upload completion success 200, got: %d (body: %s)", w.Code, w.Body.String())
	}

	// Step E: Wait and Poll for job completion
	t.Logf("Upload completed successfully. Polling database for background job execution...")
	var job modelJob
	var tasks []modelTask
	success := false

	for range 20 { // wait up to 10 seconds
		time.Sleep(500 * time.Millisecond)

		w = performRequest(engine, "GET", fmt.Sprintf("/jobs/upload/%s", uploadIDStr), nil)
		if w.Code != 200 {
			continue // job entity not created yet
		}

		var jobResp struct {
			Success bool `json:"success"`
			Data    struct {
				Job   modelJob    `json:"job"`
				Tasks []modelTask `json:"tasks"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &jobResp)
		job = jobResp.Data.Job
		tasks = jobResp.Data.Tasks

		if job.Status == "completed" || job.Status == "finished" {
			success = true
			break
		}
		if job.Status == "failed" {
			t.Fatalf("job processing failed: %s", job.ErrorMsg)
		}
	}

	if !success {
		t.Fatalf("timed out waiting for job completion. Current job status: %s. Tasks: %+v", job.Status, tasks)
	}

	// Assertions: Verify task outputs exist in database results
	t.Logf("E2E Job completed! Verifying task outputs...")
	for _, task := range tasks {
		if task.Status != "completed" {
			t.Errorf("task %s failed, status: %s, error: %s", task.Kind, task.Status, task.ErrorMsg)
		}
		if len(task.Result) > 0 {
			decoded, err := base64.StdEncoding.DecodeString(task.Result)
			if err != nil {
				t.Errorf("failed to decode task %s result: %v", task.Kind, err)
			}
			t.Logf("Task %s Result: %s", task.Kind, string(decoded))
		}
	}

	// Step F: Verify S3 chunk cleanup
	t.Logf("Verifying source chunks cleanup in S3...")
	// Wait a bit for the cleanup worker to process NATS event
	time.Sleep(1 * time.Second)
	_, err = store.Stat(ctx, cfg.S3.BucketUploads, fmt.Sprintf("uploads/%s/chunks/0", uploadIDStr))
	if err == nil {
		t.Errorf("expected source chunk 0 to be cleaned up from S3, but it still exists")
	} else {
		t.Logf("Source chunks successfully cleaned up from S3!")
	}
}

// Helpers
func performRequest(r *gin.Engine, method, path string, body io.Reader) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}

func sha256Sum(data []byte) []byte {
	// Standard simple checksum
	importC := func() [32]byte {
		// Import crypto/sha256 dynamically
		return sha256SumBlock(data)
	}
	arr := importC()
	return arr[:]
}

func sha256SumBlock(data []byte) [32]byte {
	return sha256.Sum256(data)
}

type modelJob struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	ErrorMsg  string `json:"error_msg"`
	CreatedAt string `json:"created_at"`
}

type modelTask struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Result   string `json:"result"`
	ErrorMsg string `json:"error_msg"`
}
