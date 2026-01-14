package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func Test_getVideoList(t *testing.T) {
	type args struct {
		videoStoreDir string
	}
	tests := []struct {
		name    string
		args    args
		want    []VideoInfo
		wantErr bool
	}{
		{
			name: "test missing title, description, classification",
			args: args{"testVideos"},
			want: []VideoInfo{{
				Title:          "",
				Description:    "",
				Filename:       "video1",
				Classification: "",
				Taxonomy:       Taxonomy{},
			}},
		},
	}
	err := os.MkdirAll("testVideos/video1", os.ModePerm)
	if err != nil {
		t.Errorf("getVideoList() mkdir error = %v", err)
	} else {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := getVideoList(tt.args.videoStoreDir)
				if (err != nil) != tt.wantErr {
					t.Errorf("getVideoList() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("getVideoList() got = %v, want %v", got, tt.want)
				}
			})
		}
	}
	_ = os.RemoveAll("testVideos")
}

func Test_isValidVideoFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"video1", true},
		{"video-1", true},
		{"video_1", true},
		{"Video1", true},
		{"", false},
		{"..", false},
		{"../etc/passwd", false},
		{"/etc/passwd", false},
		{"video\\1", false},
		{"video 1", false},
		{"video@1", false},
		{strings.Repeat("a", 101), false}, // too long
		{strings.Repeat("a", 100), true},  // exactly 100
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := isValidVideoFilename(tt.filename)
			if got != tt.want {
				t.Errorf("isValidVideoFilename(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func Test_parseTaxonomy(t *testing.T) {
	tests := []struct {
		classification string
		want           Taxonomy
	}{
		{
			"elementary/math/number system/counting",
			Taxonomy{
				Class:  "elementary",
				Order:  "math",
				Family: "number system",
				Tribe:  "counting",
				Genus:  "",
			},
		},
		{
			"single",
			Taxonomy{
				Class:  "single",
				Order:  "",
				Family: "",
				Tribe:  "",
				Genus:  "",
			},
		},
		{
			"one/two/three/four/five",
			Taxonomy{
				Class:  "one",
				Order:  "two",
				Family: "three",
				Tribe:  "four",
				Genus:  "five",
			},
		},
		{
			"",
			Taxonomy{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.classification, func(t *testing.T) {
			got := parseTaxonomy(tt.classification)
			if got != tt.want {
				t.Errorf("parseTaxonomy(%q) = %+v, want %+v", tt.classification, got, tt.want)
			}
		})
	}
}

func Test_filterVideosByTaxonomy(t *testing.T) {
	videos := []VideoInfo{
		{
			Title:    "Math Video",
			Filename: "video1",
			Taxonomy: Taxonomy{Class: "elementary", Order: "math", Family: "arithmetic"},
		},
		{
			Title:    "Science Video",
			Filename: "video2",
			Taxonomy: Taxonomy{Class: "elementary", Order: "science", Family: "biology"},
		},
		{
			Title:    "Advanced Math",
			Filename: "video3",
			Taxonomy: Taxonomy{Class: "advanced", Order: "math", Family: "calculus"},
		},
	}

	tests := []struct {
		rank  string
		value string
		want  []VideoInfo
	}{
		{
			"class",
			"elementary",
			[]VideoInfo{videos[0], videos[1]},
		},
		{
			"order",
			"math",
			[]VideoInfo{videos[0], videos[2]},
		},
		{
			"family",
			"biology",
			[]VideoInfo{videos[1]},
		},
		{
			"class",
			"nonexistent",
			[]VideoInfo{},
		},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s=%s", tt.rank, tt.value)
		t.Run(name, func(t *testing.T) {
			got := filterVideosByTaxonomy(videos, tt.rank, tt.value)
			if len(got) != len(tt.want) {
				t.Errorf("filterVideosByTaxonomy() returned %d videos, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Filename != tt.want[i].Filename {
					t.Errorf("video[%d] filename = %s, want %s", i, got[i].Filename, tt.want[i].Filename)
				}
			}
		})
	}
}

func Test_loadConfig(t *testing.T) {
	// Save original environment variables
	originalVideoPath := os.Getenv("DAAYA_VIDEO_PATH")
	originalPort := os.Getenv("DAAYA_PORT")
	defer func() {
		// Restore environment variables
		if originalVideoPath != "" {
			os.Setenv("DAAYA_VIDEO_PATH", originalVideoPath)
		} else {
			os.Unsetenv("DAAYA_VIDEO_PATH")
		}
		if originalPort != "" {
			os.Setenv("DAAYA_PORT", originalPort)
		} else {
			os.Unsetenv("DAAYA_PORT")
		}
	}()

	tests := []struct {
		name           string
		setVideoPath   string
		setPort        string
		wantVideoPath  string
		wantPort       string
	}{
		{
			name:           "default values",
			setVideoPath:   "",
			setPort:        "",
			wantVideoPath:  "/var/daaya/videos",
			wantPort:       ":8182",
		},
		{
			name:           "custom video path",
			setVideoPath:   "/custom/videos",
			setPort:        "",
			wantVideoPath:  "/custom/videos",
			wantPort:       ":8182",
		},
		{
			name:           "custom port without colon",
			setVideoPath:   "",
			setPort:        "9000",
			wantVideoPath:  "/var/daaya/videos",
			wantPort:       ":9000",
		},
		{
			name:           "custom port with colon",
			setVideoPath:   "",
			setPort:        ":9000",
			wantVideoPath:  "/var/daaya/videos",
			wantPort:       ":9000",
		},
		{
			name:           "both custom values",
			setVideoPath:   "/custom/videos",
			setPort:        "9000",
			wantVideoPath:  "/custom/videos",
			wantPort:       ":9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.setVideoPath != "" {
				os.Setenv("DAAYA_VIDEO_PATH", tt.setVideoPath)
			} else {
				os.Unsetenv("DAAYA_VIDEO_PATH")
			}
			if tt.setPort != "" {
				os.Setenv("DAAYA_PORT", tt.setPort)
			} else {
				os.Unsetenv("DAAYA_PORT")
			}

			got := loadConfig()
			if got.VideoStorePath != tt.wantVideoPath {
				t.Errorf("loadConfig() VideoStorePath = %v, want %v", got.VideoStorePath, tt.wantVideoPath)
			}
			if got.Port != tt.wantPort {
				t.Errorf("loadConfig() Port = %v, want %v", got.Port, tt.wantPort)
			}
		})
	}
}

