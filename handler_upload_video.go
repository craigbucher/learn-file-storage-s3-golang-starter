package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Set an upload limit of 1 GB (1 << 30 bytes) using http.MaxBytesReader:
	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

	// Extract the videoID from the URL path parameters and parse it as a UUID:
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	// Authenticate the user to get a userID:
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}
	// Get the video metadata from the database:
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	// if the user is not the video owner, return a http.StatusUnauthorized response:
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}
	// Parse the uploaded video file from the form data:
	// Use (http.Request).FormFile with the key "video" to get a multipart.File in memory:
	file, handler, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	// Remember to defer closing the file with (os.File).Close - we don't want any memory leaks:
	defer file.Close()

	// Validate the uploaded file to ensure it's an MP4 video:
	// Use mime.ParseMediaType and "video/mp4" as the MIME type
	mediaType, _, err := mime.ParseMediaType(handler.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type, only MP4 is allowed", nil)
		return
	}

	// Save the uploaded file to a temporary file on disk:
	// Use os.CreateTemp to create a temporary file. I passed in an empty string for the directory to use 
	// the system default, and the name "tubely-upload.mp4" (but you can use whatever you want)
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create temp file", err)
		return
	}
	// defer remove the temp file with os.Remove:
	defer os.Remove(tempFile.Name())
	// defer close the temp file (defer is LIFO, so it will close before the remove):
	defer tempFile.Close()

	// io.Copy the contents over from the wire to the temp file:
	if _, err := io.Copy(tempFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not write file to disk", err)
		return
	}

	// Reset the tempFile's file pointer to the beginning with .Seek(0, io.SeekStart) - this will 
	// allow us to read the file again from the beginning:
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not reset file pointer", err)
		return
	}

	// initialize empty 'directory' string:
	directory := ""
	// Call getVideoAspectRatio (below) to get aspect ratio of video:
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error determining aspect ratio", err)
		return
	}
	switch aspectRatio {
	case "16:9":
		directory = "landscape"
	case "9:16":
		directory = "portrait"
	default:
		directory = "other"
	}

	// Generate random 32-bit hex filename with extension:
	key := getAssetPath(mediaType)
	// Join directory and key = directory/filename:
	key = path.Join(directory, key)

	// Put the object into S3 using PutObject. You'll need to provide:
	//	* The bucket name
	//	* The file key. Use the same <random-32-byte-hex>.ext format as the key
	// 	* The file contents (body). The tempFile is an os.File which implements io.Reader
	//	* Content type, which is the MIME type of the file
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        tempFile,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error uploading file to S3", err)
		return
	}

	// Update the VideoURL of the video record in the database with the S3 bucket and key:
	// (getObjectURL is in assets.go)
	url := cfg.getObjectURL(key)
	video.VideoURL = &url
	// calling the UpdateVideo method on it, passing the video object (which now has its VideoURL field populated with the S3 link)
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	// use exec.Command to run the same ffprobe command. In this case, the command is ffprobe 
	// and the arguments are -v: error, -print_format: json, -show_streams, and the file path:
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)

	//  allocate an in-memory growable buffer:
	var stdout bytes.Buffer
	// redirect the command's stdout to that buffer:
	cmd.Stdout = &stdout

	// runs the command and handle errors inline:
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffprobe error: %v", err)
	}

	// create an anonymous struct type used as a JSON target for unmarshaling ffprobe's output:
	var output struct {
		// The JSON has a top-level key 'streams' mapping to an array:
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	// take the JSON bytes from the stdout buffer (the output from the ffprobe command) and parse 
	// it into the output variable:
	// (&output uses the address-of operator because json.Unmarshal needs a pointer to the struct 
	// so it can modify it directly with the parsed JSON data)
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return "", fmt.Errorf("could not parse ffprobe output: %v", err)
	}
	// get the length (number of elements) in the Streams slice:
	// if the slice is empty (has zero elements), return an error:
	// (When ffprobe analyzes a video file, it returns information about all the streams in the file 
	// (video streams, audio streams, subtitle streams, etc.). For this function to work, we need at 
	// least one video stream to get the width and height dimensions)
	if len(output.Streams) == 0 {
		return "", errors.New("no video streams found")
	}
	// access the Width and Height fields of the first stream. These fields contain the pixel 
	// dimensions of the video:
	width := output.Streams[0].Width
	height := output.Streams[0].Height

	// return a text string of the aspect ratio:
	if width == 16*height/9 {
		return "16:9", nil
	} else if height == 16*width/9 {
		return "9:16", nil
	}
	return "other", nil
}