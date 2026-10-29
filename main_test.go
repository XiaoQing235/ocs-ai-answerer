package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
			c := config{endpoint: upstream.URL, model: "test", apiKey: "test", token: "test"}
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
	handler(config{token: "test"}, http.DefaultClient).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", w.Code)
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
	c := config{endpoint: upstream.URL, model: "test", apiKey: "test", token: "test"}
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
		c := config{endpoint: upstream.URL, model: "test", apiKey: "test", token: "test"}
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
