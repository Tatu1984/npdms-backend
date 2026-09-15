package models

// DispatchPolicy is the stated configuration dispatch runs on. It is served
// as-is at GET /dispatch/policy so the thresholds on screen are the thresholds
// applied. Changing a value here changes behaviour and the screen together.

type EscalationStep struct {
	Level        int    `json:"level"`
	AfterMinutes int    `json:"afterMinutes"`
	Action       string `json:"action"`
}

type SeverityDefinition struct {
	Severity IncidentSeverity `json:"severity"`
	Meaning  string           `json:"meaning"`
	// Minutes from acknowledgement to arrival before the supervisor is alerted.
	OnSceneMinutes int `json:"onSceneMinutes"`
}

type DispatchPolicyDoc struct {
	AcknowledgementLadder []EscalationStep     `json:"acknowledgementLadder"`
	Severities            []SeverityDefinition `json:"severities"`
	Recommendation        string               `json:"recommendation"`
	Notification          string               `json:"notification"`
}

// AcknowledgementLadder: measured from the moment a unit is assigned, while it
// has not acknowledged. Each step is recorded once per incident.
var AcknowledgementLadder = []EscalationStep{
	{Level: 1, AfterMinutes: 2, Action: "Reminder to the assigned unit"},
	{Level: 2, AfterMinutes: 5, Action: "Duty supervisor at the incident's station"},
	{Level: 3, AfterMinutes: 8, Action: "Divisional control room"},
}

// SeverityScale is the four-level scale the operator chooses from.
var SeverityScale = []SeverityDefinition{
	{Severity: SeverityCritical, Meaning: "Threat to life in progress, or a serious violent crime happening now", OnSceneMinutes: 10},
	{Severity: SeverityHigh, Meaning: "Crime in progress or just occurred with suspects nearby, injury, or risk of escalation", OnSceneMinutes: 15},
	{Severity: SeverityMedium, Meaning: "Offence reported after the fact, disturbance, or traffic obstruction without injury", OnSceneMinutes: 20},
	{Severity: SeverityLow, Meaning: "Nuisance, information or assistance request with no immediate risk", OnSceneMinutes: 30},
}

// OnSceneMinutes returns the arrival threshold for a severity.
func OnSceneMinutes(s IncidentSeverity) int {
	for _, d := range SeverityScale {
		if d.Severity == s {
			return d.OnSceneMinutes
		}
	}
	return SeverityScale[len(SeverityScale)-1].OnSceneMinutes
}

func DispatchPolicy() DispatchPolicyDoc {
	return DispatchPolicyDoc{
		AcknowledgementLadder: AcknowledgementLadder,
		Severities:            SeverityScale,
		Recommendation: "Available units are ranked by straight-line distance between the incident and the unit's " +
			"last known position — the vehicle's GPS fix, or its station when there is none. This is not a route or " +
			"an arrival time. Units without a position are listed after, unranked. The operator chooses.",
		Notification: "Escalations are recorded on the incident and shown on the dispatch board. They are not " +
			"delivered by SMS, radio or push: no notification channel is integrated.",
	}
}
