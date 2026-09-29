package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnalyzeEndpointReplaysRequestID(t *testing.T) {
	body := bytes.NewBufferString(`{"start":"S","productions":["S->a"],"tokens":["a"],"requestId":"page-version-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	analyzeHandler(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var resp AnalyzeResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.RequestID != "page-version-1" || !resp.Accepted {
		t.Fatalf("response = %+v", resp)
	}
}
