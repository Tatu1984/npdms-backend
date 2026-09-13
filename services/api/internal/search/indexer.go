package search

import (
	"context"
	"fmt"
	"log"
	"time"
)

// IndexName constants for different document types
const (
	IndexFIRs      = "npdms-firs"
	IndexCases     = "npdms-cases"
	IndexPersons   = "npdms-persons"
	IndexVehicles  = "npdms-vehicles"
	IndexEvidence  = "npdms-evidence"
	IndexPersonnel = "npdms-personnel"
	IndexWarrants  = "npdms-warrants"
	IndexAlerts    = "npdms-alerts"
	IndexChallans  = "npdms-challans"
)

// Indexer handles document indexing to OpenSearch
type Indexer struct {
	client *OpenSearchClient
}

// NewIndexer creates a new indexer
func NewIndexer(client *OpenSearchClient) *Indexer {
	return &Indexer{client: client}
}

// InitializeIndices creates all required indices with mappings
func (i *Indexer) InitializeIndices(ctx context.Context) error {
	indices := map[string]map[string]interface{}{
		IndexFIRs:      i.getFIRMappings(),
		IndexCases:     i.getCaseMappings(),
		IndexPersons:   i.getPersonMappings(),
		IndexVehicles:  i.getVehicleMappings(),
		IndexEvidence:  i.getEvidenceMappings(),
		IndexPersonnel: i.getPersonnelMappings(),
		IndexWarrants:  i.getWarrantMappings(),
		IndexAlerts:    i.getAlertMappings(),
		IndexChallans:  i.getChallanMappings(),
	}

	for name, mappings := range indices {
		if err := i.client.CreateIndex(ctx, name, mappings); err != nil {
			log.Printf("Warning: Failed to create index %s: %v", name, err)
			// Continue with other indices
		} else {
			log.Printf("Index %s ready", name)
		}
	}

	return nil
}

// getFIRMappings returns OpenSearch mappings for FIR index
func (i *Indexer) getFIRMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"fir_number": map[string]interface{}{
				"type": "keyword",
			},
			"title": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
					"suggest": map[string]interface{}{"type": "completion"},
				},
			},
			"description": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"complainant_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"complainant_phone": map[string]interface{}{
				"type": "keyword",
			},
			"complainant_address": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"accused_names": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"location": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"ipc_sections": map[string]interface{}{
				"type": "keyword",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"priority": map[string]interface{}{
				"type": "keyword",
			},
			"station_id": map[string]interface{}{
				"type": "keyword",
			},
			"investigating_officer": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
			"updated_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getCaseMappings returns OpenSearch mappings for Case index
func (i *Indexer) getCaseMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"case_number": map[string]interface{}{
				"type": "keyword",
			},
			"title": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
					"suggest": map[string]interface{}{"type": "completion"},
				},
			},
			"description": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"type": map[string]interface{}{
				"type": "keyword",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"fir_ids": map[string]interface{}{
				"type": "keyword",
			},
			"assigned_officer": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"court_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
			"updated_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getPersonMappings returns OpenSearch mappings for Person index
func (i *Indexer) getPersonMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
					"suggest": map[string]interface{}{"type": "completion"},
				},
			},
			"alias": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"father_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"phone": map[string]interface{}{
				"type": "keyword",
			},
			"email": map[string]interface{}{
				"type": "keyword",
			},
			"address": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"id_type": map[string]interface{}{
				"type": "keyword",
			},
			"id_number": map[string]interface{}{
				"type": "keyword",
			},
			"occupation": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"role": map[string]interface{}{
				"type": "keyword",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getVehicleMappings returns OpenSearch mappings for Vehicle index
func (i *Indexer) getVehicleMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"registration_number": map[string]interface{}{
				"type": "keyword",
				"fields": map[string]interface{}{
					"suggest": map[string]interface{}{"type": "completion"},
				},
			},
			"chassis_number": map[string]interface{}{
				"type": "keyword",
			},
			"engine_number": map[string]interface{}{
				"type": "keyword",
			},
			"make": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"model": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"color": map[string]interface{}{
				"type": "keyword",
			},
			"owner_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getEvidenceMappings returns OpenSearch mappings for Evidence index
func (i *Indexer) getEvidenceMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"evidence_number": map[string]interface{}{
				"type": "keyword",
			},
			"type": map[string]interface{}{
				"type": "keyword",
			},
			"description": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"location": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"case_id": map[string]interface{}{
				"type": "keyword",
			},
			"fir_id": map[string]interface{}{
				"type": "keyword",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"collected_by": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getPersonnelMappings returns OpenSearch mappings for Personnel index
func (i *Indexer) getPersonnelMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"badge_number": map[string]interface{}{
				"type": "keyword",
			},
			"name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
					"suggest": map[string]interface{}{"type": "completion"},
				},
			},
			"rank": map[string]interface{}{
				"type": "keyword",
			},
			"phone": map[string]interface{}{
				"type": "keyword",
			},
			"email": map[string]interface{}{
				"type": "keyword",
			},
			"station_id": map[string]interface{}{
				"type": "keyword",
			},
			"station_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"current_duty": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getWarrantMappings returns OpenSearch mappings for Warrant index
