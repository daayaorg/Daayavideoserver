package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type VideoInfo struct {
	Title          string   `json:"title"`
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
	basePath          = "/var/daaya/"
	videoStorePath    = basePath + "videos"
	descTag           = "description"
	titleTag          = "title"
	classificationTag = "classification"
)

func main() {
	http.HandleFunc("/videos", listVideos)
	http.HandleFunc("/stream/", streamVideo)
	http.HandleFunc("/classify", classifyVideos)
	http.HandleFunc("/help", helpAPI) // Add this line

	fmt.Println("Server starting on :8182")
	log.Fatal(http.ListenAndServe(":8182", nil))
}

func listVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	videos, err := getVideoList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(videos)
	if err != nil {
		return
	}
}

func getVideoList() ([]VideoInfo, error) {
	var videos []VideoInfo

	err := filepath.Walk(videoStorePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && path != videoStorePath {
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

	title, err := os.ReadFile(filepath.Join(dirPath, "title"))
	if err != nil {
		return videoInfo, err
	}
	videoInfo.Title = strings.TrimSpace(string(title))

	description, err := os.ReadFile(filepath.Join(dirPath, "description"))
	if err != nil {
		return videoInfo, err
	}
	videoInfo.Description = strings.TrimSpace(string(description))

	classification, err := os.ReadFile(filepath.Join(dirPath, "classification"))
	if err != nil {
		return videoInfo, err
	}
	videoInfo.Classification = strings.TrimSpace(string(classification))

	videoInfo.Filename = filepath.Base(dirPath)
	videoInfo.Taxonomy = parseTaxonomy(videoInfo.Classification)

	return videoInfo, nil
}
func streamVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/stream/")
	videoPath := filepath.Join(videoStorePath, filename, filename+".mp4")

	video, err := os.Open(videoPath)
	if err != nil {
		http.Error(w, "Video not found", http.StatusNotFound)
		return
	}
	defer func(video *os.File) {
		err := video.Close()
		if err != nil {

		}
	}(video)

	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, filename, time.Now(), video)
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

	rank := r.URL.Query().Get("rank")
	value := r.URL.Query().Get("value")

	if rank == "" || value == "" {
		http.Error(w, "Both 'rank' and 'value' parameters are required", http.StatusBadRequest)
		return
	}

	videos, err := getVideoList()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
			Path:        "/videos",
			Method:      "GET",
			Description: "Returns a list of all available videos with their titles and descriptions.",
		},
		{
			Path:        "/stream/{filename}",
			Method:      "GET",
			Description: "Streams the requested video file.",
			Parameters:  "{filename}: The name of the video file to stream.",
		},
		{
			Path:        "/classify",
			Method:      "GET",
			Description: "Filters videos based on their taxonomic classification.",
			Parameters:  "rank: The taxonomic rank to filter by (class, order, family, tribe, or genus). value: The specific taxonomic value to filter for.",
		},
		{
			Path:        "/help",
			Method:      "GET",
			Description: "Provides information about all available API endpoints.",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(endpoints)
	if err != nil {
		return
	}
}
