package main

import (
	"io"
	"os"
	"net/http"
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

	// create a relative path for the asset (a filename)
	// used for the URL that clients will use to access the file (like http://localhost:8091/assets/12345.png)
	assetPath := getAssetPath(videoID, mediaType)
	// take that relative path and converts it to a full filesystem path where the file will 
	// actually be stored on disk
	assetDiskPath := cfg.getAssetDiskPath(assetPath)

	// opens a file for writing at the given path:
	//	* If it doesn't exist, creates it. If it does, truncates it to empty
	dst, err := os.Create(assetDiskPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file on server", err)
		return
	}
	defer dst.Close()	// always defer close the file we just created
	// streams all bytes from the source file (the uploaded multipart.File) to the destination dst 
	// (the os.File you created):
	// Returns the number of bytes written and an error
	if _, err = io.Copy(dst, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
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

	// builds the public URL (e.g., http://localhost:8091/assets/<id>.<ext>) from a disk path like 
	// /assets/<id>.<ext>
	url := cfg.getAssetURL(assetPath)
	// store a pointer to that string in the video struct
	// Using a pointer allows it to be nil when absent
	video.ThumbnailURL = &url
	
	// then update the record in the database by using the cfg.db.UpdateVideo function:
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	// Respond with updated JSON of the video's metadata. Use the provided respondWithJSON function and 
	// pass it the updated database.Video struct to marshal:
	respondWithJSON(w, http.StatusOK, video)
}