func (i *Indexer) getWarrantMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"warrant_number": map[string]interface{}{
				"type": "keyword",
			},
			"type": map[string]interface{}{
				"type": "keyword",
			},
			"subject_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"description": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"court_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"case_id": map[string]interface{}{
				"type": "keyword",
			},
			"valid_from": map[string]interface{}{
				"type": "date",
			},
			"valid_until": map[string]interface{}{
				"type": "date",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getAlertMappings returns OpenSearch mappings for Alert index
func (i *Indexer) getAlertMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"title": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"description": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"type": map[string]interface{}{
				"type": "keyword",
			},
			"severity": map[string]interface{}{
				"type": "keyword",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"subject_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"last_seen_location": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// getChallanMappings returns OpenSearch mappings for Traffic Challan index
func (i *Indexer) getChallanMappings() map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"challan_number": map[string]interface{}{
				"type": "keyword",
			},
			"vehicle_number": map[string]interface{}{
				"type": "keyword",
				"fields": map[string]interface{}{
					"suggest": map[string]interface{}{"type": "completion"},
				},
			},
			"driver_name": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
				"fields": map[string]interface{}{
					"keyword": map[string]interface{}{"type": "keyword"},
				},
			},
			"driver_license": map[string]interface{}{
				"type": "keyword",
			},
			"violation_type": map[string]interface{}{
				"type": "keyword",
			},
			"violation_description": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"location": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"status": map[string]interface{}{
				"type": "keyword",
			},
			"fine_amount": map[string]interface{}{
				"type": "float",
			},
			"issued_by": map[string]interface{}{
				"type":     "text",
				"analyzer": "indian_analyzer",
			},
			"created_at": map[string]interface{}{
				"type": "date",
			},
		},
	}
}

// IndexFIR indexes an FIR document
func (i *Indexer) IndexFIR(ctx context.Context, fir map[string]interface{}) error {
	id, ok := fir["id"].(string)
	if !ok {
		return fmt.Errorf("FIR missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexFIRs,
		ID:       id,
		Document: fir,
	})
}

// IndexCase indexes a Case document
func (i *Indexer) IndexCase(ctx context.Context, caseDoc map[string]interface{}) error {
	id, ok := caseDoc["id"].(string)
	if !ok {
		return fmt.Errorf("Case missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexCases,
		ID:       id,
		Document: caseDoc,
	})
}

// IndexPerson indexes a Person document
func (i *Indexer) IndexPerson(ctx context.Context, person map[string]interface{}) error {
	id, ok := person["id"].(string)
	if !ok {
		return fmt.Errorf("Person missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexPersons,
		ID:       id,
		Document: person,
	})
}