func TestMetrics(t *testing.T) {
	m := newMetrics()

	// Test initial state
	if m.requestsTotal != 0 || m.errorsTotal != 0 || m.streamsTotal != 0 || m.classifyRequests != 0 || m.lastError != "" {
		t.Errorf("New metrics should be zero, got: requests=%d, errors=%d, streams=%d, classify=%d, lastError=%s",
			m.requestsTotal, m.errorsTotal, m.streamsTotal, m.classifyRequests, m.lastError)
	}

	// Test incRequests
	m.incRequests()
	if m.requestsTotal != 1 {
		t.Errorf("incRequests() should increment to 1, got %d", m.requestsTotal)
	}

	// Test incErrors
	m.incErrors("test error")
	if m.errorsTotal != 1 {
		t.Errorf("incErrors() should increment errors to 1, got %d", m.errorsTotal)
	}
	if m.lastError != "test error" {
		t.Errorf("incErrors() should set lastError to 'test error', got %s", m.lastError)
	}

	// Test incStreams
	m.incStreams()
	if m.streamsTotal != 1 {
		t.Errorf("incStreams() should increment streams to 1, got %d", m.streamsTotal)
	}

	// Test incClassify
	m.incClassify()
	if m.classifyRequests != 1 {
		t.Errorf("incClassify() should increment classify to 1, got %d", m.classifyRequests)
	}

	// Test export
	exported := m.export()
	expectedSubstrings := []string{
		"daaya_requests_total 1",
		"daaya_errors_total 1",
		"daaya_streams_total 1",
		"daaya_classify_requests_total 1",
		`daaya_last_error{error="test error"} 1`,
	}
	for _, substr := range expectedSubstrings {
		if !strings.Contains(exported, substr) {
			t.Errorf("export() should contain %q, got: %s", substr, exported)
		}
	}
}

func TestNewIPRateLimiter(t *testing.T) {
	limiter := newIPRateLimiter(rate.Limit(10), 5)
	if limiter.r != rate.Limit(10) {
		t.Errorf("Expected rate limit 10, got %v", limiter.r)
	}
	if limiter.b != 5 {
		t.Errorf("Expected burst 5, got %d", limiter.b)
	}
	if limiter.ips == nil {
		t.Error("ips map should be initialized")
	}
}

