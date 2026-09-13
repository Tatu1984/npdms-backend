"""
Semantic Search Service
Find similar FIRs using sentence embeddings and FAISS vector search
"""

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from typing import List, Optional, Dict
import numpy as np
import faiss
from sentence_transformers import SentenceTransformer
import logging
import pickle
import os
from datetime import datetime

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="NPDMS Semantic Search",
    description="Find similar FIRs using AI-powered semantic search",
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
# CONFIGURATION
# ============================================================================

# Model configuration
EMBEDDING_MODEL = "sentence-transformers/all-MiniLM-L6-v2"  # Fast, lightweight model
EMBEDDING_DIM = 384  # Dimension of embeddings
INDEX_PATH = "/data/faiss_index.bin"
METADATA_PATH = "/data/fir_metadata.pkl"

# ============================================================================
# REQUEST/RESPONSE MODELS
# ============================================================================

class SearchRequest(BaseModel):
    query: str = Field(..., description="Search query or FIR description", min_length=5)
    top_k: int = Field(5, description="Number of results to return", ge=1, le=50)
    threshold: float = Field(0.5, description="Minimum similarity threshold", ge=0.0, le=1.0)
    filters: Dict[str, str] | None = Field(None, description="Filters (status, category, etc.)")

class FIRDocument(BaseModel):
    fir_id: str
    fir_number: str
    title: str
    description: str
    category: str
    status: str
    registered_at: str
    metadata: Dict | None = None

class AddDocumentRequest(BaseModel):
    fir_id: str
    fir_number: str
    title: str
    description: str
    category: str
    status: str
    registered_at: str
    metadata: Dict | None = None

class SearchResult(BaseModel):
    fir: FIRDocument
    similarity_score: float = Field(..., ge=0.0, le=1.0, description="Cosine similarity score")
    rank: int = Field(..., ge=1, description="Result rank")

class SearchResponse(BaseModel):
    query: str
    results: List[SearchResult]
    total_results: int
    search_time_ms: float
    timestamp: str

class IndexStats(BaseModel):
    total_documents: int
    index_size_mb: float
    embedding_dim: int
    model: str
    last_updated: str | None

class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
    index_loaded: bool
    version: str

# ============================================================================
# SEMANTIC SEARCH ENGINE
# ============================================================================

