package main

import "net/url"

type URLType int

const (
	ImageURL URLType = iota
	VideoURL
	TenorURL
	ImgurURL
)

func (t URLType) String() string {
	return [...]string{"Image", "Video", "Tenor", "Imgur"}[t]
}

type EugenURL struct {
	URL  *url.URL
	Type URLType
}
