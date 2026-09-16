# ML services

Python (FastAPI) services called by the Go API. Still here: `fir_classifier`,
`semantic_search`, `ocr`, `ocr_service`, `nlp_extractor`, `face_recognition`,
`vehicle_detection`, `video_analysis`.

## Retired on 16 September 2026

Three prototypes were removed under workstream A0 of the AI plan
(`docs/plans/ai-and-anchoring-plan.html`), along with their Go handlers, routes,
service methods, environment variables, compose services and Dockerfile targets.

| Service | Why it was retired | What replaces it |
|---------|--------------------|------------------|
| `transcription` | Sent audio to Google's speech API, breaking the rule that no audio, footage, statement or personal data leaves the jurisdiction. | Workstream A3, on-premises speech. Not built yet — there is no transcription endpoint in the meantime. |
| `crime_prediction` | Produced crime risk scores of the kind Phase 10 rules out. The platform scores places, never people. | Phase 10's transparent risk scoring and hotspot analysis. |
| `patrol_optimization` | Placed patrol centres by random sampling. | Phase 10's transparent scoring, plus road routing. |

The Go routes that fronted them are gone as well: `/api/v1/transcription/*`,
`/api/v1/ml/predictions` and `/api/v1/ml/hotspots`. The environment variables
`ML_TRANSCRIPTION_URL` and `ML_CRIME_PREDICTION_URL` are no longer read.
