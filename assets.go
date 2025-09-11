package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// ensure a directory exists at cfg.assetsRoot:
func (cfg apiConfig) ensureAssetsDir() error {
	// os.Stat(cfg.assetsRoot): checks file/dir info.
	// If it returns an error and os.IsNotExist(err) is true, the path doesn't exist.
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		// if doesn't exist, create the directory with permissions rwxr-xr-x
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	// if it does exist, return nil:
	return nil
}

// Generate the filename:
func getAssetPath(videoID uuid.UUID, mediaType string) string {
	// convert the mediaType to a file extension
	ext := mediaTypeToExt(mediaType)
	// concatinate the videoID and extension into a filename
	return fmt.Sprintf("%s%s", videoID, ext)
}

// filepath.Join(cfg.assetsRoot, assetPath) safely builds an OS-correct path by joining the assets root 
// directory with the relative asset path:
func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

// create the URL for the file:
func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

// map a MIME type to a file extension:
//	* Split the string on "/"", e.g. "image/png" -> ["image","png"]
//	* If it doesn�t split into exactly two parts, returns a default ".bin"
//	* Otherwise returns "." + the subtype, e.g. ".png", ".jpeg", ".mp4"
func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}