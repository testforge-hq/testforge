# Visual AI Service Dockerfile
# Multi-model visual understanding with DINOv2, V-JEPA 2, and SigLIP

FROM pytorch/pytorch:2.1.0-cuda12.1-cudnn8-runtime

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    libgl1-mesa-glx \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements first for caching
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements.txt

# Copy proto files
COPY proto/ proto/

# Generate Python proto files
RUN python -m grpc_tools.protoc \
    -I./proto \
    --python_out=./proto \
    --grpc_python_out=./proto \
    proto/visual_ai.proto

# Fix proto imports
RUN sed -i 's/import visual_ai_pb2/from proto import visual_ai_pb2/g' proto/visual_ai_pb2_grpc.py

# Copy source code
COPY src/ src/

# Pre-download models (optional, makes startup faster)
# Uncomment these lines to bake models into the image
# RUN python -c "import torch; torch.hub.load('facebookresearch/dinov2', 'dinov2_vitg14')"

# Environment
ENV PYTHONUNBUFFERED=1
ENV VISUAL_AI_MODELS=dinov2,vjepa2
ENV MINIO_ENDPOINT=minio:9000
ENV MINIO_ACCESS_KEY=minioadmin
ENV MINIO_SECRET_KEY=minioadmin

# Expose gRPC port
EXPOSE 50051

# Health check
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD python -c "import grpc; c=grpc.insecure_channel('localhost:50051'); grpc.channel_ready_future(c).result(timeout=5)"

# Run server
CMD ["python", "src/server.py", "--port", "50051"]
