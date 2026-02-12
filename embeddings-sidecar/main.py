"""Local embedding sidecar for Alexandria using sentence-transformers."""

from fastapi import FastAPI
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer

app = FastAPI(title="Alexandria Embeddings Sidecar")
model = SentenceTransformer("all-MiniLM-L6-v2")


class EmbedRequest(BaseModel):
    texts: list[str]


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest) -> EmbedResponse:
    vectors = model.encode(req.texts, normalize_embeddings=True)
    return EmbedResponse(embeddings=vectors.tolist())


@app.get("/health")
def health():
    return {"status": "ok"}
