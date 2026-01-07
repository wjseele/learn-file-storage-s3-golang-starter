package main

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	parts := strings.Split(*video.VideoURL, ",")
	bucket, key := parts[0], parts[1]
	newUrl, err := generatePresignedURL(cfg.s3Client, bucket, key, 10*time.Minute)
	if err != nil {
		return database.Video{}, err
	}
	video.VideoURL = &newUrl
	return video, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	client := s3.NewPresignClient(s3Client)
	req, err := client.PresignGetObject(
		context.Background(),
		&s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &key,
		},
		s3.WithPresignExpires(expireTime),
	)
	if err != nil {
		return "", err
	}

	return req.URL, nil
}