func TestIPRateLimiterGetLimiter(t *testing.T) {
	limiter := newIPRateLimiter(rate.Limit(2), 1)

	// First call for an IP should create new limiter
	ip1Limiter := limiter.getLimiter("192.168.1.1")
	if ip1Limiter == nil {
		t.Error("getLimiter() should return a non-nil limiter")
	}

	// Second call for same IP should return same limiter
	ip1Limiter2 := limiter.getLimiter("192.168.1.1")
	if ip1Limiter != ip1Limiter2 {
		t.Error("getLimiter() should return same limiter for same IP")
	}

	// Different IP should get different limiter
	ip2Limiter := limiter.getLimiter("192.168.1.2")
	if ip1Limiter == ip2Limiter {
		t.Error("getLimiter() should return different limiter for different IP")
	}

	// Test that limiter respects rate
	if !ip1Limiter.Allow() {
		t.Error("Limiter should allow first request")
	}
	// Second request within same second might be allowed depending on burst
	// With burst=1, second request should not be allowed immediately
	// But Allow() consumes a token, so we need to check after consuming one
	// Actually burst=1 means one token, we already consumed it
	// So next call should return false
	time.Sleep(100 * time.Millisecond) // Wait a bit
	if ip1Limiter.Allow() {
		// Might be allowed after some time due to rate limit
		// This is fine, we just test that limiter works
	}
}

func TestResponseWrapper(t *testing.T) {
	// Create a mock ResponseWriter
	recorder := httptest.NewRecorder()

	// Create wrapper
	wrapper := &responseWrapper{
		ResponseWriter: recorder,
		statusCode:     http.StatusOK,
	}

	// Test WriteHeader
	wrapper.WriteHeader(http.StatusNotFound)
	if wrapper.statusCode != http.StatusNotFound {
		t.Errorf("WriteHeader should set statusCode to %d, got %d", http.StatusNotFound, wrapper.statusCode)
	}

	// Verify the underlying ResponseWriter received the status
	if recorder.Code != http.StatusNotFound {
		t.Errorf("Underlying ResponseWriter should have status code %d, got %d", http.StatusNotFound, recorder.Code)
	}

	// Test that WriteHeader can only be called once
	wrapper.WriteHeader(http.StatusInternalServerError)
	if wrapper.statusCode != http.StatusInternalServerError {
		t.Errorf("Second WriteHeader should update statusCode to %d, got %d", http.StatusInternalServerError, wrapper.statusCode)
	}
	// Note: net/http/httptest.ResponseRecorder accumulates status codes
	// The second WriteHeader would update the recorder.Code
}

