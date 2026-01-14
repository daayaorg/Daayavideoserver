package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/acme/autocert"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type VideoInfo struct {
	Title          string   `json:"title"`
	Author         string   `json:"author"`
	Description    string   `json:"description"`
	Filename       string   `json:"filename"`
	Classification string   `json:"classification"`
	Taxonomy       Taxonomy `json:"taxonomy"`
}

type Taxonomy struct {
	Class  string `json:"class"`
	Order  string `json:"order"`
	Family string `json:"family"`
	Tribe  string `json:"tribe"`
	Genus  string `json:"genus"`
}

const (
	descTag           = "description"
	titleTag          = "title"
	authorTag         = "author"
	classificationTag = "classification"
)

func main() {
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      "jp@daaya.org",
		HostPolicy: autocert.HostWhitelist("api.daaya.org"), //Your domain here
		Cache:      autocert.DirCache("certs"),              //Folder for storing certificates
	}

	cfg := loadConfig()
	videoStorePath = cfg.VideoStorePath

	// Chain middlewares: rate limiting -> metrics -> logging -> handler
	http.HandleFunc("/api/v1/videos", rateLimitMiddleware(metricsMiddleware(loggingMiddleware(listVideos))))
	http.HandleFunc("/api/v1/stream/", rateLimitMiddleware(metricsMiddleware(loggingMiddleware(streamVideo))))
	http.HandleFunc("/api/v1/classify", rateLimitMiddleware(metricsMiddleware(loggingMiddleware(classifyVideos))))
	http.HandleFunc("/help", rateLimitMiddleware(metricsMiddleware(loggingMiddleware(helpAPI))))
	http.HandleFunc("/health", rateLimitMiddleware(metricsMiddleware(loggingMiddleware(healthCheck))))
	http.HandleFunc("/metrics", rateLimitMiddleware(loggingMiddleware(metricsHandler)))

	fmt.Printf("Server starting on %s\n", cfg.Port)
	fmt.Printf("Video store path: %s\n", videoStorePath)

	// Configure HTTP server with timeouts
	server := &http.Server{
		Addr:              cfg.Port,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
		TLSConfig: &tls.Config{
			GetCertificate: certManager.GetCertificate,
			MinVersion:     tls.VersionTLS12, // improves cert reputation score at https://www.ssllabs.com/ssltest/
		},
	}

	go func() {
		err := http.ListenAndServe(":http", certManager.HTTPHandler(nil))
		if err != nil {
			log.Fatal(err)
		}
	}()
	log.Fatal(server.ListenAndServeTLS("", "")) //Key and cert are coming from Let's Encrypt

}

type Config struct {
	VideoStorePath string
	Port           string
}

var (
	videoStorePath string
	// Rate limiter: 10 requests per minute per IP
	ipRateLimiter = newIPRateLimiter(10.0, 60)
	// Metrics
	metrics = newMetrics()
)

// IPRateLimiter struct for per-IP rate limiting
type IPRateLimiter struct {
	ips map[string]*rate.Limiter
	mu  sync.RWMutex
	r   rate.Limit
	b   int
}

// Metrics struct for Prometheus-style metrics
type Metrics struct {
	mu               sync.RWMutex
	requestsTotal    uint64
	errorsTotal      uint64
	streamsTotal     uint64
	classifyRequests uint64
	lastError        string
}

// newMetrics creates a new Metrics instance
func newMetrics() *Metrics {
	return &Metrics{}
}

// incRequests increments total requests counter
func (m *Metrics) incRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestsTotal++
}

// incErrors increments error counter and sets last error message
func (m *Metrics) incErrors(errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorsTotal++
	m.lastError = errMsg
}

// incStreams increments video streams counter
func (m *Metrics) incStreams() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamsTotal++
}

// incClassify increments classify requests counter
func (m *Metrics) incClassify() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.classifyRequests++
}

// export returns metrics in Prometheus text format
func (m *Metrics) export() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Prometheus exposition format
	return fmt.Sprintf(`# HELP daaya_requests_total Total HTTP requests processed
# TYPE daaya_requests_total counter
daaya_requests_total %d
# HELP daaya_errors_total Total HTTP errors
# TYPE daaya_errors_total counter
daaya_errors_total %d
# HELP daaya_streams_total Total video streams served
# TYPE daaya_streams_total counter
daaya_streams_total %d
# HELP daaya_classify_requests_total Total classify requests
# TYPE daaya_classify_requests_total counter
daaya_classify_requests_total %d
# HELP daaya_last_error Last error message
# TYPE daaya_last_error gauge
daaya_last_error{error="%s"} 1
`, m.requestsTotal, m.errorsTotal, m.streamsTotal, m.classifyRequests, m.lastError)
}

