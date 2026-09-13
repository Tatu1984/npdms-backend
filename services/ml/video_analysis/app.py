"""
Video Analysis Service for NPDMS
Implements: Key frame extraction, object detection, video summarization
"""

import os
import cv2
import numpy as np
from typing import List, Dict, Optional, Tuple
from dataclasses import dataclass, asdict
from datetime import timedelta
import hashlib
import json
from pathlib import Path

from fastapi import FastAPI, UploadFile, File, HTTPException, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import uvicorn

# Try to import ML libraries
try:
    from ultralytics import YOLO
    YOLO_AVAILABLE = True
except ImportError:
    YOLO_AVAILABLE = False
    print("Warning: YOLO not available. Using mock detection.")

app = FastAPI(
    title="NPDMS Video Analysis Service",
    description="AI-powered video evidence analysis with key frame extraction",
    version="1.0.0"
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Configuration
MODEL_PATH = os.getenv("YOLO_MODEL_PATH", "yolov8n.pt")
UPLOAD_DIR = Path(os.getenv("UPLOAD_DIR", "/tmp/video_analysis"))
UPLOAD_DIR.mkdir(parents=True, exist_ok=True)

# Detection classes of interest for police work
PRIORITY_CLASSES = {
    "person": {"priority": "HIGH", "category": "PERSON"},
    "car": {"priority": "MEDIUM", "category": "VEHICLE"},
    "truck": {"priority": "MEDIUM", "category": "VEHICLE"},
    "motorcycle": {"priority": "MEDIUM", "category": "VEHICLE"},
    "bicycle": {"priority": "LOW", "category": "VEHICLE"},
    "knife": {"priority": "CRITICAL", "category": "WEAPON"},
    "cell phone": {"priority": "LOW", "category": "ELECTRONICS"},
    "laptop": {"priority": "LOW", "category": "ELECTRONICS"},
    "backpack": {"priority": "LOW", "category": "OBJECT"},
    "handbag": {"priority": "LOW", "category": "OBJECT"},
    "suitcase": {"priority": "LOW", "category": "OBJECT"},
}


@dataclass
class Detection:
    """Represents a detected object in a frame"""
    class_name: str
    confidence: float
    bbox: Tuple[int, int, int, int]  # x1, y1, x2, y2
    category: str
    priority: str


@dataclass
class KeyFrame:
    """Represents a key frame extracted from video"""
    frame_number: int
    timestamp: float  # seconds
    timestamp_formatted: str
    reason: str
    detections: List[Detection]
    scene_score: float
    motion_score: float
    thumbnail_path: Optional[str] = None


@dataclass
class VideoAnalysisResult:
    """Complete analysis result for a video"""
    video_id: str
    duration: float
    fps: float
    total_frames: int
    frames_analyzed: int
    key_frames: List[KeyFrame]
    unique_persons_estimate: int
    vehicles_detected: List[Dict]
    weapons_detected: bool
    summary: str
    timeline: List[Dict]
    disclaimer: str = "AI analysis. Manual review of full footage required for court."


class AnalysisRequest(BaseModel):
    """Request model for video analysis"""
    sample_rate: int = 2  # Frames per second to analyze
    min_confidence: float = 0.5
    extract_keyframes: bool = True
    detect_weapons: bool = True
    detect_persons: bool = True
    detect_vehicles: bool = True


class VideoAnalyzer:
    """Main video analysis class"""

    def __init__(self):
        self.model = None
        if YOLO_AVAILABLE:
            try:
                self.model = YOLO(MODEL_PATH)
            except Exception as e:
                print(f"Failed to load YOLO model: {e}")

    def analyze_video(
        self,
        video_path: str,
        config: AnalysisRequest
    ) -> VideoAnalysisResult:
        """Analyze a video file and extract key frames"""

        cap = cv2.VideoCapture(video_path)
        if not cap.isOpened():
            raise ValueError("Could not open video file")

        # Get video properties
        fps = cap.get(cv2.CAP_PROP_FPS)
        total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
        duration = total_frames / fps if fps > 0 else 0

        # Calculate sample interval
        sample_interval = max(1, int(fps / config.sample_rate))

        key_frames = []
        all_detections = []
        prev_frame = None
        frame_idx = 0
        frames_analyzed = 0

        # Generate video ID
        video_id = hashlib.md5(video_path.encode()).hexdigest()[:12]

        while cap.isOpened():
            ret, frame = cap.read()
            if not ret:
                break

            if frame_idx % sample_interval == 0:
                frames_analyzed += 1

                # Detect objects
                detections = self._detect_objects(frame, config.min_confidence)
                all_detections.extend(detections)

                # Calculate motion score
                motion_score = self._calculate_motion(prev_frame, frame) if prev_frame is not None else 0

                # Determine if this is a key frame
                is_keyframe, reason = self._is_keyframe(
                    detections, all_detections, motion_score, prev_frame is None
                )

                if is_keyframe and config.extract_keyframes:
                    timestamp = frame_idx / fps
                    keyframe = KeyFrame(
                        frame_number=frame_idx,
                        timestamp=timestamp,
                        timestamp_formatted=str(timedelta(seconds=int(timestamp))),
                        reason=reason,
                        detections=detections,
                        scene_score=self._calculate_scene_score(detections),
                        motion_score=motion_score,
                        thumbnail_path=self._save_thumbnail(frame, video_id, frame_idx)
                    )
                    key_frames.append(keyframe)

                prev_frame = frame.copy()

            frame_idx += 1

        cap.release()

        # Generate summary and statistics
        unique_persons = self._estimate_unique_persons(all_detections)
        vehicles = self._extract_vehicles(all_detections)
        weapons_detected = any(d.category == "WEAPON" for d in all_detections)
        timeline = self._generate_timeline(key_frames)
        summary = self._generate_summary(key_frames, all_detections, duration)

        return VideoAnalysisResult(
            video_id=video_id,
            duration=duration,
            fps=fps,
            total_frames=total_frames,
            frames_analyzed=frames_analyzed,
            key_frames=key_frames,
            unique_persons_estimate=unique_persons,
            vehicles_detected=vehicles,
            weapons_detected=weapons_detected,
            summary=summary,
            timeline=timeline
        )

    def _detect_objects(self, frame: np.ndarray, min_confidence: float) -> List[Detection]:
        """Detect objects in a frame using YOLO"""
        detections = []

        if self.model is not None:
            results = self.model(frame, verbose=False)[0]

            for box in results.boxes:
                class_id = int(box.cls[0])
                class_name = results.names[class_id]
                confidence = float(box.conf[0])

                if confidence >= min_confidence:
                    x1, y1, x2, y2 = map(int, box.xyxy[0])
                    priority_info = PRIORITY_CLASSES.get(
                        class_name,
                        {"priority": "LOW", "category": "OTHER"}
                    )

                    detections.append(Detection(
                        class_name=class_name,
                        confidence=confidence,
                        bbox=(x1, y1, x2, y2),
                        category=priority_info["category"],
                        priority=priority_info["priority"]
                    ))
        else:
            # Mock detection for testing
            if np.random.random() > 0.7:
                detections.append(Detection(
                    class_name="person",
                    confidence=0.85,
                    bbox=(100, 100, 200, 300),
                    category="PERSON",
                    priority="HIGH"
                ))

        return detections

    def _calculate_motion(self, prev_frame: np.ndarray, curr_frame: np.ndarray) -> float:
        """Calculate motion score between two frames"""
        if prev_frame is None:
            return 0.0

        # Convert to grayscale
        prev_gray = cv2.cvtColor(prev_frame, cv2.COLOR_BGR2GRAY)
        curr_gray = cv2.cvtColor(curr_frame, cv2.COLOR_BGR2GRAY)

        # Calculate absolute difference
        diff = cv2.absdiff(prev_gray, curr_gray)
        motion_score = np.mean(diff) / 255.0

        return float(motion_score)

    def _is_keyframe(
        self,
        current_detections: List[Detection],
        all_detections: List[Detection],
        motion_score: float,
        is_first: bool
    ) -> Tuple[bool, str]:
        """Determine if a frame is a key frame"""

        if is_first:
            return True, "First frame"

        # High motion indicates scene change
        if motion_score > 0.15:
            return True, "Significant motion detected"

        # New person appeared
        person_count = sum(1 for d in current_detections if d.category == "PERSON")
        if person_count > 0:
            prev_person_count = sum(1 for d in all_detections[-20:] if d.category == "PERSON")
            if person_count > prev_person_count:
                return True, "New person appeared"

        # Weapon detected
        if any(d.category == "WEAPON" for d in current_detections):
            return True, "Weapon detected"

        # High priority object detected
        if any(d.priority == "CRITICAL" for d in current_detections):
            return True, "Critical object detected"

        # Vehicle appeared
        vehicle_count = sum(1 for d in current_detections if d.category == "VEHICLE")
        if vehicle_count > 0:
            return True, "Vehicle in frame"

        return False, ""

    def _calculate_scene_score(self, detections: List[Detection]) -> float:
        """Calculate importance score for a scene"""
        score = 0.0

        priority_weights = {"CRITICAL": 1.0, "HIGH": 0.7, "MEDIUM": 0.4, "LOW": 0.1}

        for d in detections:
            score += priority_weights.get(d.priority, 0.1) * d.confidence

        return min(score, 1.0)

    def _save_thumbnail(self, frame: np.ndarray, video_id: str, frame_idx: int) -> str:
        """Save a thumbnail of the frame"""
        thumbnail_dir = UPLOAD_DIR / video_id
        thumbnail_dir.mkdir(parents=True, exist_ok=True)

        # Resize for thumbnail
        thumbnail = cv2.resize(frame, (320, 180))
        path = thumbnail_dir / f"frame_{frame_idx}.jpg"
        cv2.imwrite(str(path), thumbnail)

        return str(path)

    def _estimate_unique_persons(self, detections: List[Detection]) -> int:
        """Estimate number of unique persons (simplified)"""
        person_detections = [d for d in detections if d.category == "PERSON"]
        # This is a simplified estimate - real implementation would use re-identification
        return max(1, len(person_detections) // 10) if person_detections else 0

    def _extract_vehicles(self, detections: List[Detection]) -> List[Dict]:
        """Extract vehicle information"""
        vehicles = []
        seen_types = set()

        for d in detections:
            if d.category == "VEHICLE" and d.class_name not in seen_types:
                vehicles.append({
                    "type": d.class_name,
                    "confidence": d.confidence,
                    "first_seen": True
                })
                seen_types.add(d.class_name)

        return vehicles

    def _generate_timeline(self, key_frames: List[KeyFrame]) -> List[Dict]:
        """Generate a timeline of events"""
        timeline = []

        for kf in key_frames:
            timeline.append({
                "timestamp": kf.timestamp_formatted,
                "timestamp_seconds": kf.timestamp,
                "event": kf.reason,
                "objects_detected": [d.class_name for d in kf.detections],
                "importance": kf.scene_score
            })

        return timeline

    def _generate_summary(
        self,
        key_frames: List[KeyFrame],
        all_detections: List[Detection],
        duration: float
    ) -> str:
        """Generate a text summary of the video"""
        summary_parts = []

        # Duration
        duration_str = str(timedelta(seconds=int(duration)))
        summary_parts.append(f"Video duration: {duration_str}")

        # Key frames
        summary_parts.append(f"Key frames extracted: {len(key_frames)}")

        # Persons
        person_count = sum(1 for d in all_detections if d.category == "PERSON")
        if person_count > 0:
            summary_parts.append(f"Person detections: {person_count}")

        # Vehicles
        vehicle_count = sum(1 for d in all_detections if d.category == "VEHICLE")
        if vehicle_count > 0:
            summary_parts.append(f"Vehicle detections: {vehicle_count}")

        # Weapons
        weapon_count = sum(1 for d in all_detections if d.category == "WEAPON")
        if weapon_count > 0:
            summary_parts.append(f"⚠️ WEAPON DETECTED: {weapon_count} occurrences")

        return ". ".join(summary_parts)


# Initialize analyzer
analyzer = VideoAnalyzer()


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "model_loaded": analyzer.model is not None,
        "yolo_available": YOLO_AVAILABLE
    }


@app.post("/analyze")
async def analyze_video(
    file: UploadFile = File(...),
    sample_rate: int = 2,
    min_confidence: float = 0.5,
    extract_keyframes: bool = True
):
    """Analyze an uploaded video file"""

    # Validate file type
    if not file.filename.lower().endswith(('.mp4', '.avi', '.mov', '.mkv')):
        raise HTTPException(status_code=400, detail="Unsupported video format")

    # Save uploaded file
    video_path = UPLOAD_DIR / file.filename
    with open(video_path, "wb") as f:
        content = await file.read()
        f.write(content)

    try:
        # Create analysis config
        config = AnalysisRequest(
            sample_rate=sample_rate,
            min_confidence=min_confidence,
            extract_keyframes=extract_keyframes
        )

        # Analyze video
        result = analyzer.analyze_video(str(video_path), config)

        # Convert to dict for JSON response
        return {
            "success": True,
            "result": {
                "video_id": result.video_id,
                "duration": result.duration,
                "fps": result.fps,
                "total_frames": result.total_frames,
                "frames_analyzed": result.frames_analyzed,
                "key_frames_count": len(result.key_frames),
                "key_frames": [
                    {
                        "frame_number": kf.frame_number,
                        "timestamp": kf.timestamp_formatted,
                        "reason": kf.reason,
                        "detections": [asdict(d) for d in kf.detections],
                        "scene_score": kf.scene_score,
                        "motion_score": kf.motion_score,
                        "thumbnail_path": kf.thumbnail_path
                    }
                    for kf in result.key_frames
                ],
                "unique_persons_estimate": result.unique_persons_estimate,
                "vehicles_detected": result.vehicles_detected,
                "weapons_detected": result.weapons_detected,
                "summary": result.summary,
                "timeline": result.timeline,
                "disclaimer": result.disclaimer
            }
        }

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

    finally:
        # Cleanup uploaded file
        if video_path.exists():
            video_path.unlink()


@app.post("/extract-frames")
async def extract_frames(
    file: UploadFile = File(...),
    interval_seconds: float = 1.0
):
    """Extract frames at regular intervals"""

    video_path = UPLOAD_DIR / file.filename
    with open(video_path, "wb") as f:
        content = await file.read()
        f.write(content)

    try:
        cap = cv2.VideoCapture(str(video_path))
        fps = cap.get(cv2.CAP_PROP_FPS)
        interval_frames = int(fps * interval_seconds)

        frames = []
        frame_idx = 0
        video_id = hashlib.md5(file.filename.encode()).hexdigest()[:12]

        while cap.isOpened():
            ret, frame = cap.read()
            if not ret:
                break

            if frame_idx % interval_frames == 0:
                timestamp = frame_idx / fps
                path = analyzer._save_thumbnail(frame, video_id, frame_idx)
                frames.append({
                    "frame_number": frame_idx,
                    "timestamp": str(timedelta(seconds=int(timestamp))),
                    "path": path
                })

            frame_idx += 1

        cap.release()

        return {
            "success": True,
            "video_id": video_id,
            "frames_extracted": len(frames),
            "frames": frames
        }

    finally:
        if video_path.exists():
            video_path.unlink()


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8006)
