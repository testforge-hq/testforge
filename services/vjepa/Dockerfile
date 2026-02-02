# V-JEPA 2 Visual Validation Service
FROM python:3.10-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    libgl1-mesa-glx \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy proto files
COPY proto/ ./proto/

# Generate protobuf Python code
RUN python -m grpc_tools.protoc \
    -I./proto \
    --python_out=./proto \
    --grpc_python_out=./proto \
    ./proto/vjepa.proto

# Copy source code
COPY src/ ./src/

# Create models directory
RUN mkdir -p /app/models/vjepa2

ENV PYTHONPATH=/app:/app/proto
ENV PYTHONUNBUFFERED=1

EXPOSE 50051

CMD ["python", "src/server.py", "--port", "50051", "--device", "cpu"]
