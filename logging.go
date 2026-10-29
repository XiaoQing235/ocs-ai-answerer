package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type responseResult struct {
	Status    string `json:"status"`
	Model     string `json:"model"`
	Reasoning *struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
	Usage  *responseUsage `json:"usage"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

type responseUsage struct {
	InputTokens  *int `json:"input_tokens"`
	OutputTokens *int `json:"output_tokens"`
	TotalTokens  *int `json:"total_tokens"`
	InputDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails *struct {
		ReasoningTokens *int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

type requestMetrics struct {
	started         time.Time
	first           time.Time
	completed       time.Time
	model           string
	reasoningEffort string
	usage           *responseUsage
}

type answerLog struct {
	Question          string   `json:"question"`
	Type              string   `json:"type"`
	OCSTokenPrefix    string   `json:"ocs_token_prefix"`
	RequestUnix       int64    `json:"request_unix"`
	FirstTokenMS      *float64 `json:"first_token_ms"`
	FirstToCompleteMS *float64 `json:"first_to_complete_ms"`
	TotalMS           float64  `json:"total_ms"`
	Model             string   `json:"model"`
	ReasoningEffort   string   `json:"reasoning_effort"`
	Status            string   `json:"status"`
	InputTokens       *int     `json:"input_tokens"`
	OutputTokens      *int     `json:"output_tokens"`
	CachedInputTokens *int     `json:"cached_input_tokens"`
	ReasoningTokens   *int     `json:"reasoning_tokens"`
	TotalTokens       *int     `json:"total_tokens"`
}

func (m *requestMetrics) entry(q question, c config, token, status string, end time.Time) answerLog {
	prefix := []rune(token)
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	entry := answerLog{Question: q.Title, Type: q.Type, OCSTokenPrefix: string(prefix), RequestUnix: m.started.Unix(), TotalMS: end.Sub(m.started).Seconds() * 1000, Model: c.model, ReasoningEffort: c.reasoningEffort, Status: status}
	if m.model != "" {
		entry.Model = m.model
	}
	if m.reasoningEffort != "" {
		entry.ReasoningEffort = m.reasoningEffort
	}
	if !m.first.IsZero() {
		first := m.first.Sub(m.started).Seconds() * 1000
		entry.FirstTokenMS = &first
		if !m.completed.IsZero() {
			duration := m.completed.Sub(m.first).Seconds() * 1000
			entry.FirstToCompleteMS = &duration
		}
	}
	if m.usage != nil {
		entry.InputTokens, entry.OutputTokens, entry.TotalTokens = m.usage.InputTokens, m.usage.OutputTokens, m.usage.TotalTokens
		if m.usage.InputDetails != nil {
			entry.CachedInputTokens = m.usage.InputDetails.CachedTokens
		}
		if m.usage.OutputDetails != nil {
			entry.ReasoningTokens = m.usage.OutputDetails.ReasoningTokens
		}
	}
	return entry
}

type requestLogger struct {
	mu   sync.Mutex
	file *os.File
}

func (l *requestLogger) write(entry answerLog) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return json.NewEncoder(l.file).Encode(entry)
}

func readResponseStream(body io.Reader, metrics *requestMetrics) (responseResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var data []string
	dataSize := 0
	var result responseResult
	terminal := false
	dispatch := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = nil
		dataSize = 0
		if payload == "[DONE]" {
			return nil
		}
		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return err
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" && metrics.first.IsZero() {
				metrics.first = time.Now()
			}
		case "response.completed", "response.incomplete", "response.failed":
			if err := json.Unmarshal(event.Response, &result); err != nil {
				return err
			}
			metrics.completed = time.Now()
			metrics.model, metrics.usage = result.Model, result.Usage
			if result.Reasoning != nil {
				metrics.reasoningEffort = result.Reasoning.Effort
			}
			terminal = true
		case "error":
			return errors.New("upstream stream error")
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return result, err
			}
			if terminal {
				return result, nil
			}
		} else if strings.HasPrefix(line, "data:") {
			dataSize += len(line)
			if dataSize > 2<<20 {
				return result, errors.New("upstream event too large")
			}
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	if err := dispatch(); err != nil {
		return result, err
	}
	if !terminal {
		return result, errors.New("upstream stream ended without completion")
	}
	return result, nil
}
