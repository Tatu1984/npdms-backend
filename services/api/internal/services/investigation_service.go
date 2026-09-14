package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
)

// InvestigationService implements Phase 01.
//
// The gap rules below are ordinary deterministic checks over the case record —
// counts, dates, presence of documents. There is no model involved, and each
// rule states plainly what it looked at. When AI arrives it will add
// suggestions alongside these, marked with origin "ai"; it will not replace
// them.
type InvestigationService struct {
	repo      *repository.InvestigationRepository
	auditRepo *repository.AuditRepository
	db        *pgxpool.Pool
}

func NewInvestigationService(repo *repository.InvestigationRepository, auditRepo *repository.AuditRepository, db *pgxpool.Pool) *InvestigationService {
	return &InvestigationService{repo: repo, auditRepo: auditRepo, db: db}
}

func (s *InvestigationService) audit(ctx context.Context, actor *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, description string) {
	if s.auditRepo == nil {
		return
	}
	// A failed audit write is itself significant in a police system: it must be
	// visible in the server log rather than discarded.
	if err := s.auditRepo.Log(ctx, &models.SimpleAuditLog{
		UserID:       actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Description:  &description,
		Success:      true,
	}); err != nil {
		log.Printf("audit write failed for %s on %s: %v", action, resourceType, err)
	}
}

/* ------------------------------- workspaces ------------------------------- */

func (s *InvestigationService) ListWorkspaces(ctx context.Context, filter repository.WorkspaceFilter) (*models.PaginatedResponse, error) {
	items, total, err := s.repo.ListWorkspaces(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := 0
	if filter.PageSize > 0 {
		totalPages = int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}

	return &models.PaginatedResponse{
		Data:       items,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *InvestigationService) GetWorkspace(ctx context.Context, id uuid.UUID) (*models.InvestigationWorkspace, error) {
	return s.repo.GetWorkspace(ctx, id)
}

func (s *InvestigationService) CreateWorkspace(ctx context.Context, req models.CreateWorkspaceRequest, actor *uuid.UUID) (*models.InvestigationWorkspace, error) {
	ws, err := s.repo.CreateWorkspace(ctx, req, actor)
	if err != nil {
		return nil, err
	}

	// A new workspace immediately gets its deterministic gaps so the officer
	// sees what the file is missing from the first screen.
	if _, err := s.RecomputeGaps(ctx, ws.ID); err != nil {
		return nil, err
	}

	s.audit(ctx, actor, "investigation_workspace_created", "investigation_workspace", &ws.ID,
		fmt.Sprintf("Workspace opened for %s", ws.CaseNumber))

	return s.repo.GetWorkspace(ctx, ws.ID)
}

func (s *InvestigationService) UpdateWorkspace(ctx context.Context, id uuid.UUID, req models.UpdateWorkspaceRequest, actor *uuid.UUID) (*models.InvestigationWorkspace, error) {
	ws, err := s.repo.UpdateWorkspace(ctx, id, req)
	if err != nil {
		return nil, err
	}

	description := "Workspace updated"
	if req.IOID != nil {
		description = "Investigating officer reassigned"
		if req.ReassignReason != nil && *req.ReassignReason != "" {
			description += ": " + *req.ReassignReason
		}
	} else if req.Status != nil {
		description = "Status changed to " + *req.Status
	}
	s.audit(ctx, actor, "investigation_workspace_updated", "investigation_workspace", &id, description)

	return ws, nil
}

/* --------------------------------- persons -------------------------------- */

func (s *InvestigationService) ListPersons(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspacePerson, error) {
	return s.repo.ListPersons(ctx, workspaceID)
}

func (s *InvestigationService) CreatePerson(ctx context.Context, workspaceID uuid.UUID, req models.CreatePersonRequest, actor *uuid.UUID) (*models.WorkspacePerson, error) {
	p, err := s.repo.CreatePerson(ctx, workspaceID, req, actor)
	if err != nil {
		return nil, err
	}
	// Adding a witness can open the "statement not recorded" gap.
	if _, err := s.RecomputeGaps(ctx, workspaceID); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "investigation_person_added", "workspace_person", &p.ID,
		fmt.Sprintf("%s added as %s", p.Name, p.Role))
	return p, nil
}

// UpdatePerson records developments about a person — notably that a statement
// has been taken, which is what closes the witness-statement gap.
func (s *InvestigationService) UpdatePerson(ctx context.Context, workspaceID, personID uuid.UUID, req models.UpdatePersonRequest, actor *uuid.UUID) ([]models.WorkspacePerson, error) {
	if err := s.repo.UpdatePerson(ctx, personID, req); err != nil {
		return nil, err
	}
	if _, err := s.RecomputeGaps(ctx, workspaceID); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "investigation_person_updated", "workspace_person", &personID, "Person record updated")
	return s.repo.ListPersons(ctx, workspaceID)
}

