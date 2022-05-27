package sinkcluster

import (
	"bufio"
	"encoding/json"
	"github.com/clarkduvall/hyperloglog"
	"io"
	"time"
)

func NewClusterCounter(from time.Time, to time.Time) *ClusterCounter {
	return &ClusterCounter{from: from, to: to}
}

type ClusterCounter struct {
	from time.Time
	to   time.Time
}

type ClusterCountResult struct {
	Sum           uint64
	ChunkIncluded int64
}

func (c ClusterCounter) Count(reader io.Reader) (*ClusterCountResult, error) {
	result := ClusterCountResult{}
	counter, err := hyperloglog.NewPlus(18)
	if err != nil {
		return nil, err
	}
	inputScanner := bufio.NewScanner(reader)
	for inputScanner.Scan() {
		inputLine := inputScanner.Bytes()
		sinkInfo := SinkEntry{}
		if err := json.Unmarshal(inputLine, &sinkInfo); err != nil {
			return nil, err
		}

		if (sinkInfo.RecordingStart.Before(c.from) && !sinkInfo.RecordingStart.Equal(c.from)) ||
			sinkInfo.RecordingEnd.After(c.to) {
			continue
		}

		restoredCounter, err := hyperloglog.NewPlus(18)
		if err != nil {
			return nil, err
		}
		err = restoredCounter.GobDecode(sinkInfo.Recorded)
		if err != nil {
			return nil, err
		}
		result.ChunkIncluded++
		err = counter.Merge(restoredCounter)
		if err != nil {
			return nil, err
		}
	}
	result.Sum = counter.Count()
	return &result, nil
}
