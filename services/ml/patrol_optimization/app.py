"""
Patrol Optimization Service for NPDMS
Implements: Crime hotspot prediction, patrol route optimization, resource allocation
"""

import os
import math
from typing import List, Dict, Optional, Tuple
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta
import json
import random

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
import uvicorn

# Try to import optimization libraries
try:
    import numpy as np
    from scipy.spatial.distance import cdist
    from scipy.optimize import linear_sum_assignment
    SCIPY_AVAILABLE = True
except ImportError:
    SCIPY_AVAILABLE = False
    print("Warning: scipy not available. Using simplified optimization.")

try:
    from ortools.constraint_solver import routing_enums_pb2
    from ortools.constraint_solver import pywrapcp
    ORTOOLS_AVAILABLE = True
except ImportError:
    ORTOOLS_AVAILABLE = False
    print("Warning: OR-Tools not available. Using greedy routing.")

app = FastAPI(
    title="NPDMS Patrol Optimization Service",
    description="AI-powered patrol route optimization and resource allocation",
    version="1.0.0"
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# Data models
class Location(BaseModel):
    """Geographic location"""
    latitude: float
    longitude: float
    name: Optional[str] = None


class Hotspot(BaseModel):
    """Crime hotspot"""
    id: str
    location: Location
    risk_score: float = Field(ge=0, le=1)
    crime_types: List[str] = []
    time_window: Optional[str] = None
    priority: str = "MEDIUM"


class PatrolUnit(BaseModel):
    """Available patrol unit"""
    id: str
    current_location: Location
    capacity: int = 2  # Number of officers
    vehicle_type: str = "CAR"
    shift_end_time: Optional[str] = None
    skills: List[str] = []


class OptimizationRequest(BaseModel):
    """Request for patrol optimization"""
    hotspots: List[Hotspot]
    patrol_units: List[PatrolUnit]
    shift_duration_hours: float = 8
    coverage_radius_km: float = 0.5
    min_visits_per_hotspot: int = 1
    optimize_for: str = "COVERAGE"  # COVERAGE, RESPONSE_TIME, BALANCED


class PatrolRoute(BaseModel):
    """Optimized route for a patrol unit"""
    patrol_unit_id: str
    waypoints: List[Dict]
    total_distance_km: float
    estimated_duration_minutes: float
    hotspots_covered: List[str]
    coverage_score: float


class OptimizationResult(BaseModel):
    """Complete optimization result"""
    routes: List[PatrolRoute]
    total_coverage_score: float
    uncovered_hotspots: List[str]
    estimated_response_time_improvement: float
    recommendations: List[str]
    disclaimer: str = "AI-suggested routes. SHO has final authority on deployment."


@dataclass
class GridCell:
    """Grid cell for spatial analysis"""
    id: str
    center_lat: float
    center_lng: float
    risk_score: float
    historical_incidents: int
    contributing_factors: List[str]


class PatrolOptimizer:
    """Main patrol optimization class"""

    def __init__(self):
        self.earth_radius_km = 6371

    def optimize(self, request: OptimizationRequest) -> OptimizationResult:
        """Optimize patrol routes"""

        if len(request.patrol_units) == 0:
            raise ValueError("No patrol units available")

        if len(request.hotspots) == 0:
            return OptimizationResult(
                routes=[],
                total_coverage_score=0,
                uncovered_hotspots=[],
                estimated_response_time_improvement=0,
                recommendations=["No hotspots identified for this period"]
            )

        # Calculate distance matrix
        distance_matrix = self._calculate_distance_matrix(
            request.hotspots, request.patrol_units
        )

        # Optimize based on strategy
        if request.optimize_for == "COVERAGE":
            routes = self._optimize_for_coverage(
                request.hotspots, request.patrol_units, distance_matrix, request
            )
        elif request.optimize_for == "RESPONSE_TIME":
            routes = self._optimize_for_response_time(
                request.hotspots, request.patrol_units, distance_matrix, request
            )
        else:
            routes = self._optimize_balanced(
                request.hotspots, request.patrol_units, distance_matrix, request
            )

        # Calculate coverage
        covered_hotspots = set()
        for route in routes:
            covered_hotspots.update(route.hotspots_covered)

        uncovered = [h.id for h in request.hotspots if h.id not in covered_hotspots]
        total_coverage = len(covered_hotspots) / len(request.hotspots) if request.hotspots else 0

        # Generate recommendations
        recommendations = self._generate_recommendations(
            request.hotspots, routes, uncovered
        )

        return OptimizationResult(
            routes=routes,
            total_coverage_score=total_coverage,
            uncovered_hotspots=uncovered,
            estimated_response_time_improvement=self._estimate_improvement(routes),
            recommendations=recommendations
        )

    def _calculate_distance_matrix(
        self,
        hotspots: List[Hotspot],
        patrol_units: List[PatrolUnit]
    ) -> List[List[float]]:
        """Calculate distances between all locations"""

        all_locations = (
            [(h.location.latitude, h.location.longitude) for h in hotspots] +
            [(p.current_location.latitude, p.current_location.longitude) for p in patrol_units]
        )

        n = len(all_locations)
        matrix = [[0.0] * n for _ in range(n)]

        for i in range(n):
            for j in range(n):
                if i != j:
                    matrix[i][j] = self._haversine_distance(
                        all_locations[i][0], all_locations[i][1],
                        all_locations[j][0], all_locations[j][1]
                    )

        return matrix

    def _haversine_distance(
        self, lat1: float, lon1: float, lat2: float, lon2: float
    ) -> float:
        """Calculate distance between two points in km"""

        lat1_rad = math.radians(lat1)
        lat2_rad = math.radians(lat2)
        delta_lat = math.radians(lat2 - lat1)
        delta_lon = math.radians(lon2 - lon1)

        a = (math.sin(delta_lat / 2) ** 2 +
             math.cos(lat1_rad) * math.cos(lat2_rad) * math.sin(delta_lon / 2) ** 2)
        c = 2 * math.atan2(math.sqrt(a), math.sqrt(1 - a))

        return self.earth_radius_km * c

    def _optimize_for_coverage(
        self,
        hotspots: List[Hotspot],
        patrol_units: List[PatrolUnit],
        distance_matrix: List[List[float]],
        request: OptimizationRequest
    ) -> List[PatrolRoute]:
        """Optimize routes for maximum coverage"""

        routes = []
        assigned_hotspots = set()

        # Sort hotspots by risk score (highest first)
        sorted_hotspots = sorted(hotspots, key=lambda h: h.risk_score, reverse=True)

        for unit in patrol_units:
            route_waypoints = []
            route_hotspots = []
            total_distance = 0
            current_lat = unit.current_location.latitude
            current_lng = unit.current_location.longitude

            # Greedy assignment of nearby high-risk hotspots
            available = [h for h in sorted_hotspots if h.id not in assigned_hotspots]

            while available and len(route_hotspots) < 10:  # Max 10 hotspots per route
                # Find nearest available hotspot
                nearest = min(
                    available,
                    key=lambda h: self._haversine_distance(
                        current_lat, current_lng,
                        h.location.latitude, h.location.longitude
                    )
                )

                distance = self._haversine_distance(
                    current_lat, current_lng,
                    nearest.location.latitude, nearest.location.longitude
                )

                # Check if within coverage radius and time constraints
                if distance < request.coverage_radius_km * 5:  # Allow some travel
                    route_waypoints.append({
                        "hotspot_id": nearest.id,
                        "location": {
                            "latitude": nearest.location.latitude,
                            "longitude": nearest.location.longitude,
                            "name": nearest.location.name
                        },
                        "risk_score": nearest.risk_score,
                        "priority": nearest.priority,
                        "estimated_arrival_minutes": (distance / 30) * 60,  # Assume 30 km/h
                        "recommended_patrol_duration_minutes": 15 if nearest.priority == "HIGH" else 10
                    })
                    route_hotspots.append(nearest.id)
                    assigned_hotspots.add(nearest.id)
                    total_distance += distance
                    current_lat = nearest.location.latitude
                    current_lng = nearest.location.longitude
                    available.remove(nearest)
                else:
                    break

            if route_waypoints:
                routes.append(PatrolRoute(
                    patrol_unit_id=unit.id,
                    waypoints=route_waypoints,
                    total_distance_km=round(total_distance, 2),
                    estimated_duration_minutes=round((total_distance / 30) * 60 + len(route_waypoints) * 12, 0),
                    hotspots_covered=route_hotspots,
                    coverage_score=sum(1 for h in hotspots if h.id in route_hotspots) / len(hotspots)
                ))

        return routes

    def _optimize_for_response_time(
        self,
        hotspots: List[Hotspot],
        patrol_units: List[PatrolUnit],
        distance_matrix: List[List[float]],
        request: OptimizationRequest
    ) -> List[PatrolRoute]:
        """Optimize for minimum response time to hotspots"""

        # Position units at strategic locations to minimize max distance to any hotspot
        routes = []

        # Cluster hotspots and assign patrol units
        clusters = self._cluster_hotspots(hotspots, len(patrol_units))

        for i, (unit, cluster) in enumerate(zip(patrol_units, clusters)):
            if not cluster:
                continue

            # Find centroid of cluster
            centroid_lat = sum(h.location.latitude for h in cluster) / len(cluster)
            centroid_lng = sum(h.location.longitude for h in cluster) / len(cluster)

            # Create patrol route through cluster
            route_waypoints = []
            total_distance = 0

            # Add route to centroid first
            route_waypoints.append({
                "hotspot_id": "CENTROID",
                "location": {
                    "latitude": centroid_lat,
                    "longitude": centroid_lng,
                    "name": f"Cluster {i + 1} Center"
                },
                "risk_score": sum(h.risk_score for h in cluster) / len(cluster),
                "priority": "HIGH",
                "estimated_arrival_minutes": 5,
                "recommended_patrol_duration_minutes": 20
            })

            # Add hotspots in cluster
            for h in sorted(cluster, key=lambda x: x.risk_score, reverse=True)[:5]:
                route_waypoints.append({
                    "hotspot_id": h.id,
                    "location": {
                        "latitude": h.location.latitude,
                        "longitude": h.location.longitude,
                        "name": h.location.name
                    },
                    "risk_score": h.risk_score,
                    "priority": h.priority,
                    "estimated_arrival_minutes": 3,
                    "recommended_patrol_duration_minutes": 10
                })

            routes.append(PatrolRoute(
                patrol_unit_id=unit.id,
                waypoints=route_waypoints,
                total_distance_km=self._calculate_route_distance(route_waypoints),
                estimated_duration_minutes=len(route_waypoints) * 15,
                hotspots_covered=[h.id for h in cluster],
                coverage_score=len(cluster) / len(hotspots) if hotspots else 0
            ))

        return routes

    def _optimize_balanced(
        self,
        hotspots: List[Hotspot],
        patrol_units: List[PatrolUnit],
        distance_matrix: List[List[float]],
        request: OptimizationRequest
    ) -> List[PatrolRoute]:
        """Balanced optimization considering both coverage and response time"""

        # Combine coverage and response time strategies
        coverage_routes = self._optimize_for_coverage(
            hotspots, patrol_units, distance_matrix, request
        )

        # Adjust routes based on response time considerations
        for route in coverage_routes:
            # Re-order waypoints to minimize backtracking
            if len(route.waypoints) > 2:
                route.waypoints = self._optimize_waypoint_order(route.waypoints)

        return coverage_routes

    def _cluster_hotspots(
        self, hotspots: List[Hotspot], k: int
    ) -> List[List[Hotspot]]:
        """Simple k-means clustering of hotspots"""

        if len(hotspots) <= k:
            return [[h] for h in hotspots]

        # Initialize cluster centers
        centers = random.sample(hotspots, k)
        clusters = [[] for _ in range(k)]

        for _ in range(10):  # 10 iterations
            # Assign hotspots to nearest center
            clusters = [[] for _ in range(k)]
            for h in hotspots:
                nearest_idx = min(
                    range(k),
                    key=lambda i: self._haversine_distance(
                        h.location.latitude, h.location.longitude,
                        centers[i].location.latitude, centers[i].location.longitude
                    )
                )
                clusters[nearest_idx].append(h)

            # Update centers (use hotspot with highest risk in cluster)
            for i, cluster in enumerate(clusters):
                if cluster:
                    centers[i] = max(cluster, key=lambda h: h.risk_score)

        return clusters

    def _calculate_route_distance(self, waypoints: List[Dict]) -> float:
        """Calculate total route distance"""
        total = 0
        for i in range(len(waypoints) - 1):
            loc1 = waypoints[i]["location"]
            loc2 = waypoints[i + 1]["location"]
            total += self._haversine_distance(
                loc1["latitude"], loc1["longitude"],
                loc2["latitude"], loc2["longitude"]
            )
        return round(total, 2)

    def _optimize_waypoint_order(self, waypoints: List[Dict]) -> List[Dict]:
        """Optimize order of waypoints using nearest neighbor"""
        if len(waypoints) <= 2:
            return waypoints

        ordered = [waypoints[0]]
        remaining = waypoints[1:]

        while remaining:
            current = ordered[-1]["location"]
            nearest = min(
                remaining,
                key=lambda w: self._haversine_distance(
                    current["latitude"], current["longitude"],
                    w["location"]["latitude"], w["location"]["longitude"]
                )
            )
            ordered.append(nearest)
            remaining.remove(nearest)

        return ordered

    def _estimate_improvement(self, routes: List[PatrolRoute]) -> float:
        """Estimate response time improvement percentage"""
        if not routes:
            return 0

        # Simplified estimation
        avg_coverage = sum(r.coverage_score for r in routes) / len(routes)
        return min(30, avg_coverage * 40)  # Up to 30% improvement

    def _generate_recommendations(
        self,
        hotspots: List[Hotspot],
        routes: List[PatrolRoute],
        uncovered: List[str]
    ) -> List[str]:
        """Generate recommendations for patrol optimization"""

        recommendations = []

        # Coverage recommendations
        coverage_pct = 100 - (len(uncovered) / len(hotspots) * 100) if hotspots else 100
        if coverage_pct < 80:
            recommendations.append(
                f"Current coverage is {coverage_pct:.0f}%. Consider adding more patrol units."
            )

        # High-risk area recommendations
        high_risk = [h for h in hotspots if h.risk_score > 0.7]
        if high_risk:
            recommendations.append(
                f"{len(high_risk)} high-risk areas identified. Prioritize these locations."
            )

        # Uncovered area recommendations
        if uncovered:
            recommendations.append(
                f"{len(uncovered)} hotspots not covered. Consider mobile units or foot patrol."
            )

        # Efficiency recommendations
        if routes:
            avg_distance = sum(r.total_distance_km for r in routes) / len(routes)
            if avg_distance > 20:
                recommendations.append(
                    "Routes are long. Consider splitting patrol areas for better coverage."
                )

        if not recommendations:
            recommendations.append("Patrol coverage is optimal for current risk assessment.")

        return recommendations


# Hotspot Prediction
class HotspotPredictor:
    """Predicts crime hotspots based on historical data"""

    def predict_hotspots(
        self,
        historical_data: List[Dict],
        prediction_date: datetime,
        grid_size_km: float = 0.5
    ) -> List[Hotspot]:
        """Predict hotspots for a given date"""

        # This is a simplified prediction model
        # Real implementation would use Prophet, LSTM, or gradient boosting

        hotspots = []

        # Group by location grid
        grid_counts = {}
        for incident in historical_data:
            lat = incident.get("latitude", 0)
            lng = incident.get("longitude", 0)
            grid_key = (round(lat, 3), round(lng, 3))

            if grid_key not in grid_counts:
                grid_counts[grid_key] = {
                    "count": 0,
                    "types": [],
                    "lat": lat,
                    "lng": lng
                }
            grid_counts[grid_key]["count"] += 1
            grid_counts[grid_key]["types"].append(incident.get("type", "UNKNOWN"))

        # Convert to hotspots
        if grid_counts:
            max_count = max(g["count"] for g in grid_counts.values())

            for i, ((lat, lng), data) in enumerate(grid_counts.items()):
                risk_score = data["count"] / max_count
                if risk_score >= 0.3:  # Only include significant hotspots
                    hotspots.append(Hotspot(
                        id=f"HS-{i:04d}",
                        location=Location(latitude=data["lat"], longitude=data["lng"]),
                        risk_score=risk_score,
                        crime_types=list(set(data["types"])),
                        priority="CRITICAL" if risk_score > 0.8 else "HIGH" if risk_score > 0.5 else "MEDIUM"
                    ))

        return hotspots


# Initialize services
optimizer = PatrolOptimizer()
predictor = HotspotPredictor()


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "scipy_available": SCIPY_AVAILABLE,
        "ortools_available": ORTOOLS_AVAILABLE
    }


