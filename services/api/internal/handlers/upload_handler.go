package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/npdms/api/internal/middleware"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

type UploadHandler struct {
	minioClient *minio.Client
	bucketName  string
	publicURL   string
	uploadRepo  *repository.UploadRepository
}

func NewUploadHandler(endpoint, accessKey, secretKey, bucket, publicURL string, useSSL bool, uploadRepo *repository.UploadRepository) (*UploadHandler, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, err
		}
	}

	return &UploadHandler{
		minioClient: client,
		bucketName:  bucket,
		publicURL:   publicURL,
		uploadRepo:  uploadRepo,
	}, nil
}

// GetPresignedURL generates a presigned URL for direct upload
func (h *UploadHandler) GetPresignedURL(c *gin.Context) {
	var req struct {
		Filename    string `json:"filename" binding:"required"`
		Category    string `json:"category" binding:"required"`
		ContentType string `json:"contentType" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request",
			Code:    400,
		})
		return
	}

	userID := middleware.GetUserID(c)

	// Generate unique object key
	ext := filepath.Ext(req.Filename)
	objectKey := fmt.Sprintf("%s/%s/%s%s",
		req.Category,
		time.Now().Format("2006/01/02"),
		uuid.New().String(),
		ext,
	)

	// Generate presigned URL (valid for 15 minutes)
	presignedURL, err := h.minioClient.PresignedPutObject(
		c.Request.Context(),
		h.bucketName,
		objectKey,
		15*time.Minute,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "upload_error",
			Message: "Failed to generate upload URL",
			Code:    500,
		})
		return
	}

	// Log the upload request
	_ = userID // TODO: Store upload metadata in database

	c.JSON(http.StatusOK, gin.H{
		"uploadUrl": presignedURL.String(),
		"objectKey": objectKey,
	})
}

// Upload handles multipart file upload
func (h *UploadHandler) Upload(c *gin.Context) {
	userID := middleware.GetUserID(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "validation_error",
			Message: "No file provided",
			Code:    400,
		})
		return
	}
	defer file.Close()

	category := c.PostForm("category")
	if category == "" {
		category = "uploads"
	}

	entityType := c.PostForm("entityType")
	entityId := c.PostForm("entityId")

	// Validate file size (max 500MB)
	if header.Size > 500*1024*1024 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "file_too_large",
			Message: "File size exceeds 500MB limit",
			Code:    400,
		})
		return
	}

	// Validate content type
	contentType := header.Header.Get("Content-Type")
	if !isAllowedContentType(contentType, category) {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_file_type",
			Message: "File type not allowed for this category",
			Code:    400,
		})
		return
	}

	// Generate unique object key
	ext := filepath.Ext(header.Filename)
	objectKey := fmt.Sprintf("%s/%s/%s%s",
		category,
		time.Now().Format("2006/01/02"),
		uuid.New().String(),
		ext,
	)

	// Add entity reference to path if provided
	if entityType != "" && entityId != "" {
		objectKey = fmt.Sprintf("%s/%s/%s/%s%s",
			category,
			entityType,
			entityId,
			uuid.New().String(),
			ext,
		)
	}

	// Upload to MinIO
	_, err = h.minioClient.PutObject(
		c.Request.Context(),
		h.bucketName,
		objectKey,
		file,
		header.Size,
		minio.PutObjectOptions{
			ContentType: contentType,
			UserMetadata: map[string]string{
				"uploaded-by":   userID.String(),
				"original-name": header.Filename,
				"entity-type":   entityType,
				"entity-id":     entityId,
			},
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "upload_error",
			Message: "Failed to upload file",
			Code:    500,
		})
		return
	}

	// Generate public URL
	fileURL := fmt.Sprintf("%s/%s/%s", h.publicURL, h.bucketName, objectKey)

	// Store upload metadata in database
	if h.uploadRepo != nil {
		upload := &models.Upload{
			ObjectKey:        objectKey,
			OriginalFilename: header.Filename,
			FileSize:         header.Size,
			ContentType:      contentType,
			Category:         category,
			BucketName:       h.bucketName,
			UploadedBy:       userID,
			PublicURL:        &fileURL,
		}

		// Set entity reference if provided
		if entityType != "" {
			upload.EntityType = &entityType
		}
		if entityId != "" {
			if eid, err := uuid.Parse(entityId); err == nil {
				upload.EntityID = &eid
			}
		}

		// Store metadata (non-blocking, just log errors)
		if err := h.uploadRepo.Create(c.Request.Context(), upload); err != nil {
			// Log error but don't fail the upload
			fmt.Printf("Warning: Failed to store upload metadata: %v\n", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"url":     fileURL,
		"key":     objectKey,
		"size":    header.Size,
		"name":    header.Filename,
	})
}

// GetFile retrieves a file by its key
func (h *UploadHandler) GetFile(c *gin.Context) {
	objectKey := c.Param("key")
	// Remove leading slash if present (from wildcard route)
	objectKey = strings.TrimPrefix(objectKey, "/")

	if objectKey == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "File key is required",
			Code:    400,
		})
		return
	}

	// Get object from MinIO
	object, err := h.minioClient.GetObject(
		c.Request.Context(),
		h.bucketName,
		objectKey,
		minio.GetObjectOptions{},
	)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "File not found",
			Code:    404,
		})
		return
	}
	defer object.Close()

	// Get object info
	info, err := object.Stat()
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "File not found",
			Code:    404,
		})
		return
	}

	// Set headers
	c.Header("Content-Type", info.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", info.Size))
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(objectKey)))

	// Stream the file
	io.Copy(c.Writer, object)
}

// DeleteFile removes a file
func (h *UploadHandler) DeleteFile(c *gin.Context) {
	objectKey := c.Param("key")
	// Remove leading slash if present (from wildcard route)
	objectKey = strings.TrimPrefix(objectKey, "/")

	if objectKey == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "File key is required",
			Code:    400,
		})
		return
	}

	userID := middleware.GetUserID(c)

	// Remove from MinIO
	err := h.minioClient.RemoveObject(
		c.Request.Context(),
		h.bucketName,
		objectKey,
		minio.RemoveObjectOptions{},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "delete_error",
			Message: "Failed to delete file",
			Code:    500,
		})
		return
	}

	// Soft delete metadata from database
	if h.uploadRepo != nil {
		upload, err := h.uploadRepo.GetByObjectKey(c.Request.Context(), objectKey)
		if err == nil && upload != nil {
			if err := h.uploadRepo.SoftDelete(c.Request.Context(), upload.ID, userID); err != nil {
				fmt.Printf("Warning: Failed to soft delete upload metadata: %v\n", err)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File deleted",
	})
}

func isAllowedContentType(contentType, category string) bool {
	// Evidence allows all types
	if category == "evidence" {
		return true
	}

	allowedTypes := map[string][]string{
		"photos": {
			"image/jpeg",
			"image/png",
			"image/gif",
			"image/webp",
		},
		"documents": {
			"application/pdf",
			"application/msword",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.ms-excel",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"text/plain",
		},
		"audio": {
			"audio/mpeg",
			"audio/wav",
			"audio/ogg",
			"audio/mp4",
		},
		"video": {
			"video/mp4",
			"video/webm",
			"video/ogg",
			"video/quicktime",
		},
	}

	allowed, exists := allowedTypes[category]
	if !exists {
		return true // Unknown category, allow all
	}

	for _, t := range allowed {
		if strings.HasPrefix(contentType, t) || contentType == t {
			return true
		}
	}

	return false
}
