package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
	defer r.Body.Close()

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find video", err)
		return
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User doesn't own video", err)
		return
	}

	videoFile, videoFileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't form a video", err)
		return
	}
	defer videoFile.Close()

	videoCheck, _, err := mime.ParseMediaType(videoFileHeader.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find content type", err)
		return
	}
	if videoCheck != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Wrong content type", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, videoFile)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't write video to file", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't check aspect ratio", err)
		return
	}

	tempFile.Seek(0, io.SeekStart)

	ratio := "other"
	if aspectRatio == "16:9" {
		ratio = "landscape"
	}
	if aspectRatio == "9:16" {
		ratio = "portrait"
	}

	newPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't process video", err)
		return
	}
	newFile, err := os.Open(newPath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't open the new file", err)
		return
	}
	defer os.Remove(newFile.Name())
	defer newFile.Close()

	newVideoKey := fmt.Sprintf("%s/%s.mp4", ratio, videoIDString)

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         aws.String(newVideoKey),
		Body:        newFile,
		ContentType: aws.String(videoCheck),
	})
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error uploading file to S3", err)
		return
	}

	// newUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, newVideoKey)
	newUrl := fmt.Sprintf("%s,%s", cfg.s3Bucket, newVideoKey)
	videoData.VideoURL = &newUrl

	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error updating video data in db", err)
		return
	}

	videoData, err = cfg.dbVideoToSignedVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error generating signed link to video", err)
		return
	}

}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	buf := &bytes.Buffer{}
	cmd.Stdout = buf
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	type Probe struct {
		Streams []struct {
			Width  int    `json:"width"`
			Height int    `json:"height"`
			Ratio  string `json:"display_aspect_ratio"`
		} `json:"streams"`
	}

	var p Probe
	if err := json.Unmarshal(buf.Bytes(), &p); err != nil {
		return "", err
	}
	if p.Streams[0].Ratio == "16:9" || p.Streams[0].Ratio == "9:16" {
		return p.Streams[0].Ratio, nil
	}

	if (p.Streams[0].Width%16 == 0) && (p.Streams[0].Height%9 == 0) {
		return "16:9", nil
	}
	if (p.Streams[0].Width%9 == 0) && (p.Streams[0].Height%16 == 0) {
		return "9:16", nil
	}

	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return outputFilePath, nil
}
