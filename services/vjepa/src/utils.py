"""
Utility functions for V-JEPA service.
"""

import torch
from PIL import Image
import requests
from io import BytesIO
import numpy as np
from torchvision import transforms
import os
import logging

logger = logging.getLogger(__name__)

# Image preprocessing for vision models
preprocess = transforms.Compose([
    transforms.Resize((224, 224)),
    transforms.ToTensor(),
    transforms.Normalize(
        mean=[0.485, 0.456, 0.406],
        std=[0.229, 0.224, 0.225]
    ),
])


def preprocess_image(image: Image.Image) -> torch.Tensor:
    """Preprocess image for model input."""
    tensor = preprocess(image)
    return tensor.unsqueeze(0)  # Add batch dimension


def download_image(uri: str) -> Image.Image:
    """
    Download image from URI.

    Supports:
    - HTTP/HTTPS URLs
    - S3/MinIO URIs (s3://bucket/key)
    - Local file paths
    """
    if uri.startswith("http://") or uri.startswith("https://"):
        try:
            response = requests.get(uri, timeout=30)
            response.raise_for_status()
            return Image.open(BytesIO(response.content)).convert("RGB")
        except Exception as e:
            logger.error(f"Failed to download from HTTP: {e}")
            raise

    elif uri.startswith("s3://") or uri.startswith("minio://"):
        try:
            import boto3
            from botocore.config import Config

            # Parse URI
            uri_clean = uri.replace("s3://", "").replace("minio://", "")
            parts = uri_clean.split("/", 1)
            bucket = parts[0]
            key = parts[1] if len(parts) > 1 else ""

            # Get MinIO/S3 credentials from environment
            endpoint = os.environ.get("MINIO_ENDPOINT", "http://localhost:9000")
            access_key = os.environ.get("MINIO_ACCESS_KEY", "minioadmin")
            secret_key = os.environ.get("MINIO_SECRET_KEY", "minioadmin")

            # Create S3 client
            s3 = boto3.client(
                "s3",
                endpoint_url=endpoint,
                aws_access_key_id=access_key,
                aws_secret_access_key=secret_key,
                config=Config(signature_version='s3v4'),
            )

            response = s3.get_object(Bucket=bucket, Key=key)
            return Image.open(BytesIO(response["Body"].read())).convert("RGB")
        except Exception as e:
            logger.error(f"Failed to download from S3/MinIO: {e}")
            raise

    else:
        # Local file
        if os.path.exists(uri):
            return Image.open(uri).convert("RGB")
        else:
            raise FileNotFoundError(f"File not found: {uri}")


def find_changed_regions(
    baseline: np.ndarray,
    actual: np.ndarray,
    threshold: float = 30,
    min_region_size: int = 50
) -> list:
    """
    Find regions that changed between two images.

    Args:
        baseline: Baseline image as numpy array (H, W, 3)
        actual: Actual image as numpy array (H, W, 3)
        threshold: Pixel difference threshold
        min_region_size: Minimum region size in pixels

    Returns:
        List of changed region dictionaries
    """
    try:
        from scipy import ndimage
    except ImportError:
        logger.warning("scipy not available, using basic region detection")
        return _find_changed_regions_basic(baseline, actual, threshold)

    # Ensure same size
    if baseline.shape != actual.shape:
        actual = np.array(Image.fromarray(actual).resize(
            (baseline.shape[1], baseline.shape[0])
        ))

    # Compute difference
    diff = np.abs(baseline.astype(float) - actual.astype(float))
    diff_gray = np.mean(diff, axis=2)

    # Threshold to binary mask
    changed_mask = diff_gray > threshold

    # Find connected components
    labeled, num_features = ndimage.label(changed_mask)

    regions = []
    for i in range(1, num_features + 1):
        region_mask = labeled == i
        ys, xs = np.where(region_mask)

        if len(xs) < min_region_size:
            continue

        x_min, x_max = int(xs.min()), int(xs.max())
        y_min, y_max = int(ys.min()), int(ys.max())

        # Calculate change intensity in this region
        region_diff = diff_gray[region_mask]
        avg_intensity = float(np.mean(region_diff))

        regions.append({
            "x": x_min,
            "y": y_min,
            "width": x_max - x_min,
            "height": y_max - y_min,
            "pixel_count": len(xs),
            "avg_intensity": avg_intensity,
            "significance": min(1.0, avg_intensity / 128.0)  # Normalize to 0-1
        })

    # Sort by significance
    regions.sort(key=lambda r: r["significance"], reverse=True)

    return regions