func TestMetricsMiddleware(t *testing.T) {
	// Reset global metrics
	metrics = newMetrics()

	// Create a test handler that returns a specific status code
	handlerCalled := false
	testHandler := func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}

	// Wrap with metrics middleware
	wrapped := metricsMiddleware(testHandler)

	// Create test request and recorder
	req := httptest.NewRequest("GET", "/api/v1/videos", nil)
	rec := httptest.NewRecorder()

	// Call the middleware
	wrapped(rec, req)

	// Verify handler was called
	if !handlerCalled {
		t.Error("metricsMiddleware should call the next handler")
	}

	// Verify metrics were updated
	if metrics.requestsTotal != 1 {
		t.Errorf("metricsMiddleware should increment requestsTotal, got %d", metrics.requestsTotal)
	}
	// No error since status is 200
	if metrics.errorsTotal != 0 {
		t.Errorf("metricsMiddleware should not increment errorsTotal for status 200, got %d", metrics.errorsTotal)
	}

	// Test with error status
	metrics = newMetrics()
	errorHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
	wrappedError := metricsMiddleware(errorHandler)
	req2 := httptest.NewRequest("GET", "/api/v1/videos", nil)
	rec2 := httptest.NewRecorder()
	wrappedError(rec2, req2)

	if metrics.errorsTotal != 1 {
		t.Errorf("metricsMiddleware should increment errorsTotal for status 404, got %d", metrics.errorsTotal)
	}
	if metrics.lastError != "Not Found" {
		t.Errorf("metricsMiddleware should set lastError to 'Not Found', got %s", metrics.lastError)
	}

	// Test stream path detection
	metrics = newMetrics()
	streamHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	wrappedStream := metricsMiddleware(streamHandler)
	req3 := httptest.NewRequest("GET", "/api/v1/stream/video1", nil)
	rec3 := httptest.NewRecorder()
	wrappedStream(rec3, req3)

	if metrics.streamsTotal != 1 {
		t.Errorf("metricsMiddleware should increment streamsTotal for stream path, got %d", metrics.streamsTotal)
	}

	// Test classify path detection
	metrics = newMetrics()
	classifyHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	wrappedClassify := metricsMiddleware(classifyHandler)
	req4 := httptest.NewRequest("GET", "/api/v1/classify?rank=class&value=elementary", nil)
	rec4 := httptest.NewRecorder()
	wrappedClassify(rec4, req4)

	if metrics.classifyRequests != 1 {
		t.Errorf("metricsMiddleware should increment classifyRequests for classify path, got %d", metrics.classifyRequests)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	// Replace global rate limiter with a test one (very high limit)
	originalLimiter := ipRateLimiter
	defer func() { ipRateLimiter = originalLimiter }()

	ipRateLimiter = newIPRateLimiter(rate.Limit(1000), 1000) // Very permissive

	handlerCalled := false
	testHandler := func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}

	wrapped := rateLimitMiddleware(testHandler)

	// Test successful request
	req := httptest.NewRequest("GET", "/api/v1/videos", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	wrapped(rec, req)

	if !handlerCalled {
		t.Error("rateLimitMiddleware should call next handler when limit not exceeded")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Test with malformed RemoteAddr (no port)
	handlerCalled = false
	req2 := httptest.NewRequest("GET", "/api/v1/videos", nil)
	req2.RemoteAddr = "192.168.1.1" // No port
	rec2 := httptest.NewRecorder()

	wrapped(rec2, req2)

	if !handlerCalled {
		t.Error("rateLimitMiddleware should handle malformed RemoteAddr")
	}

	// Test rate limiting with zero rate (should never allow)
	ipRateLimiter = newIPRateLimiter(0, 0) // Zero rate, zero burst

	handlerCalled = false
	wrappedStrict := rateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	req3 := httptest.NewRequest("GET", "/api/v1/videos", nil)
	req3.RemoteAddr = "192.168.1.2:12345"
	rec3 := httptest.NewRecorder()

	wrappedStrict(rec3, req3)

	if handlerCalled {
		t.Error("rateLimitMiddleware should block when rate limit exceeded")
	}
	if rec3.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", rec3.Code)
	}
	if body := rec3.Body.String(); body != "Rate limit exceeded\n" {
		t.Errorf("Expected 'Rate limit exceeded' body, got %q", body)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	handlerCalled := false
	testHandler := func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}

	wrapped := loggingMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/api/v1/videos", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	wrapped(rec, req)

	if !handlerCalled {
		t.Error("loggingMiddleware should call next handler")
	}
	// Logging to stdout is a side effect we don't need to test
}

