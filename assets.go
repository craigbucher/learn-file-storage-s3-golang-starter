package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"crypto/rand"
	"encoding/base64"
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
func getAssetPath(mediaType string) string {
	// allocates a byte slice of length 32. It’s just 32 zeroed bytes to start:
	base := make([]byte, 32)
	// fill that slice with 32 cryptographically secure random bytes using crypto/rand. 
	// The first return value is the number of bytes written (we’re ignoring it with _):
	_, err := rand.Read(base)
	// rand should never return an error, so it's fatal if it does:
	if err != nil {
		panic("failed to generate random bytes")
	}
	// convert the random 'base' to base64.RawURLEncoding, a URL-safe string:
	id := base64.RawURLEncoding.EncodeToString(base)
	// convert the mediaType to a file extension
	ext := mediaTypeToExt(mediaType)
	// concatinate the videoID and extension into a filename
	return fmt.Sprintf("%s%s", id, ext)
}

// S3 URLs are in the format https://<bucket-name>.s3.<region>.amazonaws.com/<key>. 
// Make sure you use the correct region and bucket name!
// Create a method on apiConfig that builds a public S3 object URL from a key (path/filename)
func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
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