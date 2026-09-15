package models

import "time"

// Phase 08 — Station Workload.
//
// Every figure carries the definition of what it counts. These are operational
// aggregates over stored records: nothing is estimated, scored or forecast, and
// officer figures describe assigned load, never performance.

// WorkloadScope is the set of stations a figure covers.
type WorkloadScope struct {
	Level      string     `json:"level"` // station | district | all
	Label      string     `json:"label"`
	StationIDs []string   `json:"stationIds"`
	From       *time.Time `json:"from"`
	To         *time.Time `json:"to"`
}

type WorkloadMetric struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Count      int64  `json:"count"`
	Definition string `json:"definition"`
	Href       string `json:"href"`
	// attention marks a count that represents something past a stated date.
	Attention bool `json:"attention"`
}

type WorkloadSummary struct {
	Scope       WorkloadScope    `json:"scope"`
	Metrics     []WorkloadMetric `json:"metrics"`
	GeneratedAt time.Time        `json:"generatedAt"`
}

type AgeBands struct {
	Days0to30   int64 `json:"days0to30"`
	Days31to90  int64 `json:"days31to90"`
	Days91to180 int64 `json:"days91to180"`
	Days181Plus int64 `json:"days181plus"`
	Total       int64 `json:"total"`
}

type BacklogStage struct {
	Stage      string   `json:"stage"`
	Label      string   `json:"label"`
	Definition string   `json:"definition"`
	AgeFrom    string   `json:"ageFrom"`
	Href       string   `json:"href"`
	Bands      AgeBands `json:"bands"`
}

type WorkloadBacklog struct {
	Scope      WorkloadScope  `json:"scope"`
	Stages     []BacklogStage `json:"stages"`
	Totals     AgeBands       `json:"totals"`
	Bottleneck *string        `json:"bottleneck"`
	Rule       string         `json:"rule"`
	Reason     string         `json:"reason"`
}

type StationWorkload struct {
	StationID          string `json:"stationId"`
	Code               string `json:"code"`
	Name               string `json:"name"`
	District           string `json:"district"`
	OpenInvestigations int64  `json:"openInvestigations"`
	PendingForensics   int64  `json:"pendingForensics"`
	InCourt            int64  `json:"inCourt"`
	OpenTasks          int64  `json:"openTasks"`
	Over90Days         int64  `json:"over90Days"`
	RosterStrength     int64  `json:"rosterStrength"`
	Available          int64  `json:"available"`
	// Open investigations divided by available officers; null when none are available.
	PerAvailableOfficer *float64 `json:"perAvailableOfficer"`
	Backlog             AgeBands `json:"backlog"`
}

type OfficerWorkload struct {
	UserID           string `json:"userId"`
	Name             string `json:"name"`
	Rank             string `json:"rank"`
	Badge            string `json:"badge"`
	StationCode      string `json:"stationCode"`
	FIRsAsIO         int64  `json:"firsAsIO"`
	CasesAsIO        int64  `json:"casesAsIO"`
	CasesInCourt     int64  `json:"casesInCourt"`
	WorkspacesAsIO   int64  `json:"workspacesAsIO"`
	OpenTasks        int64  `json:"openTasks"`
	OverdueTasks     int64  `json:"overdueTasks"`
	PendingForensics int64  `json:"pendingForensics"`
	HearingsNext14   int64  `json:"hearingsNext14Days"`
}

type SLAItem struct {
	Module   string    `json:"module"`
	ID       string    `json:"id"`
	Ref      string    `json:"ref"`
	Title    string    `json:"title"`
	Since    time.Time `json:"since"`
	DaysOver int64     `json:"daysOver"`
	Band     string    `json:"band"`
	Href     string    `json:"href"`
}

type SLARule struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Rule     string    `json:"rule"`
	Source   string    `json:"source"`
	Limits   string    `json:"limits"`
	Breaches int64     `json:"breaches"`
	Items    []SLAItem `json:"items"`
}

type WorkloadSLA struct {
	Scope WorkloadScope `json:"scope"`
	Rules []SLARule     `json:"rules"`
}

type TrendSeries struct {
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	Definition string  `json:"definition"`
	Values     []int64 `json:"values"`
}

type WorkloadTrends struct {
	Scope    WorkloadScope `json:"scope"`
	Interval string        `json:"interval"`
	Buckets  []time.Time   `json:"buckets"`
	Series   []TrendSeries `json:"series"`
	NotShown []string      `json:"notShown"`
}

type WorkloadScopeOption struct {
	StationID string `json:"stationId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	District  string `json:"district"`
}

type WorkloadScopes struct {
	CanCompare bool                  `json:"canCompare"`
	OwnStation *string               `json:"ownStation"`
	Stations   []WorkloadScopeOption `json:"stations"`
	Districts  []string              `json:"districts"`
}
