
# Email Tracker Service

A Go service for sending and tracking emails using Redis for storage and Gin for the web framework.

## Features

- Send emails to multiple recipients (TO, CC, BCC)
- Track email opens using pixel tracking
- Custom HTML templates with variable substitution
- Redis-based tracking storage
- Configurable tracking expiration

## Setup

### Prerequisites

- Go 1.16+
- Redis server
- SMTP server access

### Environment Variables

```bash
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=your-username
SMTP_PASSWORD=your-password
REDIS_ADDR=localhost:6379
TRACKING_DOMAIN=https://your-domain.com
TRACKING_ID_EXPIRATION=86400  # 1 day in seconds
PORT=8080  # Optional, defaults to 8080
```

### Installation

```bash
go mod download
go run main.go
```

## API Endpoints

### 1. Send Email

`POST /send`

Sends emails to multiple recipients with optional tracking.

#### Request Body:

```json
{
  "recipients": {
    "receivers": [
      {
        "email": "test@example.com",
        "tracking_id": "",
        "want_to_track": true,
        "type": "to"  // "to", "cc", or "bcc"
      }
    ],
    "from": "sender@yourdomain.com"
  },
  "email_body": {
    "html_template": "<!DOCTYPE html><html><body>Hello {{ name }}!</body></html>",
    "subject": "Test Email",
    "parameters": {
      "test@example.com": {
        "name": "John Doe"
      }
    }
  }
}
```

#### Response:

```json
{
  "status": {
    "test@example.com": "Success:tracking_id:12345-uuid"
  }
}
```

### 2. Track Email Opens

`GET /pixel/:tracking_id`

Endpoint for the tracking pixel. Returns a 1x1 transparent PNG and logs the email open.

#### Response:

- Returns a transparent 1x1 pixel image
- Status: 200 OK if tracking ID exists, 404 if not found

### 3. Check Tracking Status

`GET /status/:tracking_id`

Get the current status of an email tracking ID.

#### Response:

```json
{
  "tracking_id": "12345-uuid",
  "email": "recipient@example.com",
  "count": 2,
  "last_opened": "2025-01-18T15:04:05Z",
  "created_at": "2025-01-18T10:00:00Z"
}
```

### 4. Health Check

`GET /ping`

Simple health check endpoint.

#### Response:

```json
{
  "message": "pong"
}
```

## Template Variables

The HTML template supports variable substitution using the `{{ variable_name }}` syntax. Variables are defined per recipient in the `parameters` field of the request.

## Error Handling

- All endpoints return appropriate HTTP status codes
- Detailed error messages are included in the response
- Tracking IDs expire after the configured duration (default: 1 day)

## Examples

### Sending a Test Email

```bash
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{
    "recipients": {
      "receivers": [
        {
          "email": "test@example.com",
          "tracking_id": "",
          "want_to_track": true,
          "type": "to"
        }
      ],
      "from": "sender@yourdomain.com"
    },
    "email_body": {
      "html_template": "<!DOCTYPE html><html><body><h1>Hello {{ name }}!</h1></body></html>",
      "subject": "Test Email",
      "parameters": {
        "test@example.com": {
          "name": "John Doe"
        }
      }
    }
  }'
```

### Checking Email Status

```bash
curl http://localhost:8080/status/your-tracking-id
```

## Security Considerations

- SMTP credentials should be kept secure
- Consider implementing rate limiting for tracking endpoints
- Add authentication for status endpoint in production
- Use HTTPS in production environments
