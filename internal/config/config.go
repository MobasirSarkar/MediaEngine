package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App  App    `json:"app"`
	Log  Log    `json:"log"`
	DB   DB     `json:"db"`
	NATS NATS   `json:"nats"`
	S3   S3     `json:"s3"`
	Up   Up     `json:"uploads"`
	Path string `json:"-"`
}

type App struct {
	Env      string `json:"env"`
	HTTPAddr string `json:"http_addr"`
}

type Log struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

type DB struct {
	DSN     string `json:"dsn"`
	MaxOpen int    `json:"max_open"`
	MaxIdle int    `json:"max_idle"`
}

type NATS struct {
	URL    string `json:"url"`
	Stream string `json:"stream"`
}

type S3 struct {
	Endpoint       string        `json:"endpoint"`
	PublicEndpoint string        `json:"public_endpoint"`
	Region         string        `json:"region"`
	AccessKey      string        `json:"access_key"`
	SecretKey      string        `json:"secret_key"`
	BucketUploads  string        `json:"bucket_uploads"`
	BucketMedia    string        `json:"bucket_media"`
	PresignTTL     time.Duration `json:"presign_ttl"`
}

type Up struct {
	ChunkSizeMin int64         `json:"chunk_size_min"`
	ChunkSizeMax int64         `json:"chunk_size_max"`
	SessionTTL   time.Duration `json:"session_ttl"`
}

func Load() (*Config, error) {
	if err := loadEnv(".env"); err != nil {
		if err = loadEnv("../.env"); err != nil {
			_ = loadEnv("../../.env")
		}
	}
	c := &Config{
		App:  App{Env: "dev", HTTPAddr: ":8080"},
		Log:  Log{Level: "debug", Format: "console"},
		DB:   DB{DSN: "", MaxOpen: 20, MaxIdle: 5},
		NATS: NATS{URL: "nats://localhost:4222", Stream: "pipeline"},
		S3: S3{
			Endpoint:       "http://localhost:9000",
			PublicEndpoint: "",
			Region:         "us-east-1",
			BucketUploads:  "uploads",
			BucketMedia:    "media",
			PresignTTL:     15 * time.Minute,
		},
		Up: Up{
			ChunkSizeMin: 5 * 1024 * 1024,
			ChunkSizeMax: 50 * 1024 * 1024,
			SessionTTL:   24 * time.Hour,
		},
	}
	if path := os.Getenv("CONFIG_FILE"); path != "" {
		c.Path = path
	}
	applyEnv(c)
	if c.DB.DSN == "" {
		return nil, errors.New("DB_DSN required")
	}
	return c, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("APP_ENV"); v != "" {
		c.App.Env = v
	}
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		c.App.HTTPAddr = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		c.Log.Format = v
	}
	if v := os.Getenv("DB_DSN"); v != "" {
		c.DB.DSN = v
	}
	if v := os.Getenv("DB_MAX_OPEN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.DB.MaxOpen = n
		}
	}
	if v := os.Getenv("DB_MAX_IDLE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.DB.MaxIdle = n
		}
	}
	if v := os.Getenv("NATS_URL"); v != "" {
		c.NATS.URL = v
	}
	if v := os.Getenv("NATS_STREAM"); v != "" {
		c.NATS.Stream = v
	}
	if v := os.Getenv("S3_ENDPOINT"); v != "" {
		c.S3.Endpoint = v
	}
	if v := os.Getenv("S3_PUBLIC_ENDPOINT"); v != "" {
		c.S3.PublicEndpoint = v
	}
	if v := os.Getenv("S3_REGION"); v != "" {
		c.S3.Region = v
	}
	if v := os.Getenv("S3_ACCESS_KEY"); v != "" {
		c.S3.AccessKey = v
	}
	if v := os.Getenv("S3_SECRET_KEY"); v != "" {
		c.S3.SecretKey = v
	}
	if v := os.Getenv("S3_BUCKET_UPLOADS"); v != "" {
		c.S3.BucketUploads = v
	}
	if v := os.Getenv("S3_BUCKET_MEDIA"); v != "" {
		c.S3.BucketMedia = v
	}
	if v := os.Getenv("UPLOAD_SESSION_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Up.SessionTTL = d
		}
	}
}

func (c *Config) IsDev() bool { return strings.EqualFold(c.App.Env, "dev") }
func (c *Config) String() string {
	return fmt.Sprintf("env=%s addr=%s log=%s/%s", c.App.Env, c.App.HTTPAddr, c.Log.Level, c.Log.Format)
}

func loadEnv(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
