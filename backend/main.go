package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/analyze", analyzeHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	address := ":" + port
	log.Printf("grammar service listening on %s", address)
	if err := http.ListenAndServe(address, mux); err != nil {
		log.Fatal(err)
	}
}

func analyzeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只支持 POST 请求"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req AnalyzeRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体必须是合法 JSON：" + err.Error()})
		return
	}

	if req.RequestID == "" {
		req.RequestID = r.Header.Get("X-Request-ID")
	}
	if req.RequestID == "" {
		req.RequestID = newRequestID()
	}
	if len(req.RequestID) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "requestId 长度不能超过 100"})
		return
	}

	resp, err := Analyze(req)
	if err != nil {
		var validationErr *validationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationErr.Error(), "requestId": req.RequestID})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "分析失败"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
