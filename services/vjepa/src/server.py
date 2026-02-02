"""
V-JEPA 2 Visual Validation gRPC Service

Provides visual understanding capabilities for:
- Self-healing validation
- Visual regression detection
- UI stability detection
- Screenshot comparison
"""

import grpc
from concurrent import futures
import logging
import torch
import torch.nn.functional as F
from PIL import Image
import io
import numpy as np
from typing import List, Optional
import time
import os
import sys

# Add proto directory to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'proto'))

from model import VJEPAModel
from utils import download_image, preprocess_image, find_changed_regions

# Import generated protobuf modules
try:
    import vjepa_pb2
    import vjepa_pb2_grpc
except ImportError:
    # Generate if not exists
    import subprocess
    proto_dir = os.path.join(os.path.dirname(__file__), '..', 'proto')
    subprocess.run([
        'python', '-m', 'grpc_tools.protoc',
        f'-I{proto_dir}',
        f'--python_out={proto_dir}',
        f'--grpc_python_out={proto_dir}',
        os.path.join(proto_dir, 'vjepa.proto')
    ])
    import vjepa_pb2
    import vjepa_pb2_grpc

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class VJEPAServicer(vjepa_pb2_grpc.VJEPAServiceServicer):
    """V-JEPA 2 Visual Validation Service Implementation."""

    def __init__(self, model_path: str, device: str = "cuda"):
        self.device = torch.device(device if torch.cuda.is_available() else "cpu")
        logger.info(f"Initializing V-JEPA service on {self.device}")

        # Load model
        self.model = VJEPAModel.from_pretrained(model_path)
        self.model.to(self.device)
        self.model.eval()

        # Use FP16 for memory efficiency on GPU
        if self.device.type == "cuda":
            self.model = self.model.half()
            logger.info("Using FP16 precision for GPU inference")

        # Warm up the model
        self._warmup()

        # Metrics tracking
        self.inference_times: List[float] = []
        self.request_count = 0

        logger.info("V-JEPA service initialized successfully")

    def _warmup(self):
        """Warm up the model with a dummy inference."""
        logger.info("Warming up model...")
        dummy = torch.randn(1, 3, 224, 224).to(self.device)
        if self.device.type == "cuda":
            dummy = dummy.half()
        with torch.no_grad():
            _ = self.model.encode(dummy)
        if self.device.type == "cuda":
            torch.cuda.synchronize()
        logger.info("Model warmup complete")

    def _load_image(self, data: bytes = None, uri: str = None) -> Image.Image:
        """Load image from bytes or URI."""
        if data and len(data) > 0:
            return Image.open(io.BytesIO(data)).convert("RGB")
        elif uri and len(uri) > 0:
            return download_image(uri)
        else:
            raise ValueError("Either data or uri must be provided")

    def _get_embedding(self, image: Image.Image) -> torch.Tensor:
        """Get embedding for an image."""
        tensor = preprocess_image(image).to(self.device)
        if self.device.type == "cuda":
            tensor = tensor.half()

        with torch.no_grad():
            embedding = self.model.encode(tensor)

        return F.normalize(embedding, p=2, dim=-1)

    def CompareFrames(self, request, context):
        """Compare two frames and return semantic similarity."""
        start_time = time.time()
        self.request_count += 1

        try:
            # Load images
            baseline_img = self._load_image(
                data=request.baseline_data if request.baseline_data else None,
                uri=request.baseline_uri if request.baseline_uri else None
            )
            actual_img = self._load_image(
                data=request.actual_data if request.actual_data else None,
                uri=request.actual_uri if request.actual_uri else None
            )

            # Get embeddings
            baseline_emb = self._get_embedding(baseline_img)
            actual_emb = self._get_embedding(actual_img)

            # Compute cosine similarity
            similarity = F.cosine_similarity(baseline_emb, actual_emb).item()

            # Determine threshold
            threshold = 0.85
            if request.settings and request.settings.similarity_threshold > 0:
                threshold = request.settings.similarity_threshold
            semantic_match = similarity >= threshold

            # Find changed regions if not matching
            changed_regions = []
            if not semantic_match:
                baseline_np = np.array(baseline_img.resize((512, 512)))
                actual_np = np.array(actual_img.resize((512, 512)))
                regions = find_changed_regions(baseline_np, actual_np)

                for r in regions[:10]:  # Limit to top 10 regions
                    changed_regions.append(vjepa_pb2.ChangedRegion(
                        region=vjepa_pb2.Region(
                            x=r["x"],
                            y=r["y"],
                            width=r["width"],
                            height=r["height"]
                        ),
                        change_type="modified",
                        significance=r.get("significance", 0.5),
                        description=f"Visual change detected at ({r['x']}, {r['y']})"
                    ))

            # Generate analysis
            analysis = self._generate_analysis(similarity, semantic_match, changed_regions, request.context)

            # Track inference time
            inference_time = (time.time() - start_time) * 1000
            self.inference_times.append(inference_time)
            if len(self.inference_times) > 1000:
                self.inference_times = self.inference_times[-1000:]

            logger.info(f"CompareFrames: similarity={similarity:.4f}, match={semantic_match}, time={inference_time:.2f}ms")

            return vjepa_pb2.CompareFramesResponse(
                similarity_score=similarity,
                semantic_match=semantic_match,
                confidence=0.95,
                changed_regions=changed_regions,
                analysis=analysis,
                baseline_embedding=baseline_emb.cpu().float().numpy().tobytes(),
                actual_embedding=actual_emb.cpu().float().numpy().tobytes(),
            )

        except Exception as e:
            logger.error(f"CompareFrames error: {e}", exc_info=True)
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return vjepa_pb2.CompareFramesResponse()

    def _generate_analysis(
        self,
        similarity: float,
        semantic_match: bool,
        changed_regions: List,
        ctx: str
    ) -> str:
        """Generate human-readable analysis."""
        if semantic_match:
            return f"Frames are semantically equivalent (similarity: {similarity:.1%}). The UI state matches expected behavior."

        if similarity > 0.7:
            return f"Minor visual differences detected (similarity: {similarity:.1%}). {len(changed_regions)} region(s) changed. This may be acceptable depending on test requirements."

        if similarity > 0.5:
            return f"Significant visual differences detected (similarity: {similarity:.1%}). {len(changed_regions)} region(s) changed significantly. Review recommended."

        return f"Major visual differences detected (similarity: {similarity:.1%}). The UI appears substantially different from baseline. This likely indicates a failure or major change."

    def DetectStability(self, request, context):
        """Detect if UI is stable (not loading/animating)."""
        try:
            # Load frames
            frames = []
            if request.frames:
                frames = [self._load_image(data=f) for f in request.frames]
            elif request.frame_uris:
                frames = [self._load_image(uri=u) for u in request.frame_uris]

            if len(frames) < 2:
                return vjepa_pb2.DetectStabilityResponse(
                    is_stable=True,
                    stable_frame_index=0,
                    stability_score=1.0,
                    analysis="Single frame provided, assuming stable"
                )

            threshold = request.stability_threshold if request.stability_threshold > 0 else 0.98
            min_stable = request.min_stable_frames if request.min_stable_frames > 0 else 3

            # Get embeddings for all frames
            embeddings = [self._get_embedding(f) for f in frames]

            # Find first stable point
            stable_count = 0
            stable_frame_index = 0
            similarities = []

            for i in range(1, len(embeddings)):
                similarity = F.cosine_similarity(embeddings[i-1], embeddings[i]).item()
                similarities.append(similarity)

                if similarity >= threshold:
                    stable_count += 1
                    if stable_count >= min_stable - 1:
                        stable_frame_index = max(0, i - min_stable + 1)
                        break
                else:
                    stable_count = 0

            is_stable = stable_count >= min_stable - 1
            stability_score = sum(1 for s in similarities if s >= threshold) / len(similarities) if similarities else 1.0

            if is_stable:
                analysis = f"UI is stable from frame {stable_frame_index} (avg similarity: {np.mean(similarities):.3f})"
            else:
                analysis = f"UI still changing, {stable_count} consecutive stable frames detected (threshold: {min_stable})"

            logger.info(f"DetectStability: stable={is_stable}, frame={stable_frame_index}, score={stability_score:.3f}")

            return vjepa_pb2.DetectStabilityResponse(
                is_stable=is_stable,
                stable_frame_index=stable_frame_index,
                stability_score=stability_score,
                analysis=analysis
            )

        except Exception as e:
            logger.error(f"DetectStability error: {e}", exc_info=True)
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return vjepa_pb2.DetectStabilityResponse()

    def GenerateEmbedding(self, request, context):
        """Generate embedding vector for a frame."""
        try:
            image = self._load_image(
                data=request.image_data if request.image_data else None,
                uri=request.image_uri if request.image_uri else None
            )

            embedding = self._get_embedding(image)

            if request.normalize:
                embedding = F.normalize(embedding, p=2, dim=-1)

            embedding_np = embedding.cpu().float().numpy()

            logger.info(f"GenerateEmbedding: dim={embedding_np.shape[-1]}")

            return vjepa_pb2.GenerateEmbeddingResponse(
                embedding=embedding_np.tobytes(),
                embedding_dim=embedding_np.shape[-1],
                model_version="vjepa2-visual-encoder-v1"
            )

        except Exception as e:
            logger.error(f"GenerateEmbedding error: {e}", exc_info=True)
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return vjepa_pb2.GenerateEmbeddingResponse()

    def BatchCompare(self, request, context):
        """Compare multiple frame pairs."""
        results = []
        total_similarity = 0.0
        matches = 0

        for pair in request.pairs:
            compare_req = vjepa_pb2.CompareFramesRequest(
                baseline_data=pair.baseline_data,
                baseline_uri=pair.baseline_uri,
                actual_data=pair.actual_data,
                actual_uri=pair.actual_uri,
                context=pair.context,
                settings=request.settings
            )

            result = self.CompareFrames(compare_req, context)

            results.append(vjepa_pb2.BatchCompareResult(
                pair_id=pair.pair_id,
                result=result
            ))

            total_similarity += result.similarity_score
            if result.semantic_match:
                matches += 1

        avg_similarity = total_similarity / len(request.pairs) if request.pairs else 0.0

        logger.info(f"BatchCompare: {len(request.pairs)} pairs, avg_similarity={avg_similarity:.3f}, matches={matches}")

        return vjepa_pb2.BatchCompareResponse(
            results=results,
            average_similarity=avg_similarity,
            matches=matches,
            mismatches=len(request.pairs) - matches
        )

    def AnalyzeChange(self, request, context):
        """Analyze and describe changes between frames."""
        try:
            before = self._load_image(
                data=request.before_data if request.before_data else None,
                uri=request.before_uri if request.before_uri else None
            )
            after = self._load_image(
                data=request.after_data if request.after_data else None,
                uri=request.after_uri if request.after_uri else None
            )

            before_emb = self._get_embedding(before)
            after_emb = self._get_embedding(after)

            similarity = F.cosine_similarity(before_emb, after_emb).item()

            # Find changed regions
            changes = []
            if similarity < 0.95:
                before_np = np.array(before.resize((512, 512)))
                after_np = np.array(after.resize((512, 512)))
                regions = find_changed_regions(before_np, after_np)
                for r in regions[:5]:
                    changes.append(f"Change at ({r['x']}, {r['y']}): {r['width']}x{r['height']} pixels")

            # Determine if change is expected based on action
            action = request.action_performed.lower() if request.action_performed else ""
            expected = False

            if "click" in action and similarity < 0.95:
                expected = True
            elif "navigate" in action and similarity < 0.8:
                expected = True
            elif "fill" in action and 0.85 < similarity < 0.99:
                expected = True
            elif "type" in action and 0.90 < similarity < 0.99:
                expected = True
            elif similarity > 0.95 and ("click" not in action and "submit" not in action):
                expected = True

            # Generate description
            description = f"After '{request.action_performed}': "
            if similarity > 0.95:
                description += "No significant visual change detected."
            elif similarity > 0.8:
                description += f"Minor changes detected ({len(changes)} regions affected)."
            else:
                description += f"Significant changes detected ({len(changes)} regions affected)."

            logger.info(f"AnalyzeChange: action='{request.action_performed}', similarity={similarity:.3f}, expected={expected}")

            return vjepa_pb2.AnalyzeChangeResponse(
                description=description,
                changes=changes,
                expected_change=expected,
                confidence=0.9
            )

        except Exception as e:
            logger.error(f"AnalyzeChange error: {e}", exc_info=True)
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return vjepa_pb2.AnalyzeChangeResponse()

    def HealthCheck(self, request, context):
        """Health check endpoint."""
        memory_used = 0
        memory_total = 0

        if torch.cuda.is_available():
            memory_used = torch.cuda.memory_allocated() // (1024 * 1024)
            memory_total = torch.cuda.get_device_properties(0).total_memory // (1024 * 1024)

        avg_inference = 0.0
        if self.inference_times:
            recent = self.inference_times[-100:]
            avg_inference = sum(recent) / len(recent)

        return vjepa_pb2.HealthCheckResponse(
            healthy=True,
            model_loaded="vjepa2-visual-encoder-v1",
            device=str(self.device),
            memory_used_mb=memory_used,
            memory_total_mb=memory_total,
            avg_inference_ms=avg_inference
        )


def serve(port: int = 50051, model_path: str = "./models/vjepa2", max_workers: int = 4):
    """Start the gRPC server."""
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=max_workers),
        options=[
            ('grpc.max_send_message_length', 50 * 1024 * 1024),  # 50MB
            ('grpc.max_receive_message_length', 50 * 1024 * 1024),
        ]
    )
    vjepa_pb2_grpc.add_VJEPAServiceServicer_to_server(
        VJEPAServicer(model_path), server
    )
    server.add_insecure_port(f"[::]:{port}")
    server.start()
    logger.info(f"V-JEPA service started on port {port}")
    server.wait_for_termination()


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="V-JEPA 2 Visual Validation Service")
    parser.add_argument("--port", type=int, default=50051, help="Port to listen on")
    parser.add_argument("--model-path", type=str, default="./models/vjepa2", help="Path to model weights")
    parser.add_argument("--device", type=str, default="cuda", help="Device (cuda or cpu)")
    parser.add_argument("--workers", type=int, default=4, help="Number of worker threads")
    args = parser.parse_args()

    serve(args.port, args.model_path, args.workers)