def _find_changed_regions_basic(
    baseline: np.ndarray,
    actual: np.ndarray,
    threshold: float = 30
) -> list:
    """Basic region detection without scipy."""
    # Ensure same size
    if baseline.shape != actual.shape:
        actual = np.array(Image.fromarray(actual).resize(
            (baseline.shape[1], baseline.shape[0])
        ))

    # Compute difference
    diff = np.abs(baseline.astype(float) - actual.astype(float))
    diff_gray = np.mean(diff, axis=2)

    # Find bounding box of all changes
    changed_mask = diff_gray > threshold
    if not np.any(changed_mask):
        return []

    ys, xs = np.where(changed_mask)
    if len(xs) == 0:
        return []

    return [{
        "x": int(xs.min()),
        "y": int(ys.min()),
        "width": int(xs.max() - xs.min()),
        "height": int(ys.max() - ys.min()),
        "pixel_count": len(xs),
        "avg_intensity": float(np.mean(diff_gray[changed_mask])),
        "significance": min(1.0, float(np.mean(diff_gray[changed_mask])) / 128.0)
    }]


def compute_structural_similarity(
    baseline: np.ndarray,
    actual: np.ndarray
) -> float:
    """
    Compute structural similarity index (SSIM) between images.

    Returns value between 0 and 1, where 1 means identical.
    """
    try:
        from skimage.metrics import structural_similarity as ssim
        # Convert to grayscale
        baseline_gray = np.mean(baseline, axis=2) if baseline.ndim == 3 else baseline
        actual_gray = np.mean(actual, axis=2) if actual.ndim == 3 else actual

        # Ensure same size
        if baseline_gray.shape != actual_gray.shape:
            actual_gray = np.array(Image.fromarray(actual_gray.astype(np.uint8)).resize(
                (baseline_gray.shape[1], baseline_gray.shape[0])
            ))

        return ssim(baseline_gray, actual_gray, data_range=255)
    except ImportError:
        # Fallback to simple correlation
        baseline_flat = baseline.flatten().astype(float)
        actual_flat = actual.flatten().astype(float)

        if baseline_flat.shape != actual_flat.shape:
            return 0.0

        correlation = np.corrcoef(baseline_flat, actual_flat)[0, 1]
        return max(0.0, correlation)


def generate_diff_heatmap(
    baseline: np.ndarray,
    actual: np.ndarray
) -> np.ndarray:
    """Generate a heatmap showing differences between images."""
    # Ensure same size
    if baseline.shape != actual.shape:
        actual = np.array(Image.fromarray(actual).resize(
            (baseline.shape[1], baseline.shape[0])
        ))

    # Compute difference
    diff = np.abs(baseline.astype(float) - actual.astype(float))
    diff_gray = np.mean(diff, axis=2)

    # Normalize to 0-255
    diff_normalized = (diff_gray / diff_gray.max() * 255).astype(np.uint8) if diff_gray.max() > 0 else diff_gray.astype(np.uint8)

    # Create colored heatmap (blue to red)
    heatmap = np.zeros((*diff_normalized.shape, 3), dtype=np.uint8)
    heatmap[:, :, 0] = diff_normalized  # Red channel
    heatmap[:, :, 2] = 255 - diff_normalized  # Blue channel

    return heatmap
