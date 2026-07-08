FROM python:3.12-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Install uv for fast package management
COPY --from=ghcr.io/astral-sh/uv:latest /uv /uv_bin/uv
ENV PATH="/uv_bin:${PATH}"

# Copy project files and install dependencies
COPY ai-service/pyproject.toml ai-service/uv.lock ./
RUN uv pip install --system --no-cache -r pyproject.toml

# Copy all service source files (includes stale agent_pb2.py - will be regenerated below)
COPY ai-service/ .

# Copy the proto definition and regenerate pb2 files AFTER source copy,
# so the freshly-generated files overwrite the stale committed ones.
# This ensures gencode version always matches the installed runtime.
COPY shared/proto/agent.proto ./agent.proto
RUN python -m grpc_tools.protoc \
    -I. \
    --python_out=. \
    --grpc_python_out=. \
    agent.proto

EXPOSE 50051

CMD ["python", "server.py"]