func (s *InvestigationService) DeletePerson(ctx context.Context, workspaceID, personID uuid.UUID, actor *uuid.UUID) error {
	if err := s.repo.DeletePerson(ctx, personID); err != nil {
		return err
	}
	_, err := s.RecomputeGaps(ctx, workspaceID)
	s.audit(ctx, actor, "investigation_person_removed", "workspace_person", &personID, "Person removed from workspace")
	return err
}

/* -------------------------------- timeline -------------------------------- */

func (s *InvestigationService) ListTimeline(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspaceTimelineEntry, error) {
	return s.repo.ListTimeline(ctx, workspaceID)
}

func (s *InvestigationService) CreateWorkspaceTimelineEntry(ctx context.Context, workspaceID uuid.UUID, req models.CreateTimelineRequest, actor *uuid.UUID) (*models.WorkspaceTimelineEntry, error) {
	e, err := s.repo.CreateWorkspaceTimelineEntry(ctx, workspaceID, req, actor)
	if err != nil {
		return nil, err
	}
	if _, err := s.RecomputeGaps(ctx, workspaceID); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "investigation_timeline_added", "workspace_timeline", &e.ID, e.Title)
	return e, nil
}

func (s *InvestigationService) ReviewWorkspaceTimelineEntry(ctx context.Context, entryID uuid.UUID, state string, actor *uuid.UUID) error {
	if err := s.repo.ReviewWorkspaceTimelineEntry(ctx, entryID, state, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "investigation_timeline_reviewed", "workspace_timeline", &entryID, "Marked "+state)
	return nil
}

func (s *InvestigationService) DeleteWorkspaceTimelineEntry(ctx context.Context, workspaceID, entryID uuid.UUID, actor *uuid.UUID) error {
	if err := s.repo.DeleteWorkspaceTimelineEntry(ctx, entryID); err != nil {
		return err
	}
	_, err := s.RecomputeGaps(ctx, workspaceID)
	s.audit(ctx, actor, "investigation_timeline_removed", "workspace_timeline", &entryID, "Timeline entry removed")
	return err
}

/* ----------------------------- contradictions ----------------------------- */

func (s *InvestigationService) ListContradictions(ctx context.Context, workspaceID uuid.UUID) ([]models.Contradiction, error) {
	return s.repo.ListContradictions(ctx, workspaceID)
}

func (s *InvestigationService) CreateContradiction(ctx context.Context, workspaceID uuid.UUID, req models.CreateContradictionRequest, actor *uuid.UUID) (*models.Contradiction, error) {
	c, err := s.repo.CreateContradiction(ctx, workspaceID, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "investigation_contradiction_recorded", "workspace_contradiction", &c.ID, c.Title)
	return c, nil
}

func (s *InvestigationService) ReviewContradiction(ctx context.Context, id uuid.UUID, state string, note *string, actor *uuid.UUID) error {
	if err := s.repo.ReviewContradiction(ctx, id, state, note, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "investigation_contradiction_reviewed", "workspace_contradiction", &id, "Marked "+state)
	return nil
}

/* ----------------------------------- gaps --------------------------------- */

func (s *InvestigationService) ListGaps(ctx context.Context, workspaceID uuid.UUID, includeClosed bool) ([]models.InvestigationGap, error) {
	return s.repo.ListGaps(ctx, workspaceID, includeClosed)
}

func (s *InvestigationService) CreateGap(ctx context.Context, workspaceID uuid.UUID, req models.CreateGapRequest, actor *uuid.UUID) (*models.InvestigationGap, error) {
	g, err := s.repo.CreateGap(ctx, workspaceID, req)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "investigation_gap_added", "workspace_gap", &g.ID, g.Title)
	return g, nil
}

