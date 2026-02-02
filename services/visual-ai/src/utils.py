"""
Utility functions for Visual AI service.
"""

import io
import torch
import torchvision.transforms as T
from PIL import Image
from typing import List, Union, Optional
import logging
import os

# MinIO/S3 imports
try:
    from minio import Minio
    HAS_MINIO = True
except ImportError:
    HAS_MINIO = False

logger = logging.getLogger(__name__)

# Standard image transform for all models
TRANSFORM = T.Compose([
    T.Resize((384, 384)),
    T.ToTensor(),
    T.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225]),
])


def get_minio_client() -> Optional["Minio"]:
    """Get MinIO client from environment"""
    if not HAS_MINIO:
        return None

    endpoint = os.environ.get("MINIO_ENDPOINT", "localhost:9000")
    access_key = os.environ.get("MINIO_ACCESS_KEY", "minioadmin")
    secret_key = os.environ.get("MINIO_SECRET_KEY", "minioadmin")
    secure = os.environ.get("MINIO_SECURE", "false").lower() == "true"

    # Remove http:// or https:// prefix if present
    if endpoint.startswith("http://"):
        endpoint = endpoint[7:]
    elif endpoint.startswith("https://"):
        endpoint = endpoint[8:]
        secure = True

    try:
        client = Minio(endpoint, access_key, secret_key, secure=secure)
        return client
    except Exception as e:
        logger.warning(f"Failed to create MinIO client: {e}")
        return None


_minio_client = None


def load_image(source: Union[bytes, str]) -> Image.Image:
    """
    Load image from bytes or URI.

    Args:
        source: Either raw bytes or a MinIO URI (s3://bucket/key)

    Returns:
        PIL Image
    """
    global _minio_client

    if isinstance(source, bytes):
        return Image.open(io.BytesIO(source)).convert("RGB")

    if isinstance(source, str):
        # Check if it's a MinIO/S3 URI
        if source.startswith("s3://") or source.startswith("minio://"):
            if _minio_client is None:
                _minio_client = get_minio_client()

            if _minio_client is None:
                raise RuntimeError("MinIO client not available")

            # Parse URI: s3://bucket/key
            parts = source.replace("s3://", "").replace("minio://", "").split("/", 1)
            bucket = parts[0]
            key = parts[1] if len(parts) > 1 else ""

            try:
                response = _minio_client.get_object(bucket, key)
                data = response.read()
                response.close()
                response.release_conn()
                return Image.open(io.BytesIO(data)).convert("RGB")
            except Exception as e:
                raise RuntimeError(f"Failed to load image from MinIO: {e}")

        # Local file path
        if os.path.exists(source):
            return Image.open(source).convert("RGB")

        raise ValueError(f"Unknown image source: {source}")

    raise TypeError(f"Expected bytes or str, got {type(source)}")


def preprocess_frame(image: Image.Image) -> torch.Tensor:
    """
    Preprocess a single image for model input.

    Args:
        image: PIL Image

    Returns:
        Tensor of shape (3, 384, 384)
    """
    return TRANSFORM(image)


def preprocess_frames(images: List[Image.Image]) -> List[torch.Tensor]:
    """
    Preprocess multiple images.

    Args:
        images: List of PIL Images

    Returns:
        List of tensors
    """
    return [preprocess_frame(img) for img in images]


def load_and_preprocess(source: Union[bytes, str]) -> torch.Tensor:
    """Load and preprocess an image in one step"""
    image = load_image(source)
    return preprocess_frame(image)


def load_and_preprocess_many(sources: List[Union[bytes, str]]) -> List[torch.Tensor]:
    """Load and preprocess multiple images"""
    return [load_and_preprocess(s) for s in sources]


def tensor_to_bytes(tensor: torch.Tensor) -> bytes:
    """Convert embedding tensor to bytes for gRPC transmission"""
    return tensor.cpu().numpy().tobytes()


def bytes_to_tensor(data: bytes, shape: tuple) -> torch.Tensor:
    """Convert bytes back to tensor"""
    import numpy as np
    arr = np.frombuffer(data, dtype=np.float32).reshape(shape)
    return torch.from_numpy(arr)


def calculate_changed_regions(
    frame1: torch.Tensor,
    frame2: torch.Tensor,
    threshold: float = 0.1,
    grid_size: int = 8
) -> List[dict]:
    """
    Calculate regions that changed between two frames.

    Simple pixel-level comparison on a grid.

    Returns:
        List of {x, y, width, height, significance}
    """
    if frame1.dim() == 4:
        frame1 = frame1.squeeze(0)
    if frame2.dim() == 4:
        frame2 = frame2.squeeze(0)

    _, H, W = frame1.shape
    patch_h = H // grid_size
    patch_w = W // grid_size

    changes = []

    for i in range(grid_size):
        for j in range(grid_size):
            y_start = i * patch_h
            y_end = (i + 1) * patch_h
            x_start = j * patch_w
            x_end = (j + 1) * patch_w

            patch1 = frame1[:, y_start:y_end, x_start:x_end]
            patch2 = frame2[:, y_start:y_end, x_start:x_end]

            diff = (patch1 - patch2).abs().mean().item()

            if diff > threshold:
                changes.append({
                    "x": x_start,
                    "y": y_start,
                    "width": patch_w,
                    "height": patch_h,
                    "significance": min(diff / threshold, 1.0),
                })

    return changes


def get_device() -> str:
    """Get the best available device"""
    if torch.cuda.is_available():
        return "cuda"
    elif hasattr(torch.backends, "mps") and torch.backends.mps.is_available():
        return "mps"
    return "cpu"


def get_gpu_memory_info() -> dict:
    """Get GPU memory information"""
    if not torch.cuda.is_available():
        return {"available": False}

    return {
        "available": True,
        "device_name": torch.cuda.get_device_name(0),
        "total_mb": torch.cuda.get_device_properties(0).total_memory // (1024 * 1024),
        "allocated_mb": torch.cuda.memory_allocated(0) // (1024 * 1024),
        "cached_mb": torch.cuda.memory_reserved(0) // (1024 * 1024),
    }
