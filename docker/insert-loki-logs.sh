#!/bin/bash

# Default Loki URL
LOKI_URL=${LOKI_URL:-"http://localhost:3100"}
LOKI_PUSH_URL="$LOKI_URL/loki/api/v1/push"

# Default values
NUM_LOGS=10
JOB_NAME="dummy-logs"
APP_NAME="test-app"
ENVIRONMENT="dev"
INTERVAL=1  # seconds between log entries

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --num|-n)
      NUM_LOGS="$2"
      shift 2
      ;;
    --job|-j)
      JOB_NAME="$2"
      shift 2
      ;;
    --app|-a)
      APP_NAME="$2"
      shift 2
      ;;
    --env|-e)
      ENVIRONMENT="$2"
      shift 2
      ;;
    --interval|-i)
      INTERVAL="$2"
      shift 2
      ;;
    --help|-h)
      echo "Usage: $0 [OPTIONS]"
      echo "Insert dummy logs into Loki for testing"
      echo ""
      echo "Options:"
      echo "  --num, -n NUMBER       Number of logs to insert (default: 10)"
      echo "  --job, -j JOB_NAME     Job name for logs (default: dummy-logs)"
      echo "  --app, -a APP_NAME     Application name (default: test-app)"
      echo "  --env, -e ENVIRONMENT  Environment (default: dev)"
      echo "  --interval, -i SECONDS Seconds between log entries (default: 1)"
      echo "  --help, -h             Show this help message"
      echo ""
      echo "Environment Variables:"
      echo "  LOKI_URL               Loki server URL (default: http://localhost:3100)"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

echo "Inserting $NUM_LOGS dummy log entries into Loki at $LOKI_URL"
echo "Job: $JOB_NAME, App: $APP_NAME, Environment: $ENVIRONMENT"

# Check if Loki is accessible
echo "Testing connection to Loki..."
if ! curl -s --connect-timeout 5 "${LOKI_URL}/ready" > /dev/null; then
  echo "ERROR: Cannot connect to Loki at ${LOKI_URL}"
  echo "Make sure Loki is running and accessible. If running with Docker Compose, ensure containers are up."
  echo "Try: docker-compose up -d"
  exit 1
fi
echo "Loki connection successful!"

# Log levels to cycle through
log_levels=("INFO" "DEBUG" "WARN" "ERROR")

# Generate and insert logs
for ((i=1; i<=NUM_LOGS; i++)); do
  # Format timestamp in nanoseconds since epoch (Loki expects timestamp in nanoseconds)
  timestamp=$(date +%s)000000000
  
  # Select a log level (cycle through the levels)
  level_index=$((i % ${#log_levels[@]}))
  level="${log_levels[$level_index]}"
  
  # Generate a random log message
  case "$level" in
    "INFO")
      message="User authentication successful for user-$i"
      ;;
    "DEBUG")
      message="Processing request #$i with parameters: {id: $i, action: 'test'}"
      ;;
    "WARN")
      message="Slow database query detected, took $((RANDOM % 5000 + 1000))ms"
      ;;
    "ERROR")
      message="Failed to connect to external service after 3 retries"
      ;;
  esac
  
  # Create the JSON payload for Loki
  json_payload=$(cat <<EOF
{
  "streams": [
    {
      "stream": {
        "job": "$JOB_NAME",
        "app": "$APP_NAME",
        "environment": "$ENVIRONMENT",
        "level": "$level"
      },
      "values": [
        ["$timestamp", "$level: $message"]
      ]
    }
  ]
}
EOF
)

  # Send the log to Loki with verbose output for debugging
  response=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d "$json_payload" \
    "$LOKI_PUSH_URL" \
    --write-out "\nHTTP_CODE:%{http_code}")
  
  http_code=$(echo "$response" | grep "HTTP_CODE" | cut -d':' -f2)
  response_body=$(echo "$response" | grep -v "HTTP_CODE")
  
  if [[ "$http_code" == "204" || "$http_code" == "200" ]]; then
    echo "[$i/$NUM_LOGS] Successfully inserted $level log"
  else
    echo "[$i/$NUM_LOGS] Failed to insert log (HTTP $http_code): $response_body"
    if [[ "$i" == "1" ]]; then
      echo "Troubleshooting tips:"
      echo "1. Check if Loki is running: docker ps | grep loki"
      echo "2. If using Docker Compose, Loki might be at http://loki:3100 instead of localhost"
      echo "3. Try with: LOKI_URL=http://loki:3100 ./insert-loki-logs.sh"
      echo "4. Or if using Docker networking: LOKI_URL=http://host.docker.internal:3100 ./insert-loki-logs.sh"
    fi
  fi
  
  # Wait the specified interval before sending the next log
  if [[ $i -lt $NUM_LOGS ]]; then
    sleep $INTERVAL
  fi
done

echo "Finished inserting logs into Loki"
echo ""
echo "Query examples:"
echo "  ./test-loki-query.sh '{job=\"$JOB_NAME\"}' -15m now 100"
echo "  ./test-loki-query.sh '{app=\"$APP_NAME\",level=\"ERROR\"}' -15m now 100"
echo "  ./test-loki-query.sh '{environment=\"$ENVIRONMENT\"}' -15m now 100" 