package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func BenchmarkRandomMagic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = randomMagic()
	}
}

func BenchmarkExtractCountdown(b *testing.B) {
	body := `<HTML><BODY><b id="countdown">7200</b> seconds remaining</BODY></HTML>`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = extractCountdown(body)
	}
}

func BenchmarkExtractPortalRegex(b *testing.B) {
	html := `<script>window.location="https://192.168.55.253:1003/fgtauth?0123456789abcdef"</script>`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matches := portalRe.FindStringSubmatch(html)
		if len(matches) < 2 {
			b.Fatal("expected match")
		}
	}
}

func BenchmarkGenerateCandidateEndpoints(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = generateCandidateEndpoints()
	}
}

func BenchmarkConfigLoad(b *testing.B) {
	tmpDir := b.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_ = resetConfig()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = loadConfig()
	}
}

func BenchmarkGetPortalHTTP(b *testing.B) {
	// Mock HTTP server responding with 204 No Content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := ts.Client()
	client.Timeout = 2 * time.Second

	// Verify loopback connectivity
	testReq, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	testResp, err := client.Do(testReq)
	if err != nil {
		b.Skipf("skipping benchmark: loopback connection not permitted in sandbox: %v", err)
		return
	}
	testResp.Body.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}