@app.post("/optimize", response_model=OptimizationResult)
async def optimize_patrol(request: OptimizationRequest):
    """Optimize patrol routes"""
    try:
        result = optimizer.optimize(request)
        return result
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/predict-hotspots")
async def predict_hotspots(
    historical_incidents: List[Dict],
    prediction_date: str = None
):
    """Predict crime hotspots from historical data"""
    try:
        pred_date = datetime.fromisoformat(prediction_date) if prediction_date else datetime.now()
        hotspots = predictor.predict_hotspots(historical_incidents, pred_date)
        return {
            "success": True,
            "prediction_date": pred_date.isoformat(),
            "hotspots_count": len(hotspots),
            "hotspots": [
                {
                    "id": h.id,
                    "location": {"latitude": h.location.latitude, "longitude": h.location.longitude},
                    "risk_score": h.risk_score,
                    "crime_types": h.crime_types,
                    "priority": h.priority
                }
                for h in hotspots
            ]
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/resource-allocation")
async def allocate_resources(
    hotspots: List[Hotspot],
    available_units: int,
    shift_hours: float = 8
):
    """Suggest resource allocation based on hotspots"""

    if not hotspots:
        return {
            "success": True,
            "allocation": [],
            "recommendations": ["No hotspots identified"]
        }

    # Calculate resource needs
    high_risk = [h for h in hotspots if h.risk_score > 0.7]
    medium_risk = [h for h in hotspots if 0.4 <= h.risk_score <= 0.7]
    low_risk = [h for h in hotspots if h.risk_score < 0.4]

    # Allocation strategy
    high_risk_units = min(len(high_risk), int(available_units * 0.5))
    medium_risk_units = min(len(medium_risk), int(available_units * 0.3))
    patrol_units = available_units - high_risk_units - medium_risk_units

    allocation = []

    for i, h in enumerate(high_risk[:high_risk_units]):
        allocation.append({
            "hotspot_id": h.id,
            "units_assigned": 1,
            "patrol_type": "STATIC",
            "recommended_duration_hours": shift_hours,
            "priority": "CRITICAL"
        })

    for i, h in enumerate(medium_risk[:medium_risk_units]):
        allocation.append({
            "hotspot_id": h.id,
            "units_assigned": 1,
            "patrol_type": "MOBILE",
            "recommended_duration_hours": shift_hours / 2,
            "priority": "HIGH"
        })

    return {
        "success": True,
        "total_units": available_units,
        "allocated_to_hotspots": high_risk_units + medium_risk_units,
        "mobile_patrol_units": patrol_units,
        "allocation": allocation,
        "recommendations": [
            f"Deploy {high_risk_units} units to high-risk areas",
            f"Deploy {medium_risk_units} units to medium-risk areas",
            f"Keep {patrol_units} units for mobile patrol and response"
        ]
    }


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8007)
