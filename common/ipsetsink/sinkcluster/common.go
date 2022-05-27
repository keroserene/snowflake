package sinkcluster

import "time"

type SinkEntry struct {
	RecordingStart time.Time `json:"recordingStart"`
	RecordingEnd   time.Time `json:"recordingEnd"`
	Recorded       []byte    `json:"recorded"`
}
