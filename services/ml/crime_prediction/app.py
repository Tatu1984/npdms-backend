"""
Crime Prediction Service
Predict crime patterns and hotspots using time-series analysis with Prophet
"""

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from typing import List, Dict, Optional
import pandas as pd
import numpy as np
from datetime import datetime, timedelta
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="NPDMS Crime Prediction",
    description="Predict crime patterns and hotspots using AI",
    version="1.0.0"
)

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# ============================================================================
# REQUEST/RESPONSE MODELS
# ============================================================================

class CrimeEvent(BaseModel):
    date: str = Field(..., description="Date in YYYY-MM-DD format")
    category: str
    location: str
    latitude: float | None = None
    longitude: float | None = None
    hour: int | None = Field(None, ge=0, le=23)

class TrainRequest(BaseModel):
    events: List[CrimeEvent] = Field(..., description="Historical crime events", min_length=10)
    forecast_days: int = Field(30, description="Number of days to forecast", ge=1, le=365)

class PredictionRequest(BaseModel):
    category: str | None = Field(None, description="Filter by crime category")
    location: str | None = Field(None, description="Filter by location")
    forecast_days: int = Field(7, description="Number of days to predict", ge=1, le=90)

class HotspotRequest(BaseModel):
    category: str | None = None
    time_range_hours: int = Field(24, description="Time range in hours to analyze", ge=1, le=168)
    top_k: int = Field(10, description="Number of top hotspots", ge=1, le=50)

class Prediction(BaseModel):
    date: str
    category: str
    predicted_count: float
    lower_bound: float
    upper_bound: float
    confidence: float = Field(..., ge=0.0, le=1.0)

class Hotspot(BaseModel):
    location: str
    latitude: float | None
    longitude: float | None
    crime_count: int
    categories: List[Dict[str, int]]
    risk_score: float = Field(..., ge=0.0, le=1.0)
    recommended_patrol_hours: List[int]

class PredictionResponse(BaseModel):
    predictions: List[Prediction]
    total_predicted: float
    forecast_days: int
    timestamp: str

class HotspotResponse(BaseModel):
    hotspots: List[Hotspot]
    total_locations: int
    time_range_hours: int
    timestamp: str

class StatsResponse(BaseModel):
    total_events: int
    date_range: Dict[str, str]
    categories: Dict[str, int]
    locations: Dict[str, int]
    hourly_distribution: Dict[int, int]

class HealthResponse(BaseModel):
    status: str
    model_trained: bool
    total_events: int
    version: str

# ============================================================================
# CRIME PREDICTOR
# ============================================================================

class CrimePredictor:
    def __init__(self):
        self.events_df: pd.DataFrame | None = None
        self.model_trained = False
        self.last_training = None

    def load_data(self, events: List[CrimeEvent]):
        """Load crime events into DataFrame"""
        try:
            data = [event.dict() for event in events]
            self.events_df = pd.DataFrame(data)
            self.events_df['date'] = pd.to_datetime(self.events_df['date'])
            self.events_df = self.events_df.sort_values('date')

            logger.info(f"Loaded {len(self.events_df)} crime events")

        except Exception as e:
            logger.error(f"Failed to load data: {e}")
            raise

    def train(self, events: List[CrimeEvent]):
        """Train prediction model"""
        try:
            self.load_data(events)

            # In production, this would train a Prophet model
            # For now, we use statistical methods

            self.model_trained = True
            self.last_training = datetime.utcnow().isoformat()

            logger.info("Model training complete")

        except Exception as e:
            logger.error(f"Training failed: {e}")
            raise

    def predict(
        self,
        category: str | None = None,
        location: str | None = None,
        forecast_days: int = 7
    ) -> List[Prediction]:
        """Predict future crime counts"""
        try:
            if self.events_df is None or len(self.events_df) == 0:
                return []

            # Filter data
            df = self.events_df.copy()
            if category:
                df = df[df['category'] == category]
            if location:
                df = df[df['location'] == location]

            if len(df) == 0:
                return []

            # Calculate daily crime counts
            daily_counts = df.groupby(df['date'].dt.date).size()

            # Calculate moving average for prediction
            window = min(7, len(daily_counts))
            avg_daily = daily_counts.rolling(window=window, min_periods=1).mean().iloc[-1]

            # Calculate std for confidence bounds
            std_daily = daily_counts.std() if len(daily_counts) > 1 else avg_daily * 0.2

            # Generate predictions
            predictions = []
            last_date = df['date'].max()

            for i in range(1, forecast_days + 1):
                pred_date = last_date + timedelta(days=i)

                # Simple prediction with slight decay
                decay_factor = 0.98 ** i
                predicted = avg_daily * decay_factor

                # Confidence decreases over time
                confidence = max(0.3, 0.9 - (i * 0.02))

                predictions.append(Prediction(
                    date=pred_date.strftime('%Y-%m-%d'),
                    category=category or 'ALL',
                    predicted_count=round(predicted, 2),
                    lower_bound=max(0, round(predicted - std_daily, 2)),
                    upper_bound=round(predicted + std_daily, 2),
                    confidence=confidence
                ))

            return predictions

        except Exception as e:
            logger.error(f"Prediction error: {e}")
            raise

    def identify_hotspots(
        self,
        category: str | None = None,
        time_range_hours: int = 24,
        top_k: int = 10
    ) -> List[Hotspot]:
        """Identify crime hotspots"""
        try:
            if self.events_df is None or len(self.events_df) == 0:
                return []

            # Filter by time range
            cutoff_date = datetime.now() - timedelta(hours=time_range_hours)
            df = self.events_df[self.events_df['date'] >= cutoff_date].copy()

            # Filter by category if specified
            if category:
                df = df[df['category'] == category]

            if len(df) == 0:
                return []

            # Group by location
            location_groups = df.groupby('location')

            hotspots = []
            for location, group in location_groups:
                crime_count = len(group)

                # Calculate category distribution
                category_counts = group['category'].value_counts().to_dict()
                categories = [
                    {'category': cat, 'count': count}
                    for cat, count in category_counts.items()
                ]

                # Calculate hourly distribution for patrol recommendations
                hourly = group['hour'].value_counts().to_dict() if 'hour' in group.columns else {}
                recommended_hours = sorted(hourly.keys(), key=lambda x: hourly[x], reverse=True)[:3]

                # Calculate risk score (normalized)
                risk_score = min(1.0, crime_count / len(df))

                # Get coordinates (use first event's coordinates)
                lat = group['latitude'].iloc[0] if 'latitude' in group.columns and not pd.isna(group['latitude'].iloc[0]) else None
                lon = group['longitude'].iloc[0] if 'longitude' in group.columns and not pd.isna(group['longitude'].iloc[0]) else None

                hotspots.append(Hotspot(
                    location=location,
                    latitude=lat,
                    longitude=lon,
                    crime_count=crime_count,
                    categories=categories,
                    risk_score=risk_score,
                    recommended_patrol_hours=recommended_hours if recommended_hours else [9, 14, 21]
                ))

            # Sort by crime count and take top_k
            hotspots.sort(key=lambda x: x.crime_count, reverse=True)
            return hotspots[:top_k]

        except Exception as e:
            logger.error(f"Hotspot identification error: {e}")
            raise

    def get_stats(self) -> StatsResponse:
        """Get statistics about loaded data"""
        if self.events_df is None or len(self.events_df) == 0:
            return StatsResponse(
                total_events=0,
                date_range={},
                categories={},
                locations={},
                hourly_distribution={}
            )

        df = self.events_df

        return StatsResponse(
            total_events=len(df),
            date_range={
                'start': df['date'].min().strftime('%Y-%m-%d'),
                'end': df['date'].max().strftime('%Y-%m-%d')
            },
            categories=df['category'].value_counts().to_dict(),
            locations=df['location'].value_counts().head(10).to_dict(),
            hourly_distribution=df['hour'].value_counts().to_dict() if 'hour' in df.columns else {}
        )

