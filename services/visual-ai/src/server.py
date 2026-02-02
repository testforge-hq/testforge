"""
Visual AI Service - Unified gRPC server with multi-model support.

Models (all commercially safe):
- DINOv2: Fast baseline (Apache 2.0)
- V-JEPA 2: Video/sequence understanding (MIT + Apache 2.0)
- SigLIP: Text-image search (Apache 2.0)
"""

import grpc
from concurrent import futures
import logging
import torch
from typing import Dict, Optional, List
import os
import sys
import time

# Add parent directory to path for proto imports
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from proto import visual_ai_pb2, visual_ai_pb2_grpc
from router import ModelRouter, TaskType
from models.dinov2 import DINOv2Encoder
from models.vjepa2 import VJEPA2Encoder
from models.siglip import SigLIPEncoder
from utils import (
    load_image, preprocess_frame, preprocess_frames,
    load_and_preprocess, load_and_preprocess_many,
    tensor_to_bytes, calculate_changed_regions,
    get_device, get_gpu_memory_info
)

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class VisualAIServicer(visual_ai_pb2_grpc.VisualAIServiceServicer):
    """
    Unified Visual AI service with intelligent model routing.

    Models (all commercially safe):
    - DINOv2: Fast baseline (Apache 2.0)
    - V-JEPA 2: Video/sequence understanding (MIT + Apache 2.0)
    - SigLIP: Text-image search (Apache 2.0)
    """

    def __init__(self, config: dict):
        self.device = config.get("device", get_device())
        self.models: Dict[str, any] = {}
        self.inference_times: Dict[str, List[float]] = {}

        # Load requested models
        models_to_load = config.get("models", ["dinov2"])

        if "dinov2" in models_to_load:
            logger.info("Loading DINOv2 (Apache 2.0)...")
            try:
                self.models["dinov2"] = DINOv2Encoder(self.device)
                self.inference_times["dinov2"] = []
                logger.info("DINOv2 loaded successfully")
            except Exception as e:
                logger.error(f"Failed to load DINOv2: {e}")

        if "vjepa2" in models_to_load:
            logger.info("Loading V-JEPA 2 (MIT + Apache 2.0)...")
            try:
                self.models["vjepa2"] = VJEPA2Encoder(self.device)
                self.inference_times["vjepa2"] = []
                logger.info("V-JEPA 2 loaded successfully")
            except Exception as e:
                logger.error(f"Failed to load V-JEPA 2: {e}")

        if "siglip" in models_to_load:
            logger.info("Loading SigLIP (Apache 2.0)...")
            try:
                self.models["siglip"] = SigLIPEncoder(self.device)
                self.inference_times["siglip"] = []
                logger.info("SigLIP loaded successfully")
            except Exception as e:
                logger.error(f"Failed to load SigLIP: {e}")

        # Initialize router
        self.router = ModelRouter(list(self.models.keys()))

        logger.info(f"Visual AI service ready with models: {list(self.models.keys())}")

    def _get_model(self, task_type: TaskType, explicit_model: Optional[str] = None):
        """Get the appropriate model for a task"""
        model_name = self.router.route(task_type, explicit_model)
        return model_name, self.models.get(model_name)

    def _record_inference_time(self, model_name: str, elapsed: float):
        """Record inference time for metrics"""
        times = self.inference_times.get(model_name, [])
        times.append(elapsed)
        # Keep last 100 measurements
        if len(times) > 100:
            times = times[-100:]
        self.inference_times[model_name] = times

    def _get_avg_inference_time(self, model_name: str) -> float:
        """Get average inference time in ms"""
        times = self.inference_times.get(model_name, [])
        if not times:
            return 0.0
        return (sum(times) / len(times)) * 1000  # Convert to ms

    def CompareFrames(self, request, context):
        """Compare two frames - uses DINOv2 by default (fast)"""
        start_time = time.time()

        # Determine model
        explicit_model = None
        if request.model == visual_ai_pb2.MODEL_DINOV2:
            explicit_model = "dinov2"
        elif request.model == visual_ai_pb2.MODEL_VJEPA2:
            explicit_model = "vjepa2"

        model_name, encoder = self._get_model(TaskType.COMPARE_SIMPLE, explicit_model)

        if encoder is None:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("No encoder available")
            return visual_ai_pb2.CompareFramesResponse()

        try:
            # Load images
            baseline_source = request.baseline_data or request.baseline_uri
            actual_source = request.actual_data or request.actual_uri

            baseline = load_and_preprocess(baseline_source)
            actual = load_and_preprocess(actual_source)

            # Compare
            threshold = request.settings.similarity_threshold or 0.85
            similarity, is_similar, analysis = encoder.compare(baseline, actual, threshold)

            # Calculate changed regions if different
            changed_regions = []
            if not is_similar:
                raw_changes = calculate_changed_regions(baseline, actual)
                for change in raw_changes:
                    changed_regions.append(visual_ai_pb2.ChangedRegion(
                        region=visual_ai_pb2.Region(
                            x=change["x"],
                            y=change["y"],
                            width=change["width"],
                            height=change["height"]
                        ),
                        significance=change["significance"],
                        change_type="content",
                        description="Visual difference detected"
                    ))

            elapsed = time.time() - start_time
            self._record_inference_time(model_name, elapsed)

            return visual_ai_pb2.CompareFramesResponse(
                similarity_score=similarity,
                semantic_match=is_similar,
                confidence=0.95,
                model_used=model_name,
                analysis=analysis,
                changed_regions=changed_regions
            )

        except Exception as e:
            logger.error(f"CompareFrames error: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return visual_ai_pb2.CompareFramesResponse()

    def ValidateHealing(self, request, context):
        """
        Validate self-healing using V-JEPA 2.

        V-JEPA 2 is ideal here because it understands:
        - State transitions (before → after)
        - Semantic equivalence (different pixels, same outcome)
        """
        start_time = time.time()

        model_name, encoder = self._get_model(TaskType.HEALING_VALIDATION)

        if encoder is None:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("No encoder available")
            return visual_ai_pb2.ValidateHealingResponse()

        try:
            # Load frame sequences
            frame_sources = list(request.frame_sequence) or list(request.frame_uris)
            expected_source = request.expected_state or request.expected_state_uri

            if not frame_sources:
                context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
                context.set_details("No frames provided")
                return visual_ai_pb2.ValidateHealingResponse()

            # Split frames into before/after
            mid = len(frame_sources) // 2
            before_frames = [load_and_preprocess(f) for f in frame_sources[:max(1, mid)]]
            after_frames = [load_and_preprocess(f) for f in frame_sources[max(1, mid):]]
            expected_frames = [load_and_preprocess(expected_source)] if expected_source else after_frames

            threshold = request.similarity_threshold or 0.85

            # Validate healing
            if hasattr(encoder, 'validate_healing'):
                is_valid, similarity, confidence, analysis = encoder.validate_healing(
                    before_frames, after_frames, expected_frames, threshold
                )
            else:
                # Fallback for DINOv2
                actual_emb = encoder.encode_single(after_frames[-1])
                expected_emb = encoder.encode_single(expected_frames[-1])
                similarity = torch.nn.functional.cosine_similarity(actual_emb, expected_emb, dim=-1).item()
                is_valid = similarity >= threshold
                confidence = 0.7
                analysis = f"Simple comparison (V-JEPA 2 not available): similarity={similarity:.2%}"

            elapsed = time.time() - start_time
            self._record_inference_time(model_name, elapsed)

            # Build state comparisons
            state_comparisons = [
                visual_ai_pb2.StateComparison(
                    frame_pair="actual_vs_expected",
                    similarity=similarity,
                    analysis=analysis
                )
            ]

            return visual_ai_pb2.ValidateHealingResponse(
                valid=is_valid,
                semantic_similarity=similarity,
                state_confidence=confidence,
                transition_analysis=analysis,
                expected_transition=is_valid,
                state_comparisons=state_comparisons
            )

        except Exception as e:
            logger.error(f"ValidateHealing error: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return visual_ai_pb2.ValidateHealingResponse()

    def DetectStability(self, request, context):
        """
        Detect UI stability using V-JEPA 2.

        V-JEPA 2 understands motion/activity in frame sequences.
        """
        start_time = time.time()

        model_name, encoder = self._get_model(TaskType.STABILITY_CHECK)

        if encoder is None:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("No encoder available")
            return visual_ai_pb2.DetectStabilityResponse()

        try:
            # Load frames
            frame_sources = list(request.frames) or list(request.frame_uris)
            if not frame_sources:
                context.set_code(grpc.StatusCode.INVALID_ARGUMENT)
                context.set_details("No frames provided")
                return visual_ai_pb2.DetectStabilityResponse()

            frames = [load_and_preprocess(f) for f in frame_sources]
            threshold = request.stability_threshold or 0.98

            # Detect stability
            if hasattr(encoder, 'detect_stability'):
                is_stable, stable_at, stability_score, activity = encoder.detect_stability(frames, threshold)
            else:
                # Fallback: simple consecutive comparison
                is_stable, stable_at, stability_score, activity = self._simple_stability(
                    encoder, frames, threshold
                )

            elapsed = time.time() - start_time
            self._record_inference_time(model_name, elapsed)

            return visual_ai_pb2.DetectStabilityResponse(
                is_stable=is_stable,
                stable_at_frame=stable_at,
                stability_score=stability_score,
                motion_analysis=activity,
                detected_activities=[activity]
            )

        except Exception as e:
            logger.error(f"DetectStability error: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return visual_ai_pb2.DetectStabilityResponse()

    def _simple_stability(self, encoder, frames, threshold):
        """Simple stability detection for fallback"""
        if len(frames) < 2:
            return True, 0, 1.0, "single_frame"

        embeddings = [encoder.encode_single(f) for f in frames]
        similarities = []

        for i in range(len(embeddings) - 1):
            sim = torch.nn.functional.cosine_similarity(
                embeddings[i], embeddings[i+1], dim=-1
            ).item()
            similarities.append(sim)

        avg_sim = sum(similarities) / len(similarities)
        is_stable = all(s >= threshold for s in similarities[-2:]) if len(similarities) >= 2 else avg_sim >= threshold

        stable_at = len(frames) - 1
        for i, sim in enumerate(similarities):
            if i >= len(similarities) - 2 and sim >= threshold:
                stable_at = i
                break

        activity = "static" if avg_sim > 0.98 else "motion"

        return is_stable, stable_at, avg_sim, activity

    def FindByDescription(self, request, context):
        """Find element by text description using SigLIP"""
        start_time = time.time()

        model_name, encoder = self._get_model(TaskType.TEXT_SEARCH)

        if encoder is None or not hasattr(encoder, 'find_by_description'):
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("SigLIP not available")
            return visual_ai_pb2.FindByDescriptionResponse()

        try:
            # Load screenshot
            screenshot_source = request.screenshot or request.screenshot_uri
            screenshot = load_and_preprocess(screenshot_source)

            max_results = request.max_results or 5

            # Find matching regions
            results = encoder.find_by_description(
                screenshot,
                request.description,
                max_results=max_results
            )

            elapsed = time.time() - start_time
            self._record_inference_time(model_name, elapsed)

            elements = []
            for x, y, w, h, confidence in results:
                elements.append(visual_ai_pb2.FoundElement(
                    region=visual_ai_pb2.Region(x=x, y=y, width=w, height=h),
                    confidence=confidence,
                    matched_description=request.description
                ))

            return visual_ai_pb2.FindByDescriptionResponse(elements=elements)

        except Exception as e:
            logger.error(f"FindByDescription error: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return visual_ai_pb2.FindByDescriptionResponse()

    def GenerateEmbedding(self, request, context):
        """Generate embedding for an image"""
        start_time = time.time()

        # Determine model
        explicit_model = None
        if request.model == visual_ai_pb2.MODEL_DINOV2:
            explicit_model = "dinov2"
        elif request.model == visual_ai_pb2.MODEL_VJEPA2:
            explicit_model = "vjepa2"
        elif request.model == visual_ai_pb2.MODEL_SIGLIP:
            explicit_model = "siglip"

        model_name, encoder = self._get_model(TaskType.EMBEDDING, explicit_model)

        if encoder is None:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("No encoder available")
            return visual_ai_pb2.GenerateEmbeddingResponse()

        try:
            # Load image
            image_source = request.image_data or request.image_uri
            image = load_and_preprocess(image_source)

            # Generate embedding
            embedding = encoder.encode_single(image)
            if request.normalize:
                embedding = torch.nn.functional.normalize(embedding, p=2, dim=-1)

            elapsed = time.time() - start_time
            self._record_inference_time(model_name, elapsed)

            return visual_ai_pb2.GenerateEmbeddingResponse(
                embedding=tensor_to_bytes(embedding),
                embedding_dim=embedding.shape[-1],
                model_used=model_name,
                model_version=getattr(encoder, 'MODEL_ID', 'unknown')
            )

        except Exception as e:
            logger.error(f"GenerateEmbedding error: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return visual_ai_pb2.GenerateEmbeddingResponse()

    def BatchCompare(self, request, context):
        """Compare multiple frame pairs"""
        start_time = time.time()

        explicit_model = None
        if request.model == visual_ai_pb2.MODEL_DINOV2:
            explicit_model = "dinov2"
        elif request.model == visual_ai_pb2.MODEL_VJEPA2:
            explicit_model = "vjepa2"

        model_name, encoder = self._get_model(TaskType.COMPARE_SIMPLE, explicit_model)

        if encoder is None:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("No encoder available")
            return visual_ai_pb2.BatchCompareResponse()

        try:
            threshold = request.settings.similarity_threshold or 0.85
            results = []
            total_sim = 0.0
            matches = 0

            for pair in request.pairs:
                baseline_source = pair.baseline_data or pair.baseline_uri
                actual_source = pair.actual_data or pair.actual_uri

                baseline = load_and_preprocess(baseline_source)
                actual = load_and_preprocess(actual_source)

                similarity, is_similar, analysis = encoder.compare(baseline, actual, threshold)
                total_sim += similarity
                if is_similar:
                    matches += 1

                results.append(visual_ai_pb2.PairResult(
                    pair_id=pair.pair_id,
                    similarity_score=similarity,
                    semantic_match=is_similar,
                    analysis=analysis
                ))

            elapsed = time.time() - start_time
            self._record_inference_time(model_name, elapsed)

            return visual_ai_pb2.BatchCompareResponse(
                results=results,
                average_similarity=total_sim / len(results) if results else 0.0,
                matches=matches,
                mismatches=len(results) - matches
            )

        except Exception as e:
            logger.error(f"BatchCompare error: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return visual_ai_pb2.BatchCompareResponse()

    def AnalyzeChange(self, request, context):
        """Analyze visual change between frames"""
        start_time = time.time()

        model_name, encoder = self._get_model(TaskType.CHANGE_ANALYSIS)

        if encoder is None:
            context.set_code(grpc.StatusCode.UNAVAILABLE)
            context.set_details("No encoder available")
            return visual_ai_pb2.AnalyzeChangeResponse()

        try:
            before_source = request.before_data or request.before_uri
            after_source = request.after_data or request.after_uri

            before = load_and_preprocess(before_source)
            after = load_and_preprocess(after_source)

            if hasattr(encoder, 'analyze_change'):
                description, changes, expected, confidence = encoder.analyze_change(
                    before, after, request.action_performed
                )
            else:
                # Fallback
                similarity, _, analysis = encoder.compare(before, after)
                change_mag = 1 - similarity
                description = analysis
                changes = ["visual_change"] if change_mag > 0.05 else []
                expected = True
                confidence = 0.7

            elapsed = time.time() - start_time
            self._record_inference_time(model_name, elapsed)

            # Determine severity
            if len(changes) == 0:
                severity = "none"
                change_type = "none"
            elif "major" in str(changes).lower():
                severity = "major"
                change_type = "content"
            elif "minor" in str(changes).lower():
                severity = "minor"
                change_type = "style"
            else:
                severity = "moderate"
                change_type = "content"

            return visual_ai_pb2.AnalyzeChangeResponse(
                description=description,
                changes=changes,
                expected_change=expected,
                confidence=confidence,
                change_type=change_type,
                severity=severity
            )

        except Exception as e:
            logger.error(f"AnalyzeChange error: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return visual_ai_pb2.AnalyzeChangeResponse()

    def HealthCheck(self, request, context):
        """Health check with model status"""
        models_health = {}

        for name, encoder in self.models.items():
            info = encoder.get_info()
            models_health[name] = visual_ai_pb2.ModelHealth(
                loaded=True,
                version=info.get("model_id", "unknown"),
                license=info.get("license", "unknown"),
                memory_mb=info.get("memory_mb", 0),
                avg_inference_ms=self._get_avg_inference_time(name)
            )

        gpu_info = get_gpu_memory_info()

        return visual_ai_pb2.HealthCheckResponse(
            healthy=len(self.models) > 0,
            models=models_health,
            device=self.device,
            total_memory_mb=gpu_info.get("total_mb", 0),
            used_memory_mb=gpu_info.get("allocated_mb", 0)
        )


def serve(port: int = 50051):
    """Start the gRPC server"""
    # Parse config from environment
    models_str = os.environ.get("VISUAL_AI_MODELS", "dinov2,vjepa2")
    models = [m.strip() for m in models_str.split(",") if m.strip()]

    config = {
        "device": get_device(),
        "models": models,
    }

    logger.info(f"Starting Visual AI service with config: {config}")

    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=4),
        options=[
            ('grpc.max_send_message_length', 100 * 1024 * 1024),  # 100MB
            ('grpc.max_receive_message_length', 100 * 1024 * 1024),
        ]
    )

    visual_ai_pb2_grpc.add_VisualAIServiceServicer_to_server(
        VisualAIServicer(config), server
    )

    server.add_insecure_port(f"[::]:{port}")
    server.start()

    logger.info(f"Visual AI service started on port {port}")
    logger.info(f"Device: {config['device']}")
    logger.info(f"Models: {config['models']}")

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        logger.info("Shutting down...")
        server.stop(grace=5)


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Visual AI gRPC Service")
    parser.add_argument("--port", type=int, default=50051, help="Port to listen on")
    args = parser.parse_args()

    serve(args.port)
