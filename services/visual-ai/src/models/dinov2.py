"""
DINOv2 Encoder - Fast baseline visual encoder.

License: Apache 2.0 (commercially safe)
Source: https://github.com/facebookresearch/dinov2

DINOv2 excels at:
- Fast single-frame encoding (~50ms)
- High-quality visual embeddings
- Visual similarity comparison
- Always-on baseline for all comparisons
"""

import torch
import torch.nn as nn
import torch.nn.functional as F
from typing import List, Tuple, Optional
import logging

logger = logging.getLogger(__name__)


class DINOv2Encoder(nn.Module):
    """
    DINOv2 encoder for fast visual embeddings.

    License: Apache 2.0 (commercially usable)
    Source: https://github.com/facebookresearch/dinov2
    """

    MODEL_ID = "facebook/dinov2-giant"
    LICENSE = "Apache-2.0"

    def __init__(self, device: str = "cuda", model_size: str = "giant"):
        super().__init__()
        self.device = torch.device(device if torch.cuda.is_available() else "cpu")

        logger.info(f"Loading DINOv2-{model_size} from {self.MODEL_ID}")
        logger.info(f"License: {self.LICENSE}")

        # Load DINOv2 model
        model_name = f"dinov2_vit{model_size[0]}14"  # dinov2_vitg14
        try:
            self.model = torch.hub.load('facebookresearch/dinov2', model_name)
        except Exception as e:
            logger.warning(f"Failed to load {model_name}, trying base: {e}")
            self.model = torch.hub.load('facebookresearch/dinov2', 'dinov2_vitb14')

        self.model.to(self.device)
        self.model.eval()

        # Use FP16 for efficiency on GPU
        if self.device.type == "cuda":
            self.model = self.model.half()

        self.embed_dim = self.model.embed_dim
        logger.info(f"DINOv2 loaded: embed_dim={self.embed_dim}, device={self.device}")

    @torch.no_grad()
    def encode_single(self, frame: torch.Tensor) -> torch.Tensor:
        """
        Encode a single frame to embedding.

        Args:
            frame: Tensor of shape (3, H, W) or (1, 3, H, W)

        Returns:
            Normalized embedding of shape (1, embed_dim)
        """
        if frame.dim() == 3:
            frame = frame.unsqueeze(0)

        frame = frame.to(self.device)
        if self.device.type == "cuda":
            frame = frame.half()

        # Get CLS token embedding
        embedding = self.model(frame)

        # Normalize
        return F.normalize(embedding, p=2, dim=-1)

    @torch.no_grad()
    def encode_frames(self, frames: List[torch.Tensor]) -> torch.Tensor:
        """Encode multiple frames efficiently"""
        # Stack and batch process
        batch = torch.stack(frames).to(self.device)
        if self.device.type == "cuda":
            batch = batch.half()

        embeddings = self.model(batch)
        return F.normalize(embeddings, p=2, dim=-1)

    @torch.no_grad()
    def compare(
        self,
        frame1: torch.Tensor,
        frame2: torch.Tensor,
        threshold: float = 0.85
    ) -> Tuple[float, bool, str]:
        """
        Compare two frames semantically.

        Returns:
            (similarity_score, is_similar, analysis)
        """
        emb1 = self.encode_single(frame1)
        emb2 = self.encode_single(frame2)

        similarity = F.cosine_similarity(emb1, emb2, dim=-1).item()
        is_similar = similarity >= threshold

        if similarity > 0.95:
            analysis = "Frames are nearly identical"
        elif similarity > 0.85:
            analysis = "Frames are semantically similar with minor differences"
        elif similarity > 0.70:
            analysis = "Frames have noticeable differences"
        else:
            analysis = "Frames are significantly different"

        return similarity, is_similar, analysis

    @torch.no_grad()
    def batch_compare(
        self,
        baselines: List[torch.Tensor],
        actuals: List[torch.Tensor],
        threshold: float = 0.85
    ) -> List[Tuple[str, float, bool]]:
        """
        Compare multiple frame pairs efficiently.

        Returns:
            List of (pair_id, similarity, is_similar)
        """
        baseline_embs = self.encode_frames(baselines)
        actual_embs = self.encode_frames(actuals)

        similarities = F.cosine_similarity(baseline_embs, actual_embs, dim=-1)

        results = []
        for i, sim in enumerate(similarities.tolist()):
            results.append((f"pair_{i}", sim, sim >= threshold))

        return results

    @torch.no_grad()
    def detect_changes(
        self,
        frame1: torch.Tensor,
        frame2: torch.Tensor,
        grid_size: int = 4
    ) -> List[Tuple[int, int, float]]:
        """
        Detect regions that changed between frames.

        Uses patch-level comparison to identify changed regions.

        Returns:
            List of (grid_x, grid_y, change_score)
        """
        # Get patch tokens (not just CLS)
        if frame1.dim() == 3:
            frame1 = frame1.unsqueeze(0)
        if frame2.dim() == 3:
            frame2 = frame2.unsqueeze(0)

        frame1 = frame1.to(self.device)
        frame2 = frame2.to(self.device)

        if self.device.type == "cuda":
            frame1 = frame1.half()
            frame2 = frame2.half()

        # Get intermediate features
        features1 = self.model.get_intermediate_layers(frame1, n=1)[0]
        features2 = self.model.get_intermediate_layers(frame2, n=1)[0]

        # Reshape to grid
        h = w = int((features1.shape[1]) ** 0.5)
        features1 = features1.reshape(1, h, w, -1)
        features2 = features2.reshape(1, h, w, -1)

        # Pool to grid_size x grid_size
        pool_h = h // grid_size
        pool_w = w // grid_size

        changes = []
        for i in range(grid_size):
            for j in range(grid_size):
                patch1 = features1[:, i*pool_h:(i+1)*pool_h, j*pool_w:(j+1)*pool_w, :].mean(dim=(1,2))
                patch2 = features2[:, i*pool_h:(i+1)*pool_h, j*pool_w:(j+1)*pool_w, :].mean(dim=(1,2))

                sim = F.cosine_similarity(patch1, patch2, dim=-1).item()
                change_score = 1.0 - sim
                changes.append((i, j, change_score))

        return changes

    def get_info(self) -> dict:
        """Get model info"""
        return {
            "model_id": self.MODEL_ID,
            "license": self.LICENSE,
            "embed_dim": self.embed_dim,
            "device": str(self.device),
        }
