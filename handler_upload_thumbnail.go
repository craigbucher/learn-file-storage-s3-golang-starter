package main

import (
	"fmt"
	"net/http"
	"io"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

// This is a method on the apiConfig struct (indicated by (cfg *apiConfig)). It's an HTTP handler function that:
// 	* Takes a ResponseWriter (w) to write the HTTP response back to the client
// 	* Takes a Request (r) containing the incoming HTTP request data
// 	* The cfg receiver gives access to your application's configuration, including the database connection
func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	// Extract a path parameter called "videoID" from the URL:
	// (The path parameters are specified in the route configuration)
	videoIDString := r.PathValue("videoID")
	// convert the string into a proper UUID type:
	// uuid.Parse() attempts to parse the string as a valid UUID format:
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// calls the 'GetBearerToken' function from auth.go to extract a Bearer token from the request headers:
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// call the ValidateJWT function from the auth.go package, passing in:
	// 	* token: The JWT token (usually extracted from the request headers)
	// 	* cfg.jwtSecret: A secret key used to verify the token's authenticity
	// (userID: If the token is valid, this contains the user's ID that was encoded in the token)
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// Parse the form data:
	// Set a const maxMemory to 10MB. I just bit-shifted the number 10 to the left 20 
	// times to get an int that stores the proper number of bytes:
	const maxMemory = 10 << 20 // 10 MB
	// Use (http.Request).ParseMultipartForm with the maxMemory const as an argument:
	r.ParseMultipartForm(maxMemory)

	// Get the image data from the form:
	// Use r.FormFile to get the file data and file headers. The key the web browser 
	// is using is called "thumbnail":
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Get the media type from the form file's Content-Type header:
	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
		return
	}

	// Read all the image data into a byte slice using io.ReadAll:
	data, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reading file", err)
		return
	}

	// Get the video's metadata from the SQLite database. The apiConfig's db has a GetVideo method you can use:
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	// If the authenticated user is not the video owner, return a http.StatusUnauthorized response:
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	// Save the thumbnail to the global map:
	// Create a new thumbnail struct with the image data and media type:
	// Add the thumbnail to the global map, using the video's ID as the key:
	videoThumbnails[videoID] = thumbnail{
		data:      data,
		mediaType: mediaType,
	}

	// Update the video metadata so that it has a new thumbnail URL
	// The thumbnail URL should have this format:
	// http://localhost:<port>/api/thumbnails/{videoID}
	url := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID)
	video.ThumbnailURL = &url

	// then update the record in the database by using the cfg.db.UpdateVideo function:
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		delete(videoThumbnails, videoID)
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	// Respond with updated JSON of the video's metadata. Use the provided respondWithJSON function and 
	// pass it the updated database.Video struct to marshal:
	respondWithJSON(w, http.StatusOK, video)
}
