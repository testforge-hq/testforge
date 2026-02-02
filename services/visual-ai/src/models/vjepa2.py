"""
V-JEPA 2 Encoder - Video/sequence understanding.

License: MIT + Apache 2.0 (commercially safe)
Source: https://github.com/facebookresearch/vjepa2

V-JEPA 2 excels at:
- Understanding temporal sequences (frame-to-frame transitions)
- Detecting UI state changes
- Semantic state equivalence (did we reach the same outcome?)
- Motion/activity detection (loading, animating, stable)
"""

import torch
import torch.nn as nn
import torch.nn.functional as F
from typing import List, Tuple, Optional
import logging

logger = logging.getLogger(__name__)


class VJEPA2Encoder(nn.Module):
    """
    V-JEPA 2 encoder for video/sequence understanding.

    License: MIT + Apache 2.0 (commercially usable)
    Source: https://github.com/facebookresearch/vjepa2

    V-JEPA 2 excels at:
    - Understanding temporal sequences (frame→frame transitions)
    - Detecting UI state changes
    - Semantic state equivalence (did we reach the same outcome?)
    - Motion/activity detection (loading, animating, stable)
    """

    MODEL_ID = "facebook/vjepa2-vitg-fpc64-384"
    LICENSE = "MIT + Apache-2.0"

    def __init__(self, device: str = "cuda"):
        super().__init__()
        self.device = torch.device(device if torch.cuda.is_available() else "cpu")

        logger.info(f"Loading V-JEPA 2 from {self.MODEL_ID}")
        logger.info(f"License: {self.LICENSE}")

        try:
            # Try to load V-JEPA 2 from HuggingFace
            from transformers import AutoModel, AutoProcessor
            self.processor = AutoProcessor.from_pretrained(self.MODEL_ID, trust_remote_code=True)
            self.model = AutoModel.from_pretrained(self.MODEL_ID, trust_remote_code=True)
            self.use_hf = True
            self.embed_dim = self.model.config.hidden_size
        except Exception as e:
            logger.warning(f"Could not load V-JEPA 2 from HuggingFace: {e}")
            logger.info("Falling back to DINOv2-based sequence encoder")
            # Fallback to DINOv2 with temporal modeling
            self.model = torch.hub.load('facebookresearch/dinov2', 'dinov2_vitg14')
            self.use_hf = False
            self.embed_dim = self.model.embed_dim
            # Add temporal projection
            self.temporal_proj = nn.Linear(self.embed_dim, self.embed_dim)

        self.model.to(self.device)
        self.model.eval()

        # Use FP16 for efficiency
        if self.device.type == "cuda":
            self.model = self.model.half()
            if hasattr(self, 'temporal_proj'):
                self.temporal_proj = self.temporal_proj.half().to(self.device)

        logger.info(f"V-JEPA 2 loaded: embed_dim={self.embed_dim}, device={self.device}, use_hf={self.use_hf}")

    @torch.no_grad()
    def encode_frames(self, frames: List[torch.Tensor]) -> torch.Tensor:
        """
        Encode a sequence of frames.

        V-JEPA 2 processes frames as a video, understanding temporal relationships.

        Args:
            frames: List of tensors, each (3, H, W)

        Returns:
            Sequence embedding (1, embed_dim) that captures temporal dynamics
        """
        if self.use_hf:
            # Stack frames into video tensor (1, T, C, H, W)
            video = torch.stack(frames).unsqueeze(0).to(self.device)
            if self.device.type == "cuda":
                video = video.half()

            # Process through V-JEPA 2
            outputs = self.model(video)

            # Get sequence-level embedding
            embedding = outputs.last_hidden_state.mean(dim=1)
        else:
            # Fallback: encode each frame with DINOv2 and model temporally
            batch = torch.stack(frames).to(self.device)
            if self.device.type == "cuda":
                batch = batch.half()

            frame_embeddings = self.model(batch)  # (T, embed_dim)

            # Simple temporal modeling: weighted average with recency bias
            weights = torch.linspace(0.5, 1.0, len(frames), device=self.device)
            weights = weights / weights.sum()
            embedding = (frame_embeddings * weights.unsqueeze(1)).sum(dim=0, keepdim=True)

            # Project through temporal layer
            embedding = self.temporal_proj(embedding)

        return F.normalize(embedding, p=2, dim=-1)

    @torch.no_grad()
    def encode_single(self, frame: torch.Tensor) -> torch.Tensor:
        """Encode single frame (treats as 1-frame video)"""
        return self.encode_frames([frame])

    @torch.no_grad()
    def compare_sequences(
        self,
        seq1: List[torch.Tensor],
        seq2: List[torch.Tensor]
    ) -> Tuple[float, str]:
        """
        Compare two frame sequences semantically.

        This is where V-JEPA 2 shines - it understands if two sequences
        represent the same semantic state transition, even if frames differ.

        Returns:
            (similarity_score, analysis_text)
        """
        emb1 = self.encode_frames(seq1)
        emb2 = self.encode_frames(seq2)

        similarity = F.cosine_similarity(emb1, emb2, dim=-1).item()

        # Generate analysis based on similarity
        if similarity > 0.95:
            analysis = "Sequences are semantically equivalent - same state transition"
        elif similarity > 0.85:
            analysis = "Sequences show similar transitions with minor variations"
        elif similarity > 0.70:
            analysis = "Sequences differ but may represent related states"
        else:
            analysis = "Sequences represent significantly different state transitions"

        return similarity, analysis

    @torch.no_grad()
    def detect_stability(
        self,
        frames: List[torch.Tensor],
        threshold: float = 0.98
    ) -> Tuple[bool, int, float, str]:
        """
        Detect when UI becomes stable.

        V-JEPA 2 understands motion/activity, so it can detect:
        - Loading spinners
        - Animations
        - Content settling
        - True stability

        Returns:
            (is_stable, stable_at_frame_index, stability_score, activity_description)
        """
        if len(frames) < 2:
            return True, 0, 1.0, "Single frame - assuming stable"

        # Encode each frame
        embeddings = []
        for f in frames:
            emb = self.encode_single(f)
            embeddings.append(emb)

        # Compare consecutive frames
        similarities = []
        for i in range(len(embeddings) - 1):
            sim = F.cosine_similarity(embeddings[i], embeddings[i+1], dim=-1).item()
            similarities.append(sim)

        # Find first stable point (consecutive high-similarity pairs)
        stable_count = 0
        stable_at = -1

        for i, sim in enumerate(similarities):
            if sim >= threshold:
                stable_count += 1
                if stable_count >= 2 and stable_at == -1:
                    stable_at = i
            else:
                stable_count = 0
                stable_at = -1

        is_stable = stable_count >= 2

        # Calculate overall stability score
        stability_score = sum(similarities) / len(similarities) if similarities else 1.0

        # Analyze activity type
        avg_motion = 1 - stability_score
        if avg_motion < 0.02:
            activity = "static"
        elif avg_motion < 0.10:
            activity = "minor_motion"
        elif avg_motion < 0.30:
            activity = "animation"
        else:
            activity = "significant_change"

        return is_stable, stable_at if stable_at >= 0 else len(frames)-1, stability_score, activity

    @torch.no_grad()
    def validate_healing(
        self,
        before_frames: List[torch.Tensor],
        after_frames: List[torch.Tensor],
        expected_frames: List[torch.Tensor],
        threshold: float = 0.85
    ) -> Tuple[bool, float, float, str]:
        """
        Validate that a self-healing fix produced the expected outcome.

        V-JEPA 2 compares:
        1. The transition that occurred (before → after)
        2. The expected outcome

        This is semantic - it understands "did we end up in the same state"
        even if pixels differ slightly.

        Returns:
            (is_valid, semantic_similarity, state_confidence, analysis)
        """
        # Encode the actual transition
        actual_transition = before_frames + after_frames
        actual_emb = self.encode_frames(actual_transition)

        # Encode the expected outcome
        expected_emb = self.encode_frames(expected_frames)

        # Also compare just the final states
        actual_final = self.encode_single(after_frames[-1])
        expected_final = self.encode_single(expected_frames[-1])

        # Combined similarity (transition + final state)
        transition_sim = F.cosine_similarity(actual_emb, expected_emb, dim=-1).item()
        final_sim = F.cosine_similarity(actual_final, expected_final, dim=-1).item()

        # Weight final state more heavily for healing validation
        combined_sim = 0.3 * transition_sim + 0.7 * final_sim

        is_valid = combined_sim >= threshold
        state_confidence = 0.9 if self.use_hf else 0.75  # Higher confidence with real V-JEPA 2

        if is_valid:
            analysis = f"Healing validated: final state matches expected (similarity: {combined_sim:.2%})"
        else:
            if final_sim < threshold:
                analysis = f"Healing failed: final state differs from expected (similarity: {final_sim:.2%})"
            else:
                analysis = f"Healing uncertain: transition differs but final state similar (transition: {transition_sim:.2%}, final: {final_sim:.2%})"

        return is_valid, combined_sim, state_confidence, analysis

    @torch.no_grad()
    def analyze_change(
        self,
        before: torch.Tensor,
        after: torch.Tensor,
        action: str = ""
    ) -> Tuple[str, List[str], bool, float]:
        """
        Analyze what changed between two frames.

        Returns:
            (description, changes_list, expected_change, confidence)
        """
        before_emb = self.encode_single(before)
        after_emb = self.encode_single(after)

        similarity = F.cosine_similarity(before_emb, after_emb, dim=-1).item()
        change_magnitude = 1 - similarity

        # Determine change type
        changes = []
        if change_magnitude < 0.02:
            description = "No significant visual change detected"
            change_type = "none"
        elif change_magnitude < 0.10:
            description = "Minor visual changes (possible style or minor content update)"
            changes = ["minor_style_change"]
            change_type = "cosmetic"
        elif change_magnitude < 0.30:
            description = "Moderate visual changes (content or layout modification)"
            changes = ["content_change", "possible_layout_change"]
            change_type = "content"
        else:
            description = "Major visual changes (significant content or page change)"
            changes = ["major_content_change", "possible_navigation"]
            change_type = "major"

        # Determine if change was expected based on action
        expected = True
        if action:
            action_lower = action.lower()
            if any(kw in action_lower for kw in ["click", "submit", "navigate", "type"]):
                expected = change_magnitude > 0.02  # Should see some change
            elif "hover" in action_lower:
                expected = change_magnitude < 0.30  # Should be minor change

        confidence = 0.85 if self.use_hf else 0.70

        return description, changes, expected, confidence

    def get_info(self) -> dict:
        """Get model info"""
        return {
            "model_id": self.MODEL_ID,
            "license": self.LICENSE,
            "embed_dim": self.embed_dim,
            "device": str(self.device),
            "use_hf": self.use_hf,
        }
