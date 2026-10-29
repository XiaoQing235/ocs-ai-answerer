package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMultipleOCSTokens(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response(`["1"]`)) }))
	defer upstream.Close()
	c := config{endpoint: upstream.URL, model: "test", apiKey: "test", ocsTokens: []string{"first", "second"}}
	for _, tc := range []struct {
		token  string
		status int
	}{{"first", 200}, {"second", 200}, {"other", 401}} {
		req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(`{"title":"question","type":"single","options":"A. one"}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		handler(c, upstream.Client()).ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Errorf("token %q: got HTTP %d", tc.token, w.Code)
		}
	}
}

func TestOCSTokenConfig(t *testing.T) {
	for _, tc := range []struct {
		data  string
		valid bool
	}{
		{`{"base_url":"https://example.com/v1","api_key":"test","model":"test","ocs_token":["one","two"]}`, true},
		{`{"base_url":"https://example.com/v1","api_key":"test","model":"test","ocs_token":["one",""]}`, false},
		{`{"base_url":"https://example.com/v1","api_key":"test","model":"test","token":"old"}`, false},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := loadConfig(path)
		if (err == nil) != tc.valid {
			t.Errorf("valid=%v error=%v", tc.valid, err)
		}
	}
}

func TestLoggingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(fileConfig{BaseURL: "https://example.com/v1", APIKey: "test", Model: "test", OCSToken: []string{"one", "two"}, LogEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, _, err := loadConfig(path)
	if err != nil || !c.logging || len(c.ocsTokens) != 2 {
		t.Fatalf("logging config: %v", err)
	}
}

func TestUnauthorizedRequestIsNotLogged(t *testing.T) {
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "answers.jsonl"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	c := config{ocsTokens: []string{"allowed"}, logger: &requestLogger{file: file}}
	req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer rejected")
	w := httptest.NewRecorder()
	handler(c, http.DefaultClient).ServeHTTP(w, req)
	info, err := file.Stat()
	if err != nil || w.Code != http.StatusUnauthorized || info.Size() != 0 {
		t.Fatalf("status %d, stat error %v", w.Code, err)
	}
}

func TestLoggingStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !request.Stream {
			t.Error("stream not requested")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"{\"}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"actual-model\",\"reasoning\":{\"effort\":\"medium\"},\"usage\":{\"input_tokens\":100,\"input_tokens_details\":{\"cached_tokens\":25},\"output_tokens\":30,\"output_tokens_details\":{\"reasoning_tokens\":10},\"total_tokens\":130},\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"{\\\"answers\\\":[\\\"1\\\"]}\"}]}]}}\n\n")
	}))
	defer upstream.Close()
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "answers.jsonl"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	c := config{endpoint: upstream.URL, model: "configured-model", apiKey: "secret-for-test", ocsTokens: []string{"first-token-123456", "second-token-abcdef"}, reasoningEffort: "high", logger: &requestLogger{file: file}}
	req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(`{"title":"question","type":"single","options":"A. one"}`))
	req.Header.Set("Authorization", "Bearer second-token-abcdef")
	w := httptest.NewRecorder()
	handler(c, upstream.Client()).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var entry answerLog
	if err := json.NewDecoder(file).Decode(&entry); err != nil {
		t.Fatal(err)
	}
	if entry.Question != "question" || entry.OCSTokenPrefix != "second-token" || entry.Model != "actual-model" || entry.ReasoningEffort != "medium" || entry.Status != "success" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.RequestUnix <= 0 || entry.FirstTokenMS == nil || entry.FirstToCompleteMS == nil || *entry.FirstTokenMS < 0 || *entry.FirstToCompleteMS < 0 || entry.TotalMS+0.001 < *entry.FirstTokenMS+*entry.FirstToCompleteMS {
		t.Fatalf("invalid timing: %+v", entry)
	}
	if entry.InputTokens == nil || *entry.InputTokens != 100 || entry.OutputTokens == nil || *entry.OutputTokens != 30 || entry.CachedInputTokens == nil || *entry.CachedInputTokens != 25 || entry.ReasoningTokens == nil || *entry.ReasoningTokens != 10 || entry.TotalTokens == nil || *entry.TotalTokens != 130 {
		t.Fatalf("invalid usage: %+v", entry)
	}
}

func TestLoggingTruncatedStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\n")
	}))
	defer upstream.Close()
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "answers.jsonl"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	c := config{endpoint: upstream.URL, model: "test", apiKey: "test", ocsTokens: []string{"test"}, logger: &requestLogger{file: file}}
	req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(`{"title":"question","type":"single","options":"A. one"}`))
	req.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	handler(c, upstream.Client()).ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status %d", w.Code)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	var entry answerLog
	if err := json.NewDecoder(file).Decode(&entry); err != nil {
		t.Fatal(err)
	}
	if entry.Status != "upstream_error" || entry.FirstTokenMS == nil || entry.FirstToCompleteMS != nil || entry.InputTokens != nil {
		t.Fatalf("unexpected failure entry: %+v", entry)
	}
}

func TestIncompleteStreamUsage(t *testing.T) {
	metrics := &requestMetrics{started: time.Now()}
	stream := "data: {\"type\":\"response.incomplete\",\n" +
		"data: \"response\":{\"status\":\"incomplete\",\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n"
	result, err := readResponseStream(strings.NewReader(stream), metrics)
	if err != nil || result.Status != "incomplete" {
		t.Fatalf("status %q: %v", result.Status, err)
	}
	entry := metrics.entry(question{}, config{}, "", "upstream_error", time.Now())
	if entry.FirstTokenMS != nil || entry.FirstToCompleteMS != nil || entry.InputTokens == nil || *entry.InputTokens != 5 || entry.OutputTokens == nil || *entry.OutputTokens != 0 || entry.CachedInputTokens != nil {
		t.Fatalf("unexpected incomplete entry: %+v", entry)
	}
}

func TestConcurrentLogWrites(t *testing.T) {
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "answers.jsonl"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	logger := &requestLogger{file: file}
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := logger.write(answerLog{Question: "test"}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var entry answerLog
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Question != "test" {
			t.Fatal("invalid log line")
		}
		count++
	}
	if scanner.Err() != nil || count != 20 {
		t.Fatalf("got %d lines: %v", count, scanner.Err())
	}
}
