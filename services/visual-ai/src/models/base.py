"""
Base Encoder - Abstract base class for all visual encoders.
"""

from abc import ABC, abstractmethod
from typing import List, Tuple, Optional
import torch
import torch.nn.functional as F


class BaseEncoder(ABC):
    """Abstract base class for visual encoders"""

    MODEL_ID: str = "unknown"
    LICENSE: str = "unknown"

    def __init__(self, device: str = "cuda"):
        self.device = torch.device(device if torch.cuda.is_available() else "cpu")
        self.model = None
        self.embed_dim = 0

    @abstractmethod
    def encode_single(self, frame: torch.Tensor) -> torch.Tensor:
        """Encode a single frame to embedding"""
        pass

    def encode_frames(self, frames: List[torch.Tensor]) -> torch.Tensor:
        """Encode multiple frames - default is to encode each separately"""
        embeddings = [self.encode_single(f) for f in frames]
        return torch.stack(embeddings)

    def compare(
        self,
        frame1: torch.Tensor,
        frame2: torch.Tensor
    ) -> Tuple[float, bool, str]:
        """
        Compare two frames.

        Returns:
            (similarity_score, is_similar, analysis)
        """
        emb1 = self.encode_single(frame1)
        emb2 = self.encode_single(frame2)

        similarity = F.cosine_similarity(emb1, emb2, dim=-1).item()
        is_similar = similarity >= 0.85

        if similarity > 0.95:
            analysis = "Frames are nearly identical"
        elif similarity > 0.85:
            analysis = "Frames are semantically similar with minor differences"
        elif similarity > 0.70:
            analysis = "Frames have noticeable differences"
        else:
            analysis = "Frames are significantly different"

        return similarity, is_similar, analysis

    def batch_compare(
        self,
        pairs: List[Tuple[torch.Tensor, torch.Tensor]],
        threshold: float = 0.85
    ) -> List[Tuple[float, bool]]:
        """Compare multiple frame pairs"""
        results = []
        for f1, f2 in pairs:
            sim, is_sim, _ = self.compare(f1, f2)
            results.append((sim, is_sim))
        return results

    def get_memory_usage(self) -> int:
        """Get GPU memory usage in MB"""
        if torch.cuda.is_available():
            return torch.cuda.memory_allocated(self.device) // (1024 * 1024)
        return 0

    def get_info(self) -> dict:
        """Get model info"""
        return {
            "model_id": self.MODEL_ID,
            "license": self.LICENSE,
            "embed_dim": self.embed_dim,
            "device": str(self.device),
            "memory_mb": self.get_memory_usage(),
        }
