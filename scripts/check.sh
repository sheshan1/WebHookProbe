#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
: "${JAVA_HOME:?Set JAVA_HOME to a Java 25 JDK}"
export PATH="$JAVA_HOME/bin:$PATH"
(cd receiver && ./mvnw verify)
(cd runner && test -z "$(gofmt -l .)" && go vet ./... && go test -race -timeout 60s ./...)
(cd frontend && npm run build && npm run lint)
echo 'Automated source checks passed. RUN_RECEIVER_IT=true enables PostgreSQL tests.'