class SemanticSearchEngine:
    def __init__(self):
        self.model = None
        self.index = None
        self.documents: List[FIRDocument] = []
        self.model_loaded = False
        self.index_loaded = False
        self.last_updated = None

    def load_model(self):
        """Load sentence transformer model"""
        try:
            logger.info(f"Loading embedding model: {EMBEDDING_MODEL}")
            self.model = SentenceTransformer(EMBEDDING_MODEL)
            self.model_loaded = True
            logger.info("Model loaded successfully")
        except Exception as e:
            logger.error(f"Failed to load model: {e}")
            raise

    def load_index(self):
        """Load FAISS index and metadata from disk"""
        try:
            if os.path.exists(INDEX_PATH) and os.path.exists(METADATA_PATH):
                logger.info("Loading existing FAISS index...")
                self.index = faiss.read_index(INDEX_PATH)

                with open(METADATA_PATH, 'rb') as f:
                    data = pickle.load(f)
                    self.documents = data['documents']
                    self.last_updated = data.get('last_updated')

                self.index_loaded = True
                logger.info(f"Loaded index with {len(self.documents)} documents")
            else:
                logger.info("No existing index found, creating new index...")
                self.create_index()
        except Exception as e:
            logger.error(f"Failed to load index: {e}")
            self.create_index()

    def create_index(self):
        """Create new FAISS index"""
        try:
            # Create L2 index (can be changed to Inner Product for cosine similarity)
            self.index = faiss.IndexFlatL2(EMBEDDING_DIM)
            self.documents = []
            self.index_loaded = True
            self.last_updated = datetime.utcnow().isoformat()
            logger.info("Created new FAISS index")
        except Exception as e:
            logger.error(f"Failed to create index: {e}")
            raise

    def save_index(self):
        """Save FAISS index and metadata to disk"""
        try:
            os.makedirs(os.path.dirname(INDEX_PATH), exist_ok=True)

            faiss.write_index(self.index, INDEX_PATH)

            with open(METADATA_PATH, 'wb') as f:
                pickle.dump({
                    'documents': self.documents,
                    'last_updated': datetime.utcnow().isoformat()
                }, f)

            logger.info(f"Saved index with {len(self.documents)} documents")
        except Exception as e:
            logger.error(f"Failed to save index: {e}")
            raise

    def add_document(self, doc: FIRDocument):
        """Add document to index"""
        try:
            # Create text from title and description
            text = f"{doc.title} {doc.description}"

            # Generate embedding
            embedding = self.model.encode([text])[0]

            # Add to FAISS index
            self.index.add(np.array([embedding], dtype=np.float32))

            # Store metadata
            self.documents.append(doc)

            logger.info(f"Added document: {doc.fir_number}")

        except Exception as e:
            logger.error(f"Failed to add document: {e}")
            raise

    def search(
        self,
        query: str,
        top_k: int = 5,
        threshold: float = 0.5,
        filters: Dict | None = None
    ) -> List[SearchResult]:
        """Search for similar documents"""
        try:
            start_time = datetime.utcnow()

            # Generate query embedding
            query_embedding = self.model.encode([query])[0]

            # Search in FAISS index
            # Retrieve more results if filters are applied
            search_k = top_k * 3 if filters else top_k
            distances, indices = self.index.search(
                np.array([query_embedding], dtype=np.float32),
                search_k
            )

            # Convert L2 distances to similarity scores (0-1)
            # Similarity = 1 / (1 + distance)
            similarities = 1 / (1 + distances[0])

            # Build results
            results = []
            for idx, (doc_idx, similarity) in enumerate(zip(indices[0], similarities)):
                if similarity < threshold:
                    continue

                if doc_idx >= len(self.documents):
                    continue

                doc = self.documents[doc_idx]

                # Apply filters
                if filters:
                    if filters.get('status') and doc.status != filters['status']:
                        continue
                    if filters.get('category') and doc.category != filters['category']:
                        continue

                results.append(SearchResult(
                    fir=doc,
                    similarity_score=float(similarity),
                    rank=len(results) + 1
                ))

                if len(results) >= top_k:
                    break

            end_time = datetime.utcnow()
            search_time_ms = (end_time - start_time).total_seconds() * 1000

            logger.info(f"Search completed in {search_time_ms:.2f}ms, found {len(results)} results")

            return results, search_time_ms

        except Exception as e:
            logger.error(f"Search error: {e}")
            raise

    def get_stats(self) -> IndexStats:
        """Get index statistics"""
        index_size = 0
        if os.path.exists(INDEX_PATH):
            index_size = os.path.getsize(INDEX_PATH) / (1024 * 1024)  # MB

        return IndexStats(
            total_documents=len(self.documents),
            index_size_mb=round(index_size, 2),
            embedding_dim=EMBEDDING_DIM,
            model=EMBEDDING_MODEL,
            last_updated=self.last_updated
        )

# Initialize search engine
search_engine = SemanticSearchEngine()

# ============================================================================
# API ENDPOINTS
# ============================================================================

@app.on_event("startup")
async def startup_event():
    """Load model and index on startup"""
    logger.info("Starting Semantic Search Service...")
    try:
        search_engine.load_model()
        search_engine.load_index()
        logger.info("Startup complete")
    except Exception as e:
        logger.error(f"Startup failed: {e}")

