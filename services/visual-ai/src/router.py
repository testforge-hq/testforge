"""
Model Router - Intelligent routing between visual AI models.

Routing Strategy:
- DINOv2: Fast, always-on, single-frame tasks
- V-JEPA 2: Sequence understanding, state transitions, complex reasoning
- SigLIP: Text-to-image search, semantic descriptions
"""

from enum import Enum
from typing import Optional, List, Dict
import logging

logger = logging.getLogger(__name__)


class TaskType(Enum):
    COMPARE_SIMPLE = "compare_simple"           # Single frame comparison
    COMPARE_SEQUENCE = "compare_sequence"       # Multiple frames
    STABILITY_CHECK = "stability_check"         # Is UI stable?
    HEALING_VALIDATION = "healing_validation"   # Validate self-healing
    EMBEDDING = "embedding"                     # Generate embedding
    TEXT_SEARCH = "text_search"                 # Find by description
    VISUAL_REGRESSION = "visual_regression"     # Detect visual changes
    CHANGE_ANALYSIS = "change_analysis"         # Analyze what changed


class ModelRouter:
    """
    Intelligent routing between visual AI models.

    All models are commercially safe:
    - DINOv2: Apache 2.0
    - V-JEPA 2: MIT + Apache 2.0
    - SigLIP: Apache 2.0
    """

    ROUTING_TABLE = {
        TaskType.COMPARE_SIMPLE: "dinov2",
        TaskType.COMPARE_SEQUENCE: "vjepa2",
        TaskType.STABILITY_CHECK: "vjepa2",      # Needs sequence understanding
        TaskType.HEALING_VALIDATION: "vjepa2",   # Needs state transition reasoning
        TaskType.EMBEDDING: "dinov2",            # Fast default
        TaskType.TEXT_SEARCH: "siglip",          # Text-image alignment
        TaskType.VISUAL_REGRESSION: "dinov2",    # Fast comparison
        TaskType.CHANGE_ANALYSIS: "vjepa2",      # Needs deeper understanding
    }

    MODEL_INFO = {
        "dinov2": {
            "name": "DINOv2-giant",
            "license": "Apache-2.0",
            "source": "facebook/dinov2-giant",
            "best_for": ["fast comparison", "embeddings", "visual regression"],
            "typical_latency_ms": 50,
            "memory_mb": 1500,
        },
        "vjepa2": {
            "name": "V-JEPA 2",
            "license": "MIT + Apache-2.0",
            "source": "facebook/vjepa2-vitg-fpc64-384",
            "best_for": ["sequences", "stability", "self-healing", "state transitions"],
            "typical_latency_ms": 150,
            "memory_mb": 3000,
        },
        "siglip": {
            "name": "SigLIP-large",
            "license": "Apache-2.0",
            "source": "google/siglip-large-patch16-384",
            "best_for": ["text-to-image search", "semantic matching"],
            "typical_latency_ms": 60,
            "memory_mb": 1500,
        },
    }

    def __init__(self, available_models: List[str]):
        self.available_models = set(available_models)
        logger.info(f"Router initialized with models: {list(available_models)}")

    def route(
        self,
        task_type: TaskType,
        explicit_model: Optional[str] = None,
        frame_count: int = 1,
        needs_high_accuracy: bool = False
    ) -> str:
        """
        Determine which model to use for a given task.

        Args:
            task_type: Type of visual AI task
            explicit_model: Explicitly requested model (overrides routing)
            frame_count: Number of frames involved
            needs_high_accuracy: Whether to prefer accuracy over speed

        Returns:
            Model name to use
        """
        # Explicit model requested
        if explicit_model and explicit_model in self.available_models:
            return explicit_model

        # Get default routing
        preferred = self.ROUTING_TABLE.get(task_type, "dinov2")

        # Fallback logic if preferred model not available
        if preferred not in self.available_models:
            # V-JEPA 2 -> DINOv2 fallback (loses sequence understanding)
            if preferred == "vjepa2" and "dinov2" in self.available_models:
                logger.warning(f"V-JEPA 2 not available, falling back to DINOv2 for {task_type}")
                return "dinov2"
            # SigLIP -> DINOv2 fallback (loses text alignment)
            if preferred == "siglip" and "dinov2" in self.available_models:
                logger.warning(f"SigLIP not available, falling back to DINOv2 for {task_type}")
                return "dinov2"
            # Use whatever is available
            if self.available_models:
                return list(self.available_models)[0]
            raise RuntimeError("No models available")

        # Special routing rules
        if frame_count > 1 and "vjepa2" in self.available_models:
            # Multi-frame tasks benefit from V-JEPA 2
            return "vjepa2"

        if needs_high_accuracy and task_type == TaskType.HEALING_VALIDATION:
            # Critical tasks use V-JEPA 2 if available
            if "vjepa2" in self.available_models:
                return "vjepa2"

        return preferred

    def get_model_info(self, model_name: Optional[str] = None) -> Dict:
        """Get info about model(s)"""
        if model_name:
            return self.MODEL_INFO.get(model_name, {})
        return {k: v for k, v in self.MODEL_INFO.items() if k in self.available_models}

    def explain_routing(self, task_type: TaskType) -> str:
        """Explain why a model was chosen for a task"""
        model = self.route(task_type)
        info = self.MODEL_INFO.get(model, {})
        return f"{task_type.value} -> {model} ({info.get('license', 'unknown')}): {', '.join(info.get('best_for', []))}"
