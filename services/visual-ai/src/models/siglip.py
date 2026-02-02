"""
SigLIP Encoder - Text-to-image search and semantic matching.

License: Apache 2.0 (commercially safe)
Source: https://huggingface.co/google/siglip-large-patch16-384

SigLIP excels at:
- Finding elements by text description ("blue submit button")
- Semantic matching between text and images
- Zero-shot element localization
"""

import torch
import torch.nn as nn
import torch.nn.functional as F
from typing import List, Tuple, Optional
import logging

logger = logging.getLogger(__name__)


class SigLIPEncoder(nn.Module):
    """
    SigLIP encoder for text-to-image search.

    License: Apache 2.0 (commercially usable)
    Source: https://huggingface.co/google/siglip-large-patch16-384

    SigLIP excels at:
    - Finding elements by text description
    - Semantic text-image matching
    - Zero-shot visual search
    """

    MODEL_ID = "google/siglip-large-patch16-384"
    LICENSE = "Apache-2.0"

    def __init__(self, device: str = "cuda"):
        super().__init__()
        self.device = torch.device(device if torch.cuda.is_available() else "cpu")

        logger.info(f"Loading SigLIP from {self.MODEL_ID}")
        logger.info(f"License: {self.LICENSE}")

        try:
            from transformers import AutoProcessor, AutoModel

            self.processor = AutoProcessor.from_pretrained(self.MODEL_ID)
            self.model = AutoModel.from_pretrained(self.MODEL_ID)
            self.use_hf = True
        except Exception as e:
            logger.warning(f"Could not load SigLIP from HuggingFace: {e}")
            logger.info("Falling back to CLIP")
            try:
                import clip
                self.model, self.preprocess = clip.load("ViT-L/14@336px", device=self.device)
                self.use_hf = False
                self.processor = None
            except Exception as e2:
                logger.error(f"Could not load CLIP either: {e2}")
                raise RuntimeError("No text-image model available")

        self.model.to(self.device)
        self.model.eval()

        # Use FP16 for efficiency
        if self.device.type == "cuda" and self.use_hf:
            self.model = self.model.half()

        if self.use_hf:
            self.embed_dim = self.model.config.vision_config.hidden_size
        else:
            self.embed_dim = 768  # CLIP ViT-L

        logger.info(f"SigLIP loaded: embed_dim={self.embed_dim}, device={self.device}, use_hf={self.use_hf}")

    @torch.no_grad()
    def encode_image(self, image: torch.Tensor) -> torch.Tensor:
        """
        Encode an image to embedding.

        Args:
            image: Tensor of shape (3, H, W) or (1, 3, H, W)

        Returns:
            Normalized embedding
        """
        if image.dim() == 3:
            image = image.unsqueeze(0)

        image = image.to(self.device)
        if self.device.type == "cuda" and self.use_hf:
            image = image.half()

        if self.use_hf:
            outputs = self.model.get_image_features(pixel_values=image)
        else:
            outputs = self.model.encode_image(image)

        return F.normalize(outputs, p=2, dim=-1)

    @torch.no_grad()
    def encode_text(self, text: str) -> torch.Tensor:
        """
        Encode text to embedding.

        Args:
            text: Description string

        Returns:
            Normalized embedding
        """
        if self.use_hf:
            inputs = self.processor(text=[text], return_tensors="pt", padding=True)
            inputs = {k: v.to(self.device) for k, v in inputs.items()}
            outputs = self.model.get_text_features(**inputs)
        else:
            import clip
            text_tokens = clip.tokenize([text]).to(self.device)
            outputs = self.model.encode_text(text_tokens)

        return F.normalize(outputs, p=2, dim=-1)

    @torch.no_grad()
    def encode_single(self, frame: torch.Tensor) -> torch.Tensor:
        """Encode single frame (for compatibility with other encoders)"""
        return self.encode_image(frame)

    @torch.no_grad()
    def find_by_description(
        self,
        image: torch.Tensor,
        description: str,
        grid_size: int = 8,
        max_results: int = 5
    ) -> List[Tuple[int, int, int, int, float]]:
        """
        Find regions in image matching a text description.

        Uses sliding window approach with patch-level matching.

        Args:
            image: Full screenshot tensor (3, H, W)
            description: Text description of element to find
            grid_size: How fine to search
            max_results: Maximum regions to return

        Returns:
            List of (x, y, width, height, confidence) tuples
        """
        if image.dim() == 3:
            _, H, W = image.shape
        else:
            _, _, H, W = image.shape

        # Encode the text description
        text_emb = self.encode_text(description)

        # Create grid of patches
        patch_h = H // grid_size
        patch_w = W // grid_size

        results = []

        for i in range(grid_size):
            for j in range(grid_size):
                # Extract patch
                y_start = i * patch_h
                y_end = min((i + 1) * patch_h, H)
                x_start = j * patch_w
                x_end = min((j + 1) * patch_w, W)

                if image.dim() == 3:
                    patch = image[:, y_start:y_end, x_start:x_end]
                else:
                    patch = image[:, :, y_start:y_end, x_start:x_end]

                # Resize patch to model's expected size
                patch = F.interpolate(
                    patch.unsqueeze(0) if patch.dim() == 3 else patch,
                    size=(384, 384),
                    mode='bilinear',
                    align_corners=False
                )

                # Encode patch
                patch_emb = self.encode_image(patch)

                # Calculate similarity
                similarity = F.cosine_similarity(text_emb, patch_emb, dim=-1).item()

                results.append((
                    x_start,
                    y_start,
                    x_end - x_start,
                    y_end - y_start,
                    similarity
                ))

        # Sort by similarity and return top results
        results.sort(key=lambda x: x[4], reverse=True)
        return results[:max_results]

    @torch.no_grad()
    def match_text_to_regions(
        self,
        image: torch.Tensor,
        regions: List[Tuple[int, int, int, int]],
        description: str
    ) -> List[Tuple[int, float]]:
        """
        Match a text description to pre-defined regions.

        Args:
            image: Full screenshot tensor
            regions: List of (x, y, width, height) regions
            description: Text to match

        Returns:
            List of (region_index, similarity) sorted by similarity
        """
        text_emb = self.encode_text(description)

        results = []
        for idx, (x, y, w, h) in enumerate(regions):
            if image.dim() == 3:
                patch = image[:, y:y+h, x:x+w]
            else:
                patch = image[:, :, y:y+h, x:x+w]

            # Resize to model size
            patch = F.interpolate(
                patch.unsqueeze(0) if patch.dim() == 3 else patch,
                size=(384, 384),
                mode='bilinear',
                align_corners=False
            )

            patch_emb = self.encode_image(patch)
            similarity = F.cosine_similarity(text_emb, patch_emb, dim=-1).item()
            results.append((idx, similarity))

        results.sort(key=lambda x: x[1], reverse=True)
        return results

    @torch.no_grad()
    def compare_with_description(
        self,
        image: torch.Tensor,
        description: str
    ) -> Tuple[float, str]:
        """
        Check how well an image matches a text description.

        Returns:
            (similarity_score, analysis)
        """
        image_emb = self.encode_image(image)
        text_emb = self.encode_text(description)

        similarity = F.cosine_similarity(image_emb, text_emb, dim=-1).item()

        if similarity > 0.30:
            analysis = f"Strong match for '{description}'"
        elif similarity > 0.20:
            analysis = f"Moderate match for '{description}'"
        elif similarity > 0.10:
            analysis = f"Weak match for '{description}'"
        else:
            analysis = f"No match for '{description}'"

        return similarity, analysis

    def get_info(self) -> dict:
        """Get model info"""
        return {
            "model_id": self.MODEL_ID,
            "license": self.LICENSE,
            "embed_dim": self.embed_dim,
            "device": str(self.device),
            "use_hf": self.use_hf,
        }
