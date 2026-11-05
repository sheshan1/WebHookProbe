FROM python:3.13-alpine
WORKDIR /app
COPY sample_receiver.py fixture.py
USER 65534:65534
CMD ["python", "-u", "/app/fixture.py", "--bind", "0.0.0.0"]
