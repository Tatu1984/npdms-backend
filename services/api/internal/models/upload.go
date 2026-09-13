package models

import (
	"time"

	"github.com/google/uuid"
)

type Upload struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	ObjectKey        string     `json:"objectKey" db:"object_key"`
	OriginalFilename string     `json:"originalFilename" db:"original_filename"`
	FileSize         int64      `json:"fileSize" db:"file_size"`
	ContentType      string     `json:"contentType" db:"content_type"`
	Category         string     `json:"category" db:"category"`
	BucketName       string     `json:"bucketName" db:"bucket_name"`
	EntityType       *string    `json:"entityType,omitempty" db:"entity_type"`
	EntityID         *uuid.UUID `json:"entityId,omitempty" db:"entity_id"`
	UploadedBy       uuid.UUID  `json:"uploadedBy" db:"uploaded_by"`
	UploadedAt       time.Time  `json:"uploadedAt" db:"uploaded_at"`
	PublicURL        *string    `json:"publicUrl,omitempty" db:"public_url"`
	IsDeleted        bool       `json:"isDeleted" db:"is_deleted"`
	DeletedAt        *time.Time `json:"deletedAt,omitempty" db:"deleted_at"`
	DeletedBy        *uuid.UUID `json:"deletedBy,omitempty" db:"deleted_by"`
	CreatedAt        time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time  `json:"updatedAt" db:"updated_at"`
}

type UploadListParams struct {
	EntityType *string    `json:"entityType"`
	EntityID   *uuid.UUID `json:"entityId"`
	Category   *string    `json:"category"`
	UploadedBy *uuid.UUID `json:"uploadedBy"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
}