// IndexVehicle indexes a Vehicle document
func (i *Indexer) IndexVehicle(ctx context.Context, vehicle map[string]interface{}) error {
	id, ok := vehicle["id"].(string)
	if !ok {
		return fmt.Errorf("Vehicle missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexVehicles,
		ID:       id,
		Document: vehicle,
	})
}

// IndexEvidence indexes an Evidence document
func (i *Indexer) IndexEvidence(ctx context.Context, evidence map[string]interface{}) error {
	id, ok := evidence["id"].(string)
	if !ok {
		return fmt.Errorf("Evidence missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexEvidence,
		ID:       id,
		Document: evidence,
	})
}

// IndexPersonnel indexes a Personnel document
func (i *Indexer) IndexPersonnel(ctx context.Context, personnel map[string]interface{}) error {
	id, ok := personnel["id"].(string)
	if !ok {
		return fmt.Errorf("Personnel missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexPersonnel,
		ID:       id,
		Document: personnel,
	})
}

// IndexWarrant indexes a Warrant document
func (i *Indexer) IndexWarrant(ctx context.Context, warrant map[string]interface{}) error {
	id, ok := warrant["id"].(string)
	if !ok {
		return fmt.Errorf("Warrant missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexWarrants,
		ID:       id,
		Document: warrant,
	})
}

// IndexAlert indexes an Alert document
func (i *Indexer) IndexAlert(ctx context.Context, alert map[string]interface{}) error {
	id, ok := alert["id"].(string)
	if !ok {
		return fmt.Errorf("Alert missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexAlerts,
		ID:       id,
		Document: alert,
	})
}

// IndexChallan indexes a Traffic Challan document
func (i *Indexer) IndexChallan(ctx context.Context, challan map[string]interface{}) error {
	id, ok := challan["id"].(string)
	if !ok {
		return fmt.Errorf("Challan missing id field")
	}

	return i.client.Index(ctx, IndexDocument{
		Index:    IndexChallans,
		ID:       id,
		Document: challan,
	})
}

// DeleteDocument removes a document from an index
func (i *Indexer) DeleteDocument(ctx context.Context, index, id string) error {
	return i.client.Delete(ctx, index, id)
}

// ReindexAll reindexes all documents from the database
// This should be called during initial setup or after schema changes
func (i *Indexer) ReindexAll(ctx context.Context, dataProvider DataProvider) error {
	log.Println("Starting full reindex...")
	start := time.Now()

	// Initialize indices first
	if err := i.InitializeIndices(ctx); err != nil {
		return fmt.Errorf("failed to initialize indices: %w", err)
	}

	// Reindex FIRs
	firs, err := dataProvider.GetAllFIRs(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get FIRs: %v", err)
	} else {
		docs := make([]IndexDocument, len(firs))
		for idx, fir := range firs {
			docs[idx] = IndexDocument{Index: IndexFIRs, ID: fir["id"].(string), Document: fir}
		}
		if err := i.client.BulkIndex(ctx, docs); err != nil {
			log.Printf("Warning: Failed to bulk index FIRs: %v", err)
		} else {
			log.Printf("Indexed %d FIRs", len(firs))
		}
	}

	// Similar pattern for other document types...
	log.Printf("Full reindex completed in %v", time.Since(start))
	return nil
}

// DataProvider interface for fetching data from the database
type DataProvider interface {
	GetAllFIRs(ctx context.Context) ([]map[string]interface{}, error)
	GetAllCases(ctx context.Context) ([]map[string]interface{}, error)
	GetAllPersons(ctx context.Context) ([]map[string]interface{}, error)
	GetAllVehicles(ctx context.Context) ([]map[string]interface{}, error)
	GetAllEvidence(ctx context.Context) ([]map[string]interface{}, error)
	GetAllPersonnel(ctx context.Context) ([]map[string]interface{}, error)
	GetAllWarrants(ctx context.Context) ([]map[string]interface{}, error)
	GetAllAlerts(ctx context.Context) ([]map[string]interface{}, error)
	GetAllChallans(ctx context.Context) ([]map[string]interface{}, error)
}
