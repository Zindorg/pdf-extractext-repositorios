package mongodb

import (
	"time"

	"github.com/marco/pdf-extractext-repositorios/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type metadataBSON struct {
	Filename  string `bson:"filename"`
	MimeType  string `bson:"mime_type"`
	SizeBytes int64  `bson:"size_bytes"`
	PageCount int    `bson:"page_count"`
}

type persistedDocument struct {
	ID               bson.ObjectID `bson:"_id"`
	DocumentID       string        `bson:"document_id"`
	Checksum         string        `bson:"checksum"`
	Status           string        `bson:"status"`
	ExtractedText    string        `bson:"extracted_text"`
	Summary          *string       `bson:"summary"`
	Metadata         metadataBSON  `bson:"metadata"`
	ProcessingTimeMS int64         `bson:"processing_time_ms"`
	CreatedAt        time.Time     `bson:"created_at"`
	UpdatedAt        time.Time     `bson:"updated_at"`
	DeletedAt        *time.Time    `bson:"deleted_at,omitempty"`
}

func toPersisted(doc domain.Document) persistedDocument {
	return persistedDocument{
		DocumentID:       doc.DocumentID,
		Checksum:         doc.Checksum,
		Status:           string(doc.Status),
		ExtractedText:    doc.ExtractedText,
		Summary:          doc.Summary,
		Metadata:         metadataBSON(doc.Metadata),
		ProcessingTimeMS: doc.ProcessingTimeMS,
		CreatedAt:        doc.CreatedAt,
		UpdatedAt:        doc.UpdatedAt,
		DeletedAt:        doc.DeletedAt,
	}
}

func toDomain(p persistedDocument) domain.Document {
	return domain.Document{
		DocumentID:       p.DocumentID,
		Checksum:         p.Checksum,
		Status:           domain.Status(p.Status),
		ExtractedText:    p.ExtractedText,
		Summary:          p.Summary,
		Metadata:         domain.Metadata(p.Metadata),
		ProcessingTimeMS: p.ProcessingTimeMS,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
		DeletedAt:        p.DeletedAt,
	}
}