@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint"""
    return HealthResponse(
        status="healthy" if (search_engine.model_loaded and search_engine.index_loaded) else "unhealthy",
        model_loaded=search_engine.model_loaded,
        index_loaded=search_engine.index_loaded,
        version="1.0.0"
    )

@app.post("/search", response_model=SearchResponse)
async def search(request: SearchRequest):
    """
    Search for similar FIRs

    - **query**: Search query or FIR description
    - **top_k**: Number of results to return (default: 5, max: 50)
    - **threshold**: Minimum similarity threshold (default: 0.5)
    - **filters**: Optional filters (status, category, etc.)

    Returns:
    - **results**: List of similar FIRs with similarity scores
    - **total_results**: Total number of results found
    - **search_time_ms**: Search time in milliseconds
    """
    try:
        if not search_engine.model_loaded or not search_engine.index_loaded:
            raise HTTPException(status_code=503, detail="Search engine not ready")

        if search_engine.index.ntotal == 0:
            return SearchResponse(
                query=request.query,
                results=[],
                total_results=0,
                search_time_ms=0.0,
                timestamp=datetime.utcnow().isoformat()
            )

        results, search_time = search_engine.search(
            request.query,
            request.top_k,
            request.threshold,
            request.filters
        )

        return SearchResponse(
            query=request.query,
            results=results,
            total_results=len(results),
            search_time_ms=search_time,
            timestamp=datetime.utcnow().isoformat()
        )

    except Exception as e:
        logger.error(f"Search error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/index/add")
async def add_document(doc: AddDocumentRequest):
    """
    Add a new FIR document to the search index

    - **fir_id**: Unique FIR ID
    - **fir_number**: FIR number
    - **title**: FIR title
    - **description**: FIR description
    - **category**: Crime category
    - **status**: FIR status
    - **registered_at**: Registration timestamp
    - **metadata**: Optional metadata dictionary
    """
    try:
        if not search_engine.model_loaded or not search_engine.index_loaded:
            raise HTTPException(status_code=503, detail="Search engine not ready")

        fir_doc = FIRDocument(**doc.dict())
        search_engine.add_document(fir_doc)

        # Periodically save index (every 10 documents)
        if len(search_engine.documents) % 10 == 0:
            search_engine.save_index()

        return {
            "status": "success",
            "fir_id": doc.fir_id,
            "total_documents": len(search_engine.documents)
        }

    except Exception as e:
        logger.error(f"Add document error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/index/batch-add")
async def batch_add_documents(docs: List[AddDocumentRequest]):
    """
    Add multiple FIR documents to the search index in batch

    Useful for initial index population or bulk updates
    """
    try:
        if not search_engine.model_loaded or not search_engine.index_loaded:
            raise HTTPException(status_code=503, detail="Search engine not ready")

        for doc in docs:
            fir_doc = FIRDocument(**doc.dict())
            search_engine.add_document(fir_doc)

        search_engine.save_index()

        logger.info(f"Batch added {len(docs)} documents")

        return {
            "status": "success",
            "added_count": len(docs),
            "total_documents": len(search_engine.documents)
        }

    except Exception as e:
        logger.error(f"Batch add error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/index/rebuild")
async def rebuild_index():
    """
    Rebuild the search index from scratch

    Use with caution - this will clear the existing index
    """
    try:
        logger.info("Rebuilding index...")
        search_engine.create_index()
        search_engine.save_index()

        return {
            "status": "success",
            "message": "Index rebuilt successfully",
            "total_documents": 0
        }

    except Exception as e:
        logger.error(f"Rebuild index error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/index/stats", response_model=IndexStats)
async def get_index_stats():
    """Get search index statistics"""
    try:
        return search_engine.get_stats()
    except Exception as e:
        logger.error(f"Get stats error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/")
async def root():
    """Root endpoint with API info"""
    return {
        "service": "NPDMS Semantic Search",
        "version": "1.0.0",
        "status": "running",
        "endpoints": {
            "search": "/search",
            "add_document": "/index/add",
            "batch_add": "/index/batch-add",
            "rebuild": "/index/rebuild",
            "stats": "/index/stats",
            "health": "/health",
            "docs": "/docs"
        }
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8002)