func (s *InvestigationService) UpdateGapStatus(ctx context.Context, id uuid.UUID, status string, actor *uuid.UUID) error {
	if err := s.repo.UpdateGapStatus(ctx, id, status, actor); err != nil {
		return err
	}
	s.audit(ctx, actor, "investigation_gap_"+status, "workspace_gap", &id, "Gap marked "+status)
	return nil
}

// gapRule is one deterministic check. `applies` decides whether the gap stands;
// everything else is the text shown to the officer.
type gapRule struct {
	key      string
	kind     string
	severity string
	title    string
	titleBn  string
	detail   func(f *repository.GapFacts) string
	detailBn func(f *repository.GapFacts) string
	applies  func(f *repository.GapFacts) bool
}

// The rule set. Each one is an ordinary condition over the case record.
var gapRules = []gapRule{
	{
		key:      "no-evidence",
		kind:     "seizure",
		severity: "high",
		title:    "No evidence has been attached to the case file",
		titleBn:  "মামলার নথিতে কোনো সাক্ষ্যপ্রমাণ সংযুক্ত করা হয়নি",
		applies:  func(f *repository.GapFacts) bool { return f.EvidenceCount == 0 },
		detail: func(f *repository.GapFacts) string {
			return "Nothing has been linked to this workspace yet. Seizure lists, photographs and any recovered material should be registered and attached."
		},
		detailBn: func(f *repository.GapFacts) string {
			return "এই কর্মক্ষেত্রে এখনও কিছু সংযুক্ত করা হয়নি। বাজেয়াপ্ত তালিকা, আলোকচিত্র ও উদ্ধারকৃত সামগ্রী নথিভুক্ত করে সংযুক্ত করা উচিত।"
		},
	},
	{
		key:      "witness-statement-missing",
		kind:     "witness",
		severity: "high",
		title:    "Statements not recorded for every listed witness",
		titleBn:  "তালিকাভুক্ত প্রত্যেক সাক্ষীর বয়ান লিপিবদ্ধ হয়নি",
		applies: func(f *repository.GapFacts) bool {
			return f.WitnessCount > 0 && f.WitnessStatements < f.WitnessCount
		},
		detail: func(f *repository.GapFacts) string {
			return fmt.Sprintf("%d witness(es) are listed on this case but only %d statement(s) have been recorded under BNSS 180.",
				f.WitnessCount, f.WitnessStatements)
		},
		detailBn: func(f *repository.GapFacts) string {
			return fmt.Sprintf("এই মামলায় %d জন সাক্ষী তালিকাভুক্ত, কিন্তু বিএনএসএস ১৮০ ধারায় মাত্র %d টি বয়ান লিপিবদ্ধ হয়েছে।",
				f.WitnessCount, f.WitnessStatements)
		},
	},
	{
		key:      "no-witness",
		kind:     "witness",
		severity: "medium",
		title:    "No witness has been recorded",
		titleBn:  "কোনো সাক্ষী নথিভুক্ত হয়নি",
		applies:  func(f *repository.GapFacts) bool { return f.WitnessCount == 0 },
		detail: func(f *repository.GapFacts) string {
			return "The workspace lists no witnesses. If the occurrence had none, record that fact in the case diary."
		},
		detailBn: func(f *repository.GapFacts) string {
			return "কর্মক্ষেত্রে কোনো সাক্ষী তালিকাভুক্ত নেই। ঘটনার কোনো সাক্ষী না থাকলে কেস ডায়েরিতে তা উল্লেখ করুন।"
		},
	},
	{
		key:      "empty-timeline",
		kind:     "timeline",
		severity: "medium",
		title:    "No chronology has been built",
		titleBn:  "কোনো ঘটনাক্রম তৈরি হয়নি",
		applies:  func(f *repository.GapFacts) bool { return f.TimelineCount == 0 },
		detail: func(f *repository.GapFacts) string {
			return "No timeline entries exist. A chronology of the occurrence makes contradictions and unexplained intervals visible."
		},
		detailBn: func(f *repository.GapFacts) string {
			return "কোনো সময়রেখা নেই। ঘটনার ধারাবাহিক বিবরণ থাকলে অসঙ্গতি ও ব্যাখ্যাহীন সময় সহজে চোখে পড়ে।"
		},
	},
	{
		key:      "timeline-interval",
		kind:     "timeline",
		severity: "medium",
		title:    "Unexplained interval in the chronology",
		titleBn:  "ঘটনাক্রমে ব্যাখ্যাহীন সময়ের ফাঁক",
		applies: func(f *repository.GapFacts) bool {
			return f.TimelineCount >= 2 && f.LargestTimelineGapMinutes > 60
		},
		detail: func(f *repository.GapFacts) string {
			return fmt.Sprintf("The largest interval between consecutive timeline entries is %d minutes. Consider whether camera, call or witness material can account for it.",
				f.LargestTimelineGapMinutes)
		},
		detailBn: func(f *repository.GapFacts) string {
			return fmt.Sprintf("পরপর দুটি সময়রেখা এন্ট্রির মধ্যে সর্বোচ্চ ব্যবধান %d মিনিট। ক্যামেরা, কল বা সাক্ষীর তথ্য দিয়ে তা ব্যাখ্যা করা যায় কি না দেখুন।",
				f.LargestTimelineGapMinutes)
		},
	},
	{
		key:      "no-accused",
		kind:     "document",
		severity: "low",
		title:    "No accused or suspect recorded",
		titleBn:  "কোনো অভিযুক্ত বা সন্দেহভাজন নথিভুক্ত হয়নি",
		applies:  func(f *repository.GapFacts) bool { return f.AccusedCount == 0 },
		detail: func(f *repository.GapFacts) string {
			return "No person has been recorded as accused. In an untraced case, record the position in the case diary."
		},
		detailBn: func(f *repository.GapFacts) string {
			return "কোনো ব্যক্তিকে অভিযুক্ত হিসেবে নথিভুক্ত করা হয়নি। অজ্ঞাতপরিচয় মামলা হলে কেস ডায়েরিতে অবস্থান লিপিবদ্ধ করুন।"
		},
	},
}

