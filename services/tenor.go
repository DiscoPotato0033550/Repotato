package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

var (
	tenorAPI string
)

func init() {
	if key, ok := os.LookupEnv("TENOR_API"); ok {
		tenorAPI = key
	} else {
		log.Fatal("TENOR_API environment variable not found")
	}
}

type tenorJSON struct {
	Results []TenorResult
}

type TenorResult struct {
	ID    string       `json:"id"`
	Tags  []string     `json:"tags"`
	URL   string       `json:"url"`
	Media []TenorMedia `json:"media"`
}

type TenorMedia struct {
	NanoMP4   Media `json:"nanomp4"`
	NanoWebm  Media `json:"nanowebm"`
	TinyGIF   Media `json:"tinygif"`
	TinyMP4   Media `json:"tinymp4"`
	TinyWebm  Media `json:"tinywebm"`
	Webm      Media `json:"webm"`
	GIF       Media `json:"gif"`
	MP4       Media `json:"mp4"`
	MediumGIF Media `json:"mediumgif"`
}

type Media struct {
	URL        string  `json:"url"`
	Dimensions []int   `json:"dims"`
	Duration   float64 `json:"duration"`
	Preview    string  `json:"preview"`
	Size       int     `json:"size"`
}

func Tenor(url string) (*TenorResult, error) {
	if strings.HasPrefix(url, "https://tenor.com/view/") {
		id := url[strings.LastIndex(url, "-")+1:]

		url = fmt.Sprintf("https://api.tenor.com/v1/gifs?ids=%v&key=%v", id, tenorAPI)
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			buf, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}

			tenor := &tenorJSON{}
			err = json.Unmarshal(buf, tenor)
			if err != nil {
				return nil, err
			}

			return &tenor.Results[0], nil
		}
	}

	return nil, nil
}
