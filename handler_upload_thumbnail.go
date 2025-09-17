package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory int64 = 10 << 20

	err = r.ParseMultipartForm(int64(maxMemory))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error parsing", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error forming file", err)
		return
	}
	defer file.Close()

	mediaType := fileHeader.Header.Get("Content-Type")

	typeCheck, _, _ := mime.ParseMediaType(mediaType)
	if typeCheck != "image/jpeg" && typeCheck != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Please use jpeg or png", err)
		return
	}

	// imageData, err := io.ReadAll(file)
	// if err != nil {
	// 	respondWithError(w, http.StatusBadRequest, "Error reading file", err)
	// 	return
	// }

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could't find video", err)
		return
	}

	if userID != video.UserID {
		respondWithError(w, http.StatusUnauthorized, "Wrong userID for video", err)
		return
	}
	extension := strings.SplitAfter(mediaType, "/")

	randBytes := make([]byte, 32)
	rand.Read(randBytes)
	randName := base64.RawURLEncoding.Strict().EncodeToString(randBytes)

	fileName := fmt.Sprintf("%v.%s", randName, extension[1])
	storePath := filepath.Join(cfg.assetsRoot, fileName)
	newFile, err := os.Create(storePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error creating file", err)
		return
	}

	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error writing file", err)
		return
	}

	newThumbnailURL := fmt.Sprintf("http://localhost:8091/assets/%s", fileName)

	video.ThumbnailURL = &newThumbnailURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error updating video thumbnail", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