// newIPRateLimiter creates a new IPRateLimiter
func newIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	return &IPRateLimiter{
		ips: make(map[string]*rate.Limiter),
		mu:  sync.RWMutex{},
		r:   r,
		b:   b,
	}
}

// getLimiter returns rate limiter for given IP
func (i *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(i.r, i.b)
		i.ips[ip] = limiter
	}

	return limiter
}

// rateLimitMiddleware applies rate limiting by IP
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// If parsing fails, use full RemoteAddr
			ip = r.RemoteAddr
		}

		limiter := ipRateLimiter.getLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// responseWrapper captures HTTP status code for metrics
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// metricsMiddleware increments request counters and captures errors
func metricsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Increment total requests
		metrics.incRequests()

		// Wrap response writer to capture status code
		wrapper := &responseWrapper{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapper, r)

		// Increment error counter if status >= 400
		if wrapper.statusCode >= 400 {
			metrics.incErrors(http.StatusText(wrapper.statusCode))
		}

		// Increment specific counters based on path
		if strings.HasPrefix(r.URL.Path, "/api/v1/stream/") {
			metrics.incStreams()
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/classify") {
			metrics.incClassify()
		}
	}
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract IP for logging
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		// Call the next handler
		next.ServeHTTP(w, r)

		// Log after request completes
		duration := time.Since(start)
		fmt.Printf("[%s] %s %s %s %v\n",
			time.Now().Format("2006-01-02 15:04:05"),
			ip,
			r.Method,
			r.URL.Path,
			duration)
	}
}

func loadConfig() Config {
	cfg := Config{
		VideoStorePath: "/var/daaya/videos",
		Port:           ":8182",
	}

	if envPath := os.Getenv("DAAYA_VIDEO_PATH"); envPath != "" {
		cfg.VideoStorePath = envPath
	}

	if envPort := os.Getenv("DAAYA_PORT"); envPort != "" {
		// Ensure port starts with colon if not already
		if !strings.HasPrefix(envPort, ":") {
			cfg.Port = ":" + envPort
		} else {
			cfg.Port = envPort
		}
	}

	return cfg
}
func listVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	videos, err := getVideoList(videoStorePath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(videos)
	if err != nil {
		return
	}
}

func getVideoList(videoStoreDir string) ([]VideoInfo, error) {
	var videos []VideoInfo

	err := filepath.Walk(videoStoreDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && path != videoStoreDir {
			videoInfo, err := getVideoInfo(path)
			if err != nil {
				return err
			}
			videos = append(videos, videoInfo)
		}

		return nil
	})

	return videos, err
}

func getVideoInfo(dirPath string) (VideoInfo, error) {
	var videoInfo VideoInfo

	// Helper function to read file with graceful error handling
	readMetadataFile := func(filename string) string {
		filePath := filepath.Join(dirPath, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Log but don't fail - graceful degradation
			fmt.Printf("Warning: could not read metadata file %s: %v\n", filePath, err)
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	videoInfo.Title = readMetadataFile(titleTag)
	videoInfo.Author = readMetadataFile(authorTag)
	videoInfo.Description = readMetadataFile(descTag)
	videoInfo.Classification = readMetadataFile(classificationTag)
	videoInfo.Filename = filepath.Base(dirPath)
	videoInfo.Taxonomy = parseTaxonomy(videoInfo.Classification)

	return videoInfo, nil
}

func streamVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/api/v1/stream/")

	// Validate filename to prevent path traversal
	if !isValidVideoFilename(filename) {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	videoPath := filepath.Join(videoStorePath, filename, filename+".mp4")

	// Additional security check: ensure the resolved path is within videoStorePath
	cleanPath := filepath.Clean(videoPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(videoStorePath)+string(filepath.Separator)) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	video, err := os.Open(videoPath)
	if err != nil {
		http.Error(w, "Video not found", http.StatusNotFound)
		return
	}
	defer func(video *os.File) {
		err := video.Close()
		if err != nil {
			// Log the error but don't expose it to the client
			fmt.Printf("Error closing video file: %v\n", err)
		}
	}(video)

	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, filename, time.Now(), video)
}

// isValidVideoFilename checks if a filename is safe (alphanumeric, hyphen, underscore)
func isValidVideoFilename(filename string) bool {
	if filename == "" || len(filename) > 100 {
		return false
	}

	// Check for path traversal attempts
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return false
	}

	// Allow alphanumeric, hyphen, underscore
	for _, ch := range filename {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}

	return true
}

func parseTaxonomy(classification string) Taxonomy {
	parts := strings.Split(classification, "/")
	taxonomy := Taxonomy{}

	if len(parts) >= 1 {
		taxonomy.Class = parts[0]
	}
	if len(parts) >= 2 {
		taxonomy.Order = parts[1]
	}
	if len(parts) >= 3 {
		taxonomy.Family = parts[2]
	}
	if len(parts) >= 4 {
		taxonomy.Tribe = parts[3]
	}
	if len(parts) >= 5 {
		taxonomy.Genus = parts[4]
	}

	return taxonomy
}

func classifyVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rank := strings.TrimSpace(r.URL.Query().Get("rank"))
	value := strings.TrimSpace(r.URL.Query().Get("value"))

	if rank == "" || value == "" {
		http.Error(w, "Both 'rank' and 'value' parameters are required", http.StatusBadRequest)
		return
	}

	// Validate rank parameter
	validRanks := map[string]bool{
		"class":  true,
		"order":  true,
		"family": true,
		"tribe":  true,
		"genus":  true,
	}

	if !validRanks[strings.ToLower(rank)] {
		http.Error(w, "Invalid rank parameter. Must be one of: class, order, family, tribe, genus", http.StatusBadRequest)
		return
	}

	videos, err := getVideoList(videoStorePath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	filteredVideos := filterVideosByTaxonomy(videos, rank, value)

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(filteredVideos)
	if err != nil {
		return
	}
}

func filterVideosByTaxonomy(videos []VideoInfo, rank, value string) []VideoInfo {
	var filteredVideos []VideoInfo

	for _, video := range videos {
		switch strings.ToLower(rank) {
		case "class":
			if strings.EqualFold(video.Taxonomy.Class, value) {
				filteredVideos = append(filteredVideos, video)
			}
		case "order":
			if strings.EqualFold(video.Taxonomy.Order, value) {
				filteredVideos = append(filteredVideos, video)
			}
		case "family":
			if strings.EqualFold(video.Taxonomy.Family, value) {
				filteredVideos = append(filteredVideos, video)
			}
		case "tribe":
			if strings.EqualFold(video.Taxonomy.Tribe, value) {
				filteredVideos = append(filteredVideos, video)
			}
		case "genus":
			if strings.EqualFold(video.Taxonomy.Genus, value) {
				filteredVideos = append(filteredVideos, video)
			}
		}
	}

	return filteredVideos
}

// APIEndpoint represents information about an API endpoint
type APIEndpoint struct {
	Path        string `json:"path"`
	Method      string `json:"method"`
	Description string `json:"description"`
	Parameters  string `json:"parameters,omitempty"`
}

func helpAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	endpoints := []APIEndpoint{
		{
			Path:        "/api/v1/videos",
			Method:      "GET",
			Description: "Returns a list of all available videos with their titles and descriptions.",
		},
		{
			Path:        "/api/v1/stream/{filename}",
			Method:      "GET",
			Description: "Streams the requested video file.",
			Parameters:  "{filename}: The name of the video file to stream.",
		},
		{
			Path:        "/api/v1/classify?rank=rankName&value=rankValue",
			Method:      "GET",
			Description: "Filters videos based on their taxonomic classification. For example, https://host/classify?rank=class&value=elementary",
			Parameters:  "rank: The taxonomic rank to filter by (class, order, family, tribe, or genus). value: The specific taxonomic value to filter for.",
		},
		{
			Path:        "/help",
			Method:      "GET",
			Description: "Provides information about all available API endpoints.",
		},
		{
			Path:        "/health",
			Method:      "GET",
			Description: "Health check endpoint. Returns 200 if service is healthy, 503 otherwise.",
		},
		{
			Path:        "/metrics",
			Method:      "GET",
			Description: "Prometheus metrics endpoint. Returns service metrics in text format.",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(endpoints)
	if err != nil {
		return
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if video directory is accessible
	_, err := os.Stat(videoStorePath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "unhealthy",
			"error":   fmt.Sprintf("Video directory inaccessible: %v", err),
			"message": "Service unavailable",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"message": "Service is running",
	})
}

// metricsHandler returns Prometheus-style metrics
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(metrics.export()))
}