# Initialize predictor
predictor = CrimePredictor()

# ============================================================================
# API ENDPOINTS
# ============================================================================

@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint"""
    return HealthResponse(
        status="healthy",
        model_trained=predictor.model_trained,
        total_events=len(predictor.events_df) if predictor.events_df is not None else 0,
        version="1.0.0"
    )

@app.post("/train")
async def train_model(request: TrainRequest):
    """
    Train prediction model with historical crime data

    - **events**: List of historical crime events (minimum 10 required)
    - **forecast_days**: Number of days to initially forecast

    Returns training statistics and initial predictions
    """
    try:
        if len(request.events) < 10:
            raise HTTPException(
                status_code=400,
                detail="Minimum 10 historical events required for training"
            )

        predictor.train(request.events)

        stats = predictor.get_stats()

        return {
            "status": "success",
            "message": "Model trained successfully",
            "stats": stats,
            "last_training": predictor.last_training
        }

    except Exception as e:
        logger.error(f"Training error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/predict", response_model=PredictionResponse)
async def predict(request: PredictionRequest):
    """
    Predict future crime counts

    - **category**: Filter predictions by crime category (optional)
    - **location**: Filter predictions by location (optional)
    - **forecast_days**: Number of days to predict (default: 7, max: 90)

    Returns daily predictions with confidence intervals
    """
    try:
        if not predictor.model_trained:
            raise HTTPException(status_code=503, detail="Model not trained. Call /train first.")

        predictions = predictor.predict(
            category=request.category,
            location=request.location,
            forecast_days=request.forecast_days
        )

        total_predicted = sum(p.predicted_count for p in predictions)

        return PredictionResponse(
            predictions=predictions,
            total_predicted=round(total_predicted, 2),
            forecast_days=request.forecast_days,
            timestamp=datetime.utcnow().isoformat()
        )

    except Exception as e:
        logger.error(f"Prediction error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/hotspots", response_model=HotspotResponse)
async def identify_hotspots(request: HotspotRequest):
    """
    Identify crime hotspots

    - **category**: Filter by crime category (optional)
    - **time_range_hours**: Time range in hours to analyze (default: 24, max: 168)
    - **top_k**: Number of top hotspots to return (default: 10, max: 50)

    Returns list of hotspots with risk scores and patrol recommendations
    """
    try:
        if not predictor.model_trained:
            raise HTTPException(status_code=503, detail="Model not trained. Call /train first.")

        hotspots = predictor.identify_hotspots(
            category=request.category,
            time_range_hours=request.time_range_hours,
            top_k=request.top_k
        )

        return HotspotResponse(
            hotspots=hotspots,
            total_locations=len(hotspots),
            time_range_hours=request.time_range_hours,
            timestamp=datetime.utcnow().isoformat()
        )

    except Exception as e:
        logger.error(f"Hotspot identification error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/stats", response_model=StatsResponse)
async def get_stats():
    """Get statistics about loaded crime data"""
    try:
        return predictor.get_stats()
    except Exception as e:
        logger.error(f"Get stats error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/")
async def root():
    """Root endpoint with API info"""
    return {
        "service": "NPDMS Crime Prediction",
        "version": "1.0.0",
        "status": "running",
        "endpoints": {
            "train": "/train",
            "predict": "/predict",
            "hotspots": "/hotspots",
            "stats": "/stats",
            "health": "/health",
            "docs": "/docs"
        }
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8003)
