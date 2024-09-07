package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/acme/autocert"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	basePath          = "/var/daaya/"
	videoStorePath    = basePath + "videos"
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
	server := &http.Server{
		Addr: ":https",
		TLSConfig: &tls.Config{
			GetCertificate: certManager.GetCertificate,
			MinVersion:     tls.VersionTLS11, // improves cert reputation score at https://www.ssllabs.com/ssltest/
		},
	}
	http.HandleFunc("/api/v1/videos", listVideos)
	http.HandleFunc("/api/v1/stream/", streamVideo)
	http.HandleFunc("/api/v1/classify", classifyVideos)
	http.HandleFunc("/help", helpAPI) // Add this line

	fmt.Println("Server starting on :80 and :443")
	go func() {
		err := http.ListenAndServe(":http", certManager.HTTPHandler(nil))
		if err != nil {
			log.Fatal(err)
		}
	}()
	log.Fatal(server.ListenAndServeTLS("", "")) //Key and cert are coming from Let's Encrypt

	// old code. remove after testing
	// log.Fatal(http.ListenAndServe(":8182", nil))

}

func listVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	videos, err := getVideoList(videoStorePath)
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

	pathStr := filepath.Join(dirPath, titleTag)
	title, err := os.ReadFile(pathStr)
	if err != nil {
		fmt.Printf("could not find 'title' in %s, error=%v\n", pathStr, err)
		//dont return error here. The title was not found. return an empty title
		//return videoInfo, err
	}
	videoInfo.Title = strings.TrimSpace(string(title))

	pathStr = filepath.Join(dirPath, authorTag)
	author, err := os.ReadFile(pathStr)
	if err != nil {
		fmt.Printf("could not find 'author' in %s, error=%v\n", pathStr, err)
		//dont return error here. The author was not found. return an empty author
		//return videoInfo, err
	}
	videoInfo.Author = strings.TrimSpace(string(author))

	description, err := os.ReadFile(filepath.Join(dirPath, descTag))
	if err != nil {
		fmt.Printf("could not find 'description' in %s, error=%v\n", pathStr, err)
		//dont return error here. The description was not found. return an empty description
		//return videoInfo, err
	}
	videoInfo.Description = strings.TrimSpace(string(description))

	classification, err := os.ReadFile(filepath.Join(dirPath, classificationTag))
	if err != nil {
		fmt.Printf("could not find 'classification' in %s, error=%v\n", pathStr, err)
		//dont return error here. The classification was not found. return an empty classification
		//return videoInfo, err
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

	filename := strings.TrimPrefix(r.URL.Path, "/api/v1/stream/")
	videoPath := filepath.Join(videoStorePath, filename, filename+".mp4")

	println("videoPath=%s", videoPath)

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

	videos, err := getVideoList(videoStorePath)
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
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(endpoints)
	if err != nil {
		return
	}
}
