package ydls

import (
	"net/url"
	"time"

	"github.com/wader/ydls/internal/rss"
	"github.com/wader/ydls/internal/youtubedl"
)

func RSSFromYDLSInfo(options DownloadOptions, info youtubedl.Info, linkIconRawURL string) rss.RSS {
	enclosureDownloadOptions := options.Format.EnclosureDownloadOptions
	baseURL := options.BaseURL
	if baseURL == nil {
		baseURL = &url.URL{}
	}

	feedURL := baseURL.ResolveReference(
		&url.URL{Path: enclosureDownloadOptions.Format.Name + "/" + info.WebpageURL},
	)

	channel := &rss.Channel{
		Title:       firstNonEmpty(info.Title, info.PlaylistTitle, info.Artist, info.Creator, info.Uploader),
		Description: info.Description,
		Link:        info.WebpageURL,
	}

	thumbnail := firstNonEmpty(info.Thumbnail, linkIconRawURL)
	if thumbnail != "" {
		channel.Image = &rss.Image{
			URL:   thumbnail,
			Title: info.Title,
			Link:  info.WebpageURL,
		}
		channel.ItunesImage = thumbnail
	}

	for _, entry := range info.Entries {
		// skip nested playlists
		if entry.Type == "playlist" || entry.Type == "multi_video" {
			continue
		}

		GUID := feedURL.ResolveReference(&url.URL{
			Fragment: entry.ID,
		}).String()

		entryDownloadOptions := options.Format.EnclosureDownloadOptions
		entryDownloadOptions.MediaRawURL = entry.WebpageURL
		entryDownloadOptions.BaseURL = baseURL.ResolveReference(
			// itunes requires url path to end with .mp3 etc
			&url.URL{Path: "media." + enclosureDownloadOptions.Format.Ext},
		)

		enclosure := &rss.Enclosure{
			URL:  entryDownloadOptions.URL().String(),
			Type: enclosureDownloadOptions.Format.MIMEType,
		}

		pubDate := ""
		if entry.UploadDate != "" {
			if d, err := time.Parse("20060102", entry.UploadDate); err == nil {
				pubDate = d.Format(time.RFC1123Z)
			}
		}

		channel.Items = append(channel.Items, &rss.Item{
			GUID:         GUID,
			PubDate:      pubDate,
			ItunesAuthor: entry.Artist,
			ItunesImage:  entry.Thumbnail,
			Link:         entry.WebpageURL,
			Title:        firstNonEmpty(entry.Title, entry.Episode),
			Description:  entry.Description,
			Enclosure:    enclosure,
		})
	}

	return rss.RSS{
		Version:     "2.0",
		XMLNSItunes: rss.XMLNSItunes,
		Channel:     channel,
	}
}
