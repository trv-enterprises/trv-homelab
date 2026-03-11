FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY bridge.py .

ENTRYPOINT ["python3", "bridge.py"]
CMD ["/etc/caseta-bridge/config.yaml"]
