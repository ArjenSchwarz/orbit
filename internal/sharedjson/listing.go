package sharedjson

import (
	"cmp"
	"time"

	"github.com/arjenschwarz/orbit/internal/sessions"
)

// ListingDocument lists sessions oldest first, with listing warnings.
type ListingDocument struct {
	SchemaVersion int              `json:"schema_version"`
	Kind          string           `json:"kind"`
	Scope         Scope            `json:"scope"`
	Sessions      []ListingRow     `json:"sessions"`
	Warnings      []ListingWarning `json:"warnings"`
}

// Scope is the listing's scope: one project, or all projects.
type Scope struct {
	Project     string `json:"project,omitempty"`
	AllProjects bool   `json:"all_projects,omitempty"`
}

// ListingRow is a listed session: the shared session metadata plus listing fields.
type ListingRow struct {
	Metadata
	SizeBytes      int64  `json:"size_bytes"`
	FileModifiedAt string `json:"file_modified_at,omitempty"` // filesystem time that orders a session without a recorded one
}

// ListingWarning is a non-fatal listing problem and the source it came from.
type ListingWarning struct {
	Source  string `json:"source"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Listing projects listed sessions in the order given, which for the CLI is
// oldest first. Listing metadata comes from the lister's bounded reads; the
// session document is authoritative.
func Listing(list []sessions.SessionInfo, warnings []sessions.ListWarning, scope Scope) ListingDocument {
	doc := ListingDocument{
		SchemaVersion: SchemaVersion,
		Kind:          "listing",
		Scope:         scope,
		Sessions:      make([]ListingRow, 0, len(list)),
		Warnings:      make([]ListingWarning, 0, len(warnings)),
	}
	for _, s := range list {
		doc.Sessions = append(doc.Sessions, listingRow(s))
	}
	for _, w := range warnings {
		lw := ListingWarning{Source: w.Source, Code: cmp.Or(w.Code, sessions.WarningSourceFailed)}
		if w.Err != nil {
			lw.Message = w.Err.Error()
		}
		doc.Warnings = append(doc.Warnings, lw)
	}
	return doc
}

func listingRow(s sessions.SessionInfo) ListingRow {
	row := ListingRow{
		Metadata: Metadata{
			Source:   s.Source,
			ID:       s.ID,
			ThreadID: s.ThreadID,
			Project:  s.Project,
			Model:    s.Model,
			Provider: s.Provider,
			Origin:   s.Origin,
		},
		SizeBytes: s.Size,
	}
	if !s.CreatedAt.IsZero() {
		ts := s.CreatedAt.UTC().Format(time.RFC3339Nano)
		if s.TimeFromFilesystem {
			row.FileModifiedAt = ts
		} else {
			row.CreatedAt = ts
		}
	}
	return row
}
