package model

type MediaKind string

const (
	MediaImage MediaKind = "image"
	MediaVideo MediaKind = "video"
	MediaOther MediaKind = "other"
)

type MediaAsset struct {
	UploadID string    `json:"upload_id"`
	Kind     MediaKind `json:"kind"`
	Width    int       `json:"width,omitempty"`
	Height   int       `json:"height,omitempty"`
	Duration float64   `json:"duration_seconds,omitempty"`
	Codec    string    `json:"codec,omitempty"`
	Bitrate  int64     `json:"bitrate,omitempty"`
	FPS      float64   `json:"fps,omitempty"`
	HasAudio bool      `json:"has_audio"`
}