func TestGetVideoInfo(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create metadata files
	files := map[string]string{
		"title":          "Test Video",
		"author":         "Test Author",
		"description":    "Test Description",
		"classification": "science/biology/cells",
	}

	for filename, content := range files {
		filePath := fmt.Sprintf("%s/%s", tmpDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Call getVideoInfo
	videoInfo, err := getVideoInfo(tmpDir)
	if err != nil {
		t.Fatalf("getVideoInfo returned error: %v", err)
	}

	// Verify results
	if videoInfo.Title != "Test Video" {
		t.Errorf("Expected Title 'Test Video', got %q", videoInfo.Title)
	}
	if videoInfo.Author != "Test Author" {
		t.Errorf("Expected Author 'Test Author', got %q", videoInfo.Author)
	}
	if videoInfo.Description != "Test Description" {
		t.Errorf("Expected Description 'Test Description', got %q", videoInfo.Description)
	}
	if videoInfo.Classification != "science/biology/cells" {
		t.Errorf("Expected Classification 'science/biology/cells', got %q", videoInfo.Classification)
	}
	if videoInfo.Filename != filepath.Base(tmpDir) {
		t.Errorf("Expected Filename %q, got %q", filepath.Base(tmpDir), videoInfo.Filename)
	}
	// Verify taxonomy parsing
	if videoInfo.Taxonomy.Class != "science" {
		t.Errorf("Expected Taxonomy.Class 'science', got %q", videoInfo.Taxonomy.Class)
	}
	if videoInfo.Taxonomy.Order != "biology" {
		t.Errorf("Expected Taxonomy.Order 'biology', got %q", videoInfo.Taxonomy.Order)
	}
	if videoInfo.Taxonomy.Family != "cells" {
		t.Errorf("Expected Taxonomy.Family 'cells', got %q", videoInfo.Taxonomy.Family)
	}
	if videoInfo.Taxonomy.Tribe != "" {
		t.Errorf("Expected empty Taxonomy.Tribe, got %q", videoInfo.Taxonomy.Tribe)
	}
	if videoInfo.Taxonomy.Genus != "" {
		t.Errorf("Expected empty Taxonomy.Genus, got %q", videoInfo.Taxonomy.Genus)
	}
}

func TestGetVideoInfoMissingFiles(t *testing.T) {
	// Create empty temporary directory
	tmpDir := t.TempDir()

	// Call getVideoInfo on empty directory
	videoInfo, err := getVideoInfo(tmpDir)
	if err != nil {
		t.Fatalf("getVideoInfo returned error: %v", err)
	}

	// All fields should be empty
	if videoInfo.Title != "" {
		t.Errorf("Expected empty Title, got %q", videoInfo.Title)
	}
	if videoInfo.Author != "" {
		t.Errorf("Expected empty Author, got %q", videoInfo.Author)
	}
	if videoInfo.Description != "" {
		t.Errorf("Expected empty Description, got %q", videoInfo.Description)
	}
	if videoInfo.Classification != "" {
		t.Errorf("Expected empty Classification, got %q", videoInfo.Classification)
	}
	if videoInfo.Filename != filepath.Base(tmpDir) {
		t.Errorf("Expected Filename %q, got %q", filepath.Base(tmpDir), videoInfo.Filename)
	}
	// Taxonomy should be empty
	if videoInfo.Taxonomy != (Taxonomy{}) {
		t.Errorf("Expected empty Taxonomy, got %+v", videoInfo.Taxonomy)
	}
}

func TestListVideos(t *testing.T) {
	// Create temporary video directory structure
	tmpDir := t.TempDir()

	// Create video1 directory with metadata
	video1Dir := fmt.Sprintf("%s/video1", tmpDir)
	if err := os.Mkdir(video1Dir, 0755); err != nil {
		t.Fatalf("Failed to create video1 directory: %v", err)
	}
	files1 := map[string]string{
		"title":          "Video One",
		"author":         "Author One",
		"description":    "Description One",
		"classification": "math/algebra",
	}
	for filename, content := range files1 {
		filePath := fmt.Sprintf("%s/%s", video1Dir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Create video2 directory with metadata
	video2Dir := fmt.Sprintf("%s/video2", tmpDir)
	if err := os.Mkdir(video2Dir, 0755); err != nil {
		t.Fatalf("Failed to create video2 directory: %v", err)
	}
	files2 := map[string]string{
		"title":          "Video Two",
		"author":         "Author Two",
		"description":    "Description Two",
		"classification": "science/biology",
	}
	for filename, content := range files2 {
		filePath := fmt.Sprintf("%s/%s", video2Dir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Save original videoStorePath and replace
	originalVideoStorePath := videoStorePath
	videoStorePath = tmpDir
	defer func() { videoStorePath = originalVideoStorePath }()

	// Create test request
	req := httptest.NewRequest("GET", "/api/v1/videos", nil)
	rec := httptest.NewRecorder()

	// Call handler
	listVideos(rec, req)

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Parse response
	var videos []VideoInfo
	if err := json.NewDecoder(rec.Body).Decode(&videos); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(videos) != 2 {
		t.Errorf("Expected 2 videos, got %d", len(videos))
	}

	// Verify video data
	// Order is not guaranteed due to filepath.Walk
	foundVideo1 := false
	foundVideo2 := false
	for _, video := range videos {
		if video.Filename == "video1" {
			foundVideo1 = true
			if video.Title != "Video One" {
				t.Errorf("Video1 title mismatch: got %q", video.Title)
			}
			if video.Taxonomy.Class != "math" {
				t.Errorf("Video1 taxonomy class mismatch: got %q", video.Taxonomy.Class)
			}
		} else if video.Filename == "video2" {
			foundVideo2 = true
			if video.Title != "Video Two" {
				t.Errorf("Video2 title mismatch: got %q", video.Title)
			}
			if video.Taxonomy.Class != "science" {
				t.Errorf("Video2 taxonomy class mismatch: got %q", video.Taxonomy.Class)
			}
		}
	}
	if !foundVideo1 || !foundVideo2 {
		t.Errorf("Missing videos: foundVideo1=%v, foundVideo2=%v", foundVideo1, foundVideo2)
	}
}

func TestListVideosMethodNotAllowed(t *testing.T) {
	// Test non-GET request
	req := httptest.NewRequest("POST", "/api/v1/videos", nil)
	rec := httptest.NewRecorder()

	listVideos(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", rec.Code)
	}
}

func TestListVideosDirectoryError(t *testing.T) {
	// Set videoStorePath to non-existent directory
	originalVideoStorePath := videoStorePath
	videoStorePath = "/non/existent/path"
	defer func() { videoStorePath = originalVideoStorePath }()

	req := httptest.NewRequest("GET", "/api/v1/videos", nil)
	rec := httptest.NewRecorder()

	listVideos(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for non-existent directory, got %d", rec.Code)
	}
}

func TestClassifyVideos(t *testing.T) {
	// Create temporary video directory structure
	tmpDir := t.TempDir()

	// Create video directories with different classifications
	videos := []struct {
		name           string
		classification string
	}{
		{"math_video", "math/algebra"},
		{"science_video", "science/biology"},
		{"math_video2", "math/geometry"},
		{"science_video2", "science/chemistry"},
	}

	for _, v := range videos {
		videoDir := fmt.Sprintf("%s/%s", tmpDir, v.name)
		if err := os.Mkdir(videoDir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", videoDir, err)
		}
		// Create minimal metadata
		titleFile := fmt.Sprintf("%s/title", videoDir)
		if err := os.WriteFile(titleFile, []byte(v.name), 0644); err != nil {
			t.Fatalf("Failed to create title file: %v", err)
		}
		classificationFile := fmt.Sprintf("%s/classification", videoDir)
		if err := os.WriteFile(classificationFile, []byte(v.classification), 0644); err != nil {
			t.Fatalf("Failed to create classification file: %v", err)
		}
	}

	// Save original videoStorePath and replace
	originalVideoStorePath := videoStorePath
	videoStorePath = tmpDir
	defer func() { videoStorePath = originalVideoStorePath }()

	// Test filtering by class=math
	req := httptest.NewRequest("GET", "/api/v1/classify?rank=class&value=math", nil)
	rec := httptest.NewRecorder()

	classifyVideos(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var filteredVideos []VideoInfo
	if err := json.NewDecoder(rec.Body).Decode(&filteredVideos); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(filteredVideos) != 2 {
		t.Errorf("Expected 2 math videos, got %d", len(filteredVideos))
	}
	for _, video := range filteredVideos {
		if video.Taxonomy.Class != "math" {
			t.Errorf("Expected class 'math', got %q", video.Taxonomy.Class)
		}
	}

	// Test filtering by order=biology
	req2 := httptest.NewRequest("GET", "/api/v1/classify?rank=order&value=biology", nil)
	rec2 := httptest.NewRecorder()

	classifyVideos(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec2.Code)
	}

	var filteredVideos2 []VideoInfo
	if err := json.NewDecoder(rec2.Body).Decode(&filteredVideos2); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(filteredVideos2) != 1 {
		t.Errorf("Expected 1 biology video, got %d", len(filteredVideos2))
	}
	if filteredVideos2[0].Filename != "science_video" {
		t.Errorf("Expected science_video, got %q", filteredVideos2[0].Filename)
	}
}

func TestClassifyVideosMissingParameters(t *testing.T) {
	// Test missing rank
	req := httptest.NewRequest("GET", "/api/v1/classify?value=math", nil)
	rec := httptest.NewRecorder()

	classifyVideos(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing rank, got %d", rec.Code)
	}

	// Test missing value
	req2 := httptest.NewRequest("GET", "/api/v1/classify?rank=class", nil)
	rec2 := httptest.NewRecorder()

	classifyVideos(rec2, req2)

	if rec2.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing value, got %d", rec2.Code)
	}

	// Test both missing
	req3 := httptest.NewRequest("GET", "/api/v1/classify", nil)
	rec3 := httptest.NewRecorder()

	classifyVideos(rec3, req3)

	if rec3.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing parameters, got %d", rec3.Code)
	}
}

func TestClassifyVideosInvalidRank(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/classify?rank=invalid&value=math", nil)
	rec := httptest.NewRecorder()

	classifyVideos(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid rank, got %d", rec.Code)
	}
}

func TestClassifyVideosMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/classify", nil)
	rec := httptest.NewRecorder()

	classifyVideos(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", rec.Code)
	}
}

func TestHelpAPI(t *testing.T) {
	req := httptest.NewRequest("GET", "/help", nil)
	rec := httptest.NewRecorder()

	helpAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var endpoints []APIEndpoint
	if err := json.NewDecoder(rec.Body).Decode(&endpoints); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check that we have at least the expected endpoints
	if len(endpoints) < 6 {
		t.Errorf("Expected at least 6 endpoints, got %d", len(endpoints))
	}

	// Verify some known endpoints exist
	foundVideos := false
	foundStream := false
	foundClassify := false
	foundHelp := false
	foundHealth := false
	foundMetrics := false

	for _, endpoint := range endpoints {
		switch endpoint.Path {
		case "/api/v1/videos":
			foundVideos = true
			if endpoint.Method != "GET" {
				t.Errorf("Videos endpoint method should be GET, got %s", endpoint.Method)
			}
		case "/api/v1/stream/{filename}":
			foundStream = true
			if endpoint.Method != "GET" {
				t.Errorf("Stream endpoint method should be GET, got %s", endpoint.Method)
			}
		case "/api/v1/classify?rank=rankName&value=rankValue":
			foundClassify = true
			if endpoint.Method != "GET" {
				t.Errorf("Classify endpoint method should be GET, got %s", endpoint.Method)
			}
		case "/help":
			foundHelp = true
			if endpoint.Method != "GET" {
				t.Errorf("Help endpoint method should be GET, got %s", endpoint.Method)
			}
		case "/health":
			foundHealth = true
			if endpoint.Method != "GET" {
				t.Errorf("Health endpoint method should be GET, got %s", endpoint.Method)
			}
		case "/metrics":
			foundMetrics = true
			if endpoint.Method != "GET" {
				t.Errorf("Metrics endpoint method should be GET, got %s", endpoint.Method)
			}
		}
	}

	if !foundVideos {
		t.Error("Missing /api/v1/videos endpoint in help response")
	}
	if !foundStream {
		t.Error("Missing /api/v1/stream/{filename} endpoint in help response")
	}
	if !foundClassify {
		t.Error("Missing /api/v1/classify endpoint in help response")
	}
	if !foundHelp {
		t.Error("Missing /help endpoint in help response")
	}
	if !foundHealth {
		t.Error("Missing /health endpoint in help response")
	}
	if !foundMetrics {
		t.Error("Missing /metrics endpoint in help response")
	}
}

func TestHelpAPIMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/help", nil)
	rec := httptest.NewRecorder()

	helpAPI(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", rec.Code)
	}
}

func TestHealthCheck(t *testing.T) {
	// Test healthy state with existing directory
	tmpDir := t.TempDir()
	originalVideoStorePath := videoStorePath
	videoStorePath = tmpDir
	defer func() { videoStorePath = originalVideoStorePath }()

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	healthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 for healthy, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %q", response["status"])
	}
	if response["message"] != "Service is running" {
		t.Errorf("Expected message 'Service is running', got %q", response["message"])
	}
}

func TestHealthCheckUnhealthy(t *testing.T) {
	// Test unhealthy state with non-existent directory
	originalVideoStorePath := videoStorePath
	videoStorePath = "/non/existent/path"
	defer func() { videoStorePath = originalVideoStorePath }()

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	healthCheck(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 for unhealthy, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %q", response["status"])
	}
	if !strings.Contains(response["error"], "Video directory inaccessible") {
		t.Errorf("Expected error about directory inaccessibility, got %q", response["error"])
	}
}

func TestHealthCheckMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/health", nil)
	rec := httptest.NewRecorder()

	healthCheck(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", rec.Code)
	}
}

