package main

import (
	"os"
	"reflect"
	"testing"
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
