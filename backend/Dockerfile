FROM python:3.11-slim

WORKDIR /app

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

# ponytail: installs only what server.py imports. requirements.txt pins
# emergentintegrations (private Emergent.sh SDK, 404 on PyPI -> build fails) and
# carries unused heavy deps (pandas, numpy, boto3, jq, typer, dev tools).
# Upgrade path: switch back to `pip install -r requirements.txt` once that file
# drops emergentintegrations and adds httpx + Pillow.
RUN pip install --no-cache-dir \
        fastapi==0.110.1 \
        uvicorn==0.25.0 \
        motor==3.3.1 \
        pymongo==4.6.3 \
        pydantic==2.7.4 \
        email-validator==2.2.0 \
        pyjwt==2.10.1 \
        bcrypt==4.1.3 \
        python-dotenv==1.0.1 \
        python-multipart==0.0.9 \
        httpx==0.27.0 \
        Pillow==10.4.0

COPY server.py .

RUN useradd --create-home --uid 10001 appuser
USER appuser

EXPOSE 8000

CMD ["uvicorn", "server:app", "--host", "0.0.0.0", "--port", "8000"]