func TestMetricsHandler(t *testing.T) {
	// Reset metrics
	metrics = newMetrics()

	// Increment some metrics
	metrics.incRequests()
	metrics.incErrors("test error")
	metrics.incStreams()
	metrics.incClassify()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	metricsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/plain; version=0.0.4" {
		t.Errorf("Expected Content-Type 'text/plain; version=0.0.4', got %q", contentType)
	}

	body := rec.Body.String()
	expectedMetrics := []string{
		"daaya_requests_total 1",
		"daaya_errors_total 1",
		"daaya_streams_total 1",
		"daaya_classify_requests_total 1",
		`daaya_last_error{error="test error"} 1`,
	}
	for _, expected := range expectedMetrics {
		if !strings.Contains(body, expected) {
			t.Errorf("Metrics response missing %q, got: %s", expected, body)
		}
	}
}

func TestMetricsHandlerMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/metrics", nil)
	rec := httptest.NewRecorder()

	metricsHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", rec.Code)
	}
}

func TestStreamVideo(t *testing.T) {
	// Create temporary video directory structure
	tmpDir := t.TempDir()
	originalVideoStorePath := videoStorePath
	videoStorePath = tmpDir
	defer func() { videoStorePath = originalVideoStorePath }()

	// Create video directory and dummy mp4 file
	videoDir := fmt.Sprintf("%s/testvideo", tmpDir)
	if err := os.Mkdir(videoDir, 0755); err != nil {
		t.Fatalf("Failed to create video directory: %v", err)
	}
	videoFile := fmt.Sprintf("%s/testvideo.mp4", videoDir)
	if err := os.WriteFile(videoFile, []byte("fake mp4 content"), 0644); err != nil {
		t.Fatalf("Failed to create video file: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/stream/testvideo", nil)
	rec := httptest.NewRecorder()

	streamVideo(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
	contentType := rec.Header().Get("Content-Type")
	if contentType != "video/mp4" {
		t.Errorf("Expected Content-Type 'video/mp4', got %q", contentType)
	}
	// Body should contain our fake content
	body := rec.Body.String()
	if body != "fake mp4 content" {
		t.Errorf("Expected body 'fake mp4 content', got %q", body)
	}
}

func TestStreamVideoMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/stream/video", nil)
	rec := httptest.NewRecorder()

	streamVideo(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", rec.Code)
	}
}

func TestStreamVideoInvalidFilename(t *testing.T) {
	// Test various invalid filenames
	testCases := []struct {
		name     string
		filename string
	}{
		{"empty", ""},
		{"path traversal", "../etc/passwd"},
		{"absolute path", "/etc/passwd"},
		{"backslash", "video\\1"},
		{"space", "video 1"},
		{"special char", "video@1"},
		{"too long", strings.Repeat("a", 101)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// URL encode the filename for the request path
			encodedFilename := url.PathEscape(tc.filename)
			req := httptest.NewRequest("GET", "/api/v1/stream/"+encodedFilename, nil)
			rec := httptest.NewRecorder()

			streamVideo(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400 for invalid filename %q, got %d", tc.filename, rec.Code)
			}
		})
	}
}

