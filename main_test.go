package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResponseSettings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		effort string
		limit  int
	}{
		{name: "unset"},
		{name: "effort only", effort: "low"},
		{name: "limit only", limit: 2048},
		{name: "configured", effort: "high", limit: 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if _, ok := body["stream"]; ok {
					t.Error("stream enabled with logging disabled")
				}
				if tc.effort == "" {
					if _, ok := body["reasoning"]; ok {
						t.Error("unexpected reasoning")
					}
				} else {
					var reasoning struct {
						Effort string `json:"effort"`
					}
					if json.Unmarshal(body["reasoning"], &reasoning) != nil || reasoning.Effort != tc.effort {
						t.Error("incorrect reasoning effort")
					}
				}
				if tc.limit == 0 {
					if _, ok := body["max_output_tokens"]; ok {
						t.Error("unexpected token limit")
					}
				} else {
					var limit int
					if json.Unmarshal(body["max_output_tokens"], &limit) != nil || limit != tc.limit {
						t.Error("incorrect token limit")
					}
				}
				fmt.Fprint(w, response(`["1"]`))
			}))
			defer upstream.Close()
			path := filepath.Join(t.TempDir(), "config.json")
			data, err := json.Marshal(fileConfig{BaseURL: upstream.URL, APIKey: "test", Model: "test", OCSToken: []string{"test"}, ReasoningEffort: tc.effort, MaxOutputTokens: tc.limit})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			c, _, err := loadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			answer, err := answerQuestion(context.Background(), upstream.Client(), c, question{Title: "question", Type: "single", Options: "A. one"})
			if err != nil || answer != "one" {
				t.Fatalf("answer %q: %v", answer, err)
			}
		})
	}
}

func TestInvalidResponseSettings(t *testing.T) {
	for _, tc := range []struct {
		effort string
		limit  int
	}{
		{effort: "unknown"},
		{limit: -1},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		data, err := json.Marshal(fileConfig{BaseURL: "https://example.com/v1", APIKey: "test", Model: "test", OCSToken: []string{"test"}, ReasoningEffort: tc.effort, MaxOutputTokens: tc.limit})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadConfig(path); err == nil {
			t.Fatalf("accepted invalid settings: %+v", tc)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if _, _, err := loadConfig(path); err == nil {
		t.Fatal("expected new configuration to require editing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var generated fileConfig
	if err := json.Unmarshal(data, &generated); err != nil || generated.Listen != "127.0.0.1:7777" || generated.APIKey != "" || len(generated.OCSToken) != 0 {
		t.Fatalf("invalid generated config: %v", err)
	}
	if _, _, err := loadConfig(path); err == nil {
		t.Fatal("expected blank configuration to fail")
	}
	valid := fileConfig{BaseURL: "https://example.com/v1/", APIKey: "test", Model: "test", OCSToken: []string{"test"}}
	data, _ = json.Marshal(valid)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	c, listen, err := loadConfig(path)
	if err != nil || c.endpoint != "https://example.com/v1" || listen != "127.0.0.1:7777" {
		t.Fatalf("unexpected configuration: %v, %s", err, listen)
	}
	valid.Listen = "127.0.0.1:9000"
	data, _ = json.Marshal(valid)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, listen, err = loadConfig(path)
	if err != nil || listen != valid.Listen {
		t.Fatalf("custom listen: %v, %s", err, listen)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(path); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func response(answers string) string {
	text, _ := json.Marshal(`{"answers":` + answers + `}`)
	return fmt.Sprintf(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":%s}]}]}`, text)
}

func TestAnswerTypes(t *testing.T) {
	for _, tc := range []struct{ kind, modelAnswer, want string }{
		{"single", `["2"]`, "two"},
		{"multiple", `["1","2"]`, "one#two"},
		{"judgement", `["正确"]`, "正确"},
		{"completion", `["first","second"]`, "first#second"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response(tc.modelAnswer)) }))
			defer upstream.Close()
			c := config{endpoint: upstream.URL, model: "test", apiKey: "test", ocsTokens: []string{"test"}}
			req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(fmt.Sprintf(`{"title":"question","type":%q,"options":"A. one\nB. two"}`, tc.kind)))
			req.Header.Set("Authorization", "Bearer test")
			w := httptest.NewRecorder()
			handler(c, upstream.Client()).ServeHTTP(w, req)
			var got map[string]string
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil || got["answer"] != tc.want {
				t.Fatalf("status %d, body %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAuthentication(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(`{}`))
	handler(config{ocsTokens: []string{"test"}}, http.DefaultClient).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", w.Code)
	}
}

func TestConnectionProbe(t *testing.T) {
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req := httptest.NewRequest(method, "/api/answer?t=123", nil)
		w := httptest.NewRecorder()
		handler(config{ocsTokens: []string{"test"}}, http.DefaultClient).ServeHTTP(w, req)
		if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
			t.Errorf("%s: status %d body %q", method, w.Code, w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPut, "/api/answer", nil)
	w := httptest.NewRecorder()
	handler(config{ocsTokens: []string{"test"}}, http.DefaultClient).ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT: status %d", w.Code)
	}
}

func TestRetryAndImages(t *testing.T) {
	attempts := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		var body struct {
			Input []struct {
				Content []map[string]string `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		found := false
		for _, part := range body.Input[0].Content {
			if part["type"] == "input_image" && part["image_url"] == "https://example.com/test.png" {
				found = true
			}
		}
		if !found {
			t.Error("image missing")
		}
		fmt.Fprint(w, response(`["1"]`))
	}))
	defer upstream.Close()
	c := config{endpoint: upstream.URL, model: "test", apiKey: "test", ocsTokens: []string{"test"}}
	req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(`{"title":"image https://example.com/test.png","type":"single","options":"A. one"}`))
	req.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	handler(c, upstream.Client()).ServeHTTP(w, req)
	if w.Code != http.StatusOK || attempts != 3 {
		t.Fatalf("status %d attempts %d", w.Code, attempts)
	}
}

func TestInvalidModelAnswerAndNonRetryableError(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		attempts := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if invalid {
				fmt.Fprint(w, response(`["3"]`))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
		}))
		c := config{endpoint: upstream.URL, model: "test", apiKey: "test", ocsTokens: []string{"test"}}
		req := httptest.NewRequest(http.MethodPost, "/api/answer", strings.NewReader(`{"title":"question","type":"single","options":"A. one"}`))
		req.Header.Set("Authorization", "Bearer test")
		w := httptest.NewRecorder()
		handler(c, upstream.Client()).ServeHTTP(w, req)
		upstream.Close()
		if w.Code != http.StatusBadGateway || attempts != 1 {
			t.Fatalf("status %d attempts %d", w.Code, attempts)
		}
	}
}