// RecomputeGaps applies every rule and reconciles the derived gaps: a rule that
// now holds is upserted, one that no longer holds is closed. Officer-created
// gaps are never touched.
func (s *InvestigationService) RecomputeGaps(ctx context.Context, workspaceID uuid.UUID) ([]models.InvestigationGap, error) {
	facts, err := s.repo.GapFacts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	for _, rule := range gapRules {
		if rule.applies(facts) {
			var due *time.Time
			if rule.severity == "high" || rule.severity == "critical" {
				d := time.Now().AddDate(0, 0, 7)
				due = &d
			}
			if err := s.repo.UpsertRuleGap(ctx, workspaceID, rule.key, rule.title, rule.titleBn,
				rule.detail(facts), rule.detailBn(facts), rule.kind, rule.severity, due); err != nil {
				return nil, err
			}
		} else if err := s.repo.CloseRuleGap(ctx, workspaceID, rule.key); err != nil {
			return nil, err
		}
	}

	return s.repo.ListGaps(ctx, workspaceID, false)
}

/* ---------------------------------- tasks --------------------------------- */

func (s *InvestigationService) ListTasks(ctx context.Context, workspaceID uuid.UUID) ([]models.InvestigationTask, error) {
	return s.repo.ListTasks(ctx, workspaceID)
}

func (s *InvestigationService) CreateTask(ctx context.Context, workspaceID uuid.UUID, req models.CreateTaskRequest, actor *uuid.UUID) (*models.InvestigationTask, error) {
	t, err := s.repo.CreateTask(ctx, workspaceID, req, actor)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "investigation_task_created", "investigation_task", &t.ID, t.Title)
	return t, nil
}

