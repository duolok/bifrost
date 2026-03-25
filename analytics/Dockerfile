FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY src/ src/

RUN adduser --disabled-password --uid 1000 analytics
USER analytics

EXPOSE 8090

CMD ["python", "-m", "src.main"]