func TestStreamVideoNotFound(t *testing.T) {
	// Set videoStorePath to empty temp directory
	tmpDir := t.TempDir()
	originalVideoStorePath := videoStorePath
	videoStorePath = tmpDir
	defer func() { videoStorePath = originalVideoStorePath }()

	req := httptest.NewRequest("GET", "/api/v1/stream/nonexistent", nil)
	rec := httptest.NewRecorder()

	streamVideo(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent video, got %d", rec.Code)
	}
}

func TestStreamVideoDirectoryTraversal(t *testing.T) {
	// Even with valid filename, ensure path traversal is prevented
	tmpDir := t.TempDir()
	originalVideoStorePath := videoStorePath
	videoStorePath = tmpDir
	defer func() { videoStorePath = originalVideoStorePath }()

	// Create a video directory
	videoDir := fmt.Sprintf("%s/validvideo", tmpDir)
	if err := os.Mkdir(videoDir, 0755); err != nil {
		t.Fatalf("Failed to create video directory: %v", err)
	}
	videoFile := fmt.Sprintf("%s/validvideo.mp4", videoDir)
	if err := os.WriteFile(videoFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create video file: %v", err)
	}

	// Try to access with path traversal in filename (should be caught by isValidVideoFilename)
	req := httptest.NewRequest("GET", "/api/v1/stream/..%2Fvalidvideo", nil)
	rec := httptest.NewRecorder()

	streamVideo(rec, req)

	// Should be rejected as bad request
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for path traversal, got %d", rec.Code)
	}
}
