# Backend + web UI for the invoice/receipt extractor.
#
# It talks to Ollama over HTTP. Run Ollama on the HOST so it can use the GPU
# (Docker cannot reach the host GPU on Windows/macOS) and pull the models first.
# Build context is this directory (python/).
FROM python:3.12-slim

WORKDIR /srv

# PyMuPDF + Pillow ship manylinux wheels, so no extra system packages are needed.
# Editable install keeps the source in place so app/main.py can find web/index.html.
COPY pyproject.toml README.md ./
COPY app ./app
COPY web ./web
RUN pip install --no-cache-dir -e .

# Sensible defaults; override in docker-compose.yml or with `docker run -e`.
ENV OLLAMA_HOST=http://host.docker.internal:11434 \
    OLLAMA_TIMEOUT_S=600 \
    OLLAMA_NUM_CTX=8192 \
    TEXT_PATH_MODEL=qwen2.5vl:3b-q4_K_M \
    VISION_PATH_MODEL=qwen2.5vl:3b-q4_K_M

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8000/health').status==200 else 1)"

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
