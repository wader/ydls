package rss

import (
	"encoding/xml"
)

// MIMEType for RSS. Seems text/xml is more used than application/rss+xml
const MIMEType = "text/xml"

// XMLNSItunes is "http://www.itunes.com/dtds/podcast-1.0.dtd"
const XMLNSItunes = "http://www.itunes.com/dtds/podcast-1.0.dtd"

// RSS <rss> root element
type RSS struct {
	XMLName     xml.Name `xml:"rss"`
	Version     string   `xml:"version,attr"` // hack for xml namespace prefix
	XMLNSItunes string   `xml:"xmlns:itunes,attr"`
	Channel     *Channel `xml:"channel"`
}

// Channel <channel> rss element
type Channel struct {
	XMLName       xml.Name     `xml:"channel"`
	Title         string       `xml:"title,omitempty"`
	Description   string       `xml:"description,omitempty"`
	Link          string       `xml:"link,omitempty"`
	LastBuildDate string       `xml:"lastBuildDate,omitempty"`
	Image         *Image       `xml:"image,omitempty"`
	ItunesImage   *ItunesImage `xml:"itunes:image,omitempty"`
	Items         []*Item      `xml:"item"`
}

// Image <image> rss>channel element
type Image struct {
	XMLName xml.Name `xml:"image"`
	Title   string   `xml:"title,omitempty"`
	URL     string   `xml:"url,omitempty"`
	Link    string   `xml:"link,omitempty"`
}

// ItunesImage <itunes:image:> rss>channel>item element
type ItunesImage struct {
	XMLName xml.Name `xml:"itunes:image"`
	HRef    string   `xml:"href,attr,omitempty"`
}

// Item <item> rss>channel element
type Item struct {
	XMLName      xml.Name     `xml:"item"`
	Title        string       `xml:"title,omitempty"`
	ItunesAuthor string       `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd author,omitempty"`
	ItunesImage  *ItunesImage `xml:"itunes:image,omitempty"`
	Link         string       `xml:"link,omitempty"`
	Description  string       `xml:"description,omitempty"`
	PubDate      string       `xml:"pubDate,omitempty"`
	GUID         string       `xml:"guid,omitempty"`
	Enclosure    *Enclosure   `xml:"enclosure"`
}

// Enclosure <enclosure> rss>channel>item element
type Enclosure struct {
	XMLName xml.Name `xml:"enclosure"`
	URL     string   `xml:"url,attr,omitempty"`
	Type    string   `xml:"type,attr,omitempty"`
	Length  string   `xml:"length,attr,omitempty"`
}
