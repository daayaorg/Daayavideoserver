package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

var (
	g errgroup.Group
)

func routerServe() http.Handler {
	e := gin.New()
	e.Use(gin.Recovery())
	e.GET("/", func(c *gin.Context) {
		c.JSON(
			http.StatusOK,
			// gin.H is a shortcut for map[string]interface{}
			gin.H{
				"code":    http.StatusOK,
				"message": "Welcome server 01",
			},
		)
	})

	return e
}

func routerUpload() http.Handler {
	e := gin.New()
	e.Use(gin.Recovery())
	e.GET("/", func(c *gin.Context) {
		c.JSON(
			http.StatusOK,
			gin.H{
				"code":    http.StatusOK,
				"message": "Welcome server 02",
			},
		)
	})
	e.MaxMultipartMemory = 8 << 20 // 8 MiB
	e.POST("/upload", func(c *gin.Context) {
		// single file
		file, _ := c.FormFile("file")
		log.Println(file.Filename)

		// Upload the file to specific dst.
		err := c.SaveUploadedFile(file, videoStorePath+"/video01")
		if err != nil {
			return
		}

		c.String(http.StatusOK, fmt.Sprintf("'%s' uploaded!", file.Filename))
	})

	return e
}

type VideoEntry struct {
	path           string
	title          string
	desc           string
	classification []string
}

func readVideos() []VideoEntry {
	entries, err := os.ReadDir(videoStorePath)
	if err != nil {
		log.Fatal(err)
		return nil
	}
	var videoEntries []VideoEntry
	for _, videoDir := range entries {
		if videoDir.IsDir() {
			videoDirEntries, err := os.ReadDir(videoStorePath + videoDir.Name())
			if err == nil {
				var videoEntry VideoEntry
				videoEntries = append(videoEntries, videoEntry)
				for _, entry := range videoDirEntries {
					if strings.HasSuffix(entry.Name(), ".mp4") {
						videoEntry.path = videoStorePath + "/" + videoDir.Name() + "/" + entry.Name()
					}
					switch entry.Name() {
					case descTag:
						bytes, err := os.ReadFile(entry.Name())
						if err == nil {
							videoEntry.desc = string(bytes)
						}
						break
					case titleTag:
						bytes, err := os.ReadFile(entry.Name())
						if err == nil {
							videoEntry.title = string(bytes)
						}
						break
					case classificationTag:
						bytes, err := os.ReadFile(entry.Name())
						if err == nil {
							videoEntry.classification = strings.Split(string(bytes), "/")
						}
						break
					}
				}
			}
		}
	}
	fmt.Println(videoEntries)
	return videoEntries

}

func main1() {
	if err := os.MkdirAll(videoStorePath, 0755); err != nil {
		return
	}

	readVideos()
	server01 := &http.Server{
		Addr:         ":8080",
		Handler:      routerServe(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	server02 := &http.Server{
		Addr:         ":8081",
		Handler:      routerUpload(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	g.Go(func() error {
		return server01.ListenAndServe()
	})

	g.Go(func() error {
		return server02.ListenAndServe()
	})

	if err := g.Wait(); err != nil {
		log.Fatal(err)
	}
}