func (s *InvestigationService) UpdateTask(ctx context.Context, workspaceID, taskID uuid.UUID, req models.UpdateTaskRequest, actor *uuid.UUID) ([]models.InvestigationTask, error) {
	if err := s.repo.UpdateTask(ctx, taskID, req, actor); err != nil {
		return nil, err
	}

	// Completing a task that was raised against a gap does not by itself mean
	// the gap is answered. An officer-created gap is closed directly, because
	// the officer who raised it is the one closing it. A derived gap is left to
	// the rules: the recompute below closes it only if its condition no longer
	// holds, so the file cannot be marked complete while the material is still
	// missing.
	if req.Status != nil && *req.Status == "done" {
		tasks, err := s.repo.ListTasks(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		for _, t := range tasks {
			if t.ID != taskID || t.GapID == nil {
				continue
			}
			gaps, err := s.repo.ListGaps(ctx, workspaceID, true)
			if err != nil {
				return nil, err
			}
			for _, g := range gaps {
				if g.ID == *t.GapID && g.Origin == models.OriginOfficer {
					if err := s.repo.UpdateGapStatus(ctx, g.ID, "closed", actor); err != nil {
						return nil, err
					}
				}
			}
		}
		if _, err := s.RecomputeGaps(ctx, workspaceID); err != nil {
			return nil, err
		}
	}

	s.audit(ctx, actor, "investigation_task_updated", "investigation_task", &taskID, "Task updated")
	return s.repo.ListTasks(ctx, workspaceID)
}

func (s *InvestigationService) DeleteTask(ctx context.Context, taskID uuid.UUID, actor *uuid.UUID) error {
	if err := s.repo.DeleteTask(ctx, taskID); err != nil {
		return err
	}
	s.audit(ctx, actor, "investigation_task_deleted", "investigation_task", &taskID, "Task deleted")
	return nil
}

/* --------------------------- officers and links --------------------------- */

// ListOfficers backs the assignment picker.
func (s *InvestigationService) ListOfficers(ctx context.Context, search string, stationID *uuid.UUID) ([]models.Officer, error) {
	return s.repo.ListOfficers(ctx, search, stationID)
}

// LinkGraph returns the relationships recorded on a case.
func (s *InvestigationService) LinkGraph(ctx context.Context, workspaceID uuid.UUID) (*models.LinkGraph, error) {
	return s.repo.LinkGraph(ctx, workspaceID)
}

/* --------------------------------- evidence ------------------------------- */

func (s *InvestigationService) ListEvidence(ctx context.Context, workspaceID uuid.UUID) ([]models.WorkspaceEvidenceLink, error) {
	return s.repo.ListEvidence(ctx, workspaceID)
}

func (s *InvestigationService) LinkEvidence(ctx context.Context, workspaceID uuid.UUID, req models.LinkEvidenceRequest, actor *uuid.UUID) ([]models.WorkspaceEvidenceLink, error) {
	if err := s.repo.LinkEvidence(ctx, workspaceID, req.EvidenceID, req.Note, actor); err != nil {
		return nil, err
	}
	if _, err := s.RecomputeGaps(ctx, workspaceID); err != nil {
		return nil, err
	}
	s.audit(ctx, actor, "investigation_evidence_linked", "workspace_evidence", &req.EvidenceID, "Evidence linked to workspace")
	return s.repo.ListEvidence(ctx, workspaceID)
}

func (s *InvestigationService) UnlinkEvidence(ctx context.Context, workspaceID, evidenceID uuid.UUID, actor *uuid.UUID) error {
	if err := s.repo.UnlinkEvidence(ctx, workspaceID, evidenceID); err != nil {
		return err
	}
	_, err := s.RecomputeGaps(ctx, workspaceID)
	s.audit(ctx, actor, "investigation_evidence_unlinked", "workspace_evidence", &evidenceID, "Evidence unlinked")
	return err
}

/* ----------------------------------- brief -------------------------------- */

// Brief assembles the case file as it stands. It is a compilation of recorded
// material, not a generated narrative — every line traces to a row an officer
// entered.
func (s *InvestigationService) Brief(ctx context.Context, workspaceID uuid.UUID) (*models.WorkspaceBrief, error) {
	ws, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	timeline, err := s.repo.ListTimeline(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	persons, err := s.repo.ListPersons(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	evidence, err := s.repo.ListEvidence(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	contradictions, err := s.repo.ListContradictions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	gaps, err := s.repo.ListGaps(ctx, workspaceID, false)
	if err != nil {
		return nil, err
	}
	tasks, err := s.repo.ListTasks(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	open := []models.InvestigationTask{}
	for _, t := range tasks {
		if t.Status != "done" {
			open = append(open, t)
		}
	}

	return &models.WorkspaceBrief{
		Workspace:      *ws,
		Timeline:       timeline,
		Persons:        persons,
		Evidence:       evidence,
		Contradictions: contradictions,
		Gaps:           gaps,
		OpenTasks:      open,
		GeneratedAt:    time.Now(),
	}, nil
}
