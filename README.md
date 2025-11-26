# AI Email Box Backend

Backend API server built with Go (Golang) for AI Email Box application.

## Features

- ✅ Email/Password Authentication
- ✅ Google OAuth Authentication
- ✅ JWT Access & Refresh Tokens
- ✅ Token Refresh & Rotation
- ✅ Mock Email API (Mailboxes, Emails, Attachments)
- ✅ CORS Support
- ✅ RESTful API Design

## Tech Stack

- **Go 1.25+**
- **Gin** - Web framework
- **JWT** - Token authentication
- **Bcrypt** - Password hashing
- **Google OAuth2** - Google Sign-In

## Project Structure

```
aiemailbox-be/
├── cmd/
│   └── server/
│       └── main.go           # Application entry point
├── config/
│   └── config.go             # Configuration management
├── internal/
│   ├── handlers/
│   │   ├── auth.go           # Authentication handlers
│   │   └── email.go          # Email handlers (mock data)
│   ├── middleware/
│   │   ├── auth.go           # JWT authentication middleware
│   │   └── cors.go           # CORS middleware
│   ├── models/
│   │   ├── user.go           # User models
│   │   └── email.go          # Email models
│   └── utils/
│       ├── jwt.go            # JWT utilities
│       └── password.go       # Password hashing utilities
├── .env                      # Environment variables
├── .gitignore
└── go.mod
```

## Installation & Setup

### Prerequisites

- Go 1.25 or higher
- Git

### 1. Clone the repository

```bash
cd aiemailbox-be
```

### 2. Install dependencies

Dependencies are vendored in the `vendor/` directory. To update dependencies:

```bash
# Download and vendor dependencies
go mod vendor

# Or just download without vendoring
go mod download
```

### 3. Configure environment variables

Edit the `.env` file:

```env
PORT=8080
JWT_SECRET=your-secret-key-change-in-production
JWT_ACCESS_EXPIRATION=15m
JWT_REFRESH_EXPIRATION=7d
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
FRONTEND_URL=http://localhost:3000
```

**Important Security Notes:**
- Change `JWT_SECRET` to a strong random string in production
- Never commit `.env` file to version control
- Use environment-specific configurations for different deployments

### 4. Run the server

```bash
# Run with vendored dependencies
go run -mod=vendor cmd/server/main.go

# Or let Go automatically use vendor if present
go run cmd/server/main.go
```

The server will start on `http://localhost:8080`

**Note:** With vendored dependencies, Go will automatically use the `vendor/` directory instead of the global module cache.

## API Endpoints

### Authentication

#### Sign Up
```http
POST /api/auth/signup
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "password123",
  "name": "John Doe"
}
```

#### Login
```http
POST /api/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "password123"
}
```

#### Google OAuth
```http
POST /api/auth/google
Content-Type: application/json

{
  "token": "google-id-token"
}
```

#### Refresh Token
```http
POST /api/auth/refresh
Content-Type: application/json

{
  "refreshToken": "your-refresh-token"
}
```

#### Get Current User
```http
GET /api/auth/me
Authorization: Bearer <access-token>
```

#### Logout
```http
POST /api/auth/logout
Authorization: Bearer <access-token>
```

### Email (Protected Routes)

#### Get Mailboxes
```http
GET /api/mailboxes
Authorization: Bearer <access-token>
```

#### Get Emails for Mailbox
```http
GET /api/mailboxes/:mailboxId/emails?page=1&perPage=20
Authorization: Bearer <access-token>
```

#### Get Email Detail
```http
GET /api/emails/:emailId
Authorization: Bearer <access-token>
```

## Authentication Flow

1. **Login/Signup**: User provides credentials → Server returns access token (15min) and refresh token (7 days)
2. **Access Token**: Stored in-memory on client, sent with each API request
3. **Refresh Token**: Stored in localStorage, used to obtain new access token when expired
4. **Token Refresh**: Client automatically requests new access token using refresh token
5. **Logout**: Clears both tokens on client and server

## Token Storage Strategy

### Access Token (In-Memory)
- **Storage**: JavaScript variable (not localStorage/cookies)
- **Lifetime**: 15 minutes
- **Security**: Protected from XSS attacks, lost on page refresh (requires re-authentication or refresh)

### Refresh Token (LocalStorage)
- **Storage**: Browser localStorage
- **Lifetime**: 7 days
- **Security**: Vulnerable to XSS but allows persistent sessions
- **Mitigation**: Short access token lifetime, token rotation on refresh

### Security Considerations

**Why this approach?**
- Access tokens are short-lived and in-memory, minimizing XSS risk
- Refresh tokens enable persistent sessions without re-login
- Token rotation on refresh prevents token replay attacks
- HTTPS required in production to prevent MITM attacks

**Production Recommendations:**
1. Use HTTPS only
2. Implement token rotation (current implementation rotates refresh tokens)
3. Consider HttpOnly cookies for refresh tokens (requires server-side session handling)
4. Implement rate limiting on refresh endpoint
5. Add refresh token revocation/blacklist for compromised tokens

## Development

### Build

```bash
# Build with vendored dependencies
go build -mod=vendor -o server cmd/server/main.go

# Or without specifying (Go will auto-detect vendor)
go build -o server cmd/server/main.go
```

### Run binary

```bash
./server
```

### Run with live reload (using air)

```bash
# Install air
go install github.com/air-verse/air@latest

# Run
air
```

## Deployment

### Option 1: Build and Deploy Binary

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o server cmd/server/main.go

# Upload to server and run
./server
```

### Option 2: Docker

Create `Dockerfile`:

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o server cmd/server/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=builder /app/.env .
EXPOSE 8080
CMD ["./server"]
```

Build and run:

```bash
docker build -t aiemailbox-be .
docker run -p 8080:8080 aiemailbox-be
```

### Option 3: Cloud Platforms

- **Railway**: Connect GitHub repo, set environment variables
- **Render**: Create new Web Service from GitHub
- **Fly.io**: `fly launch` and `fly deploy`
- **Heroku**: Add `Procfile` with `web: ./server`

## Environment Variables for Production

Set these in your hosting platform:

```
PORT=8080
JWT_SECRET=<strong-random-string>
JWT_ACCESS_EXPIRATION=15m
JWT_REFRESH_EXPIRATION=168h
GOOGLE_CLIENT_ID=<your-google-oauth-client-id>
GOOGLE_CLIENT_SECRET=<your-google-oauth-secret>
FRONTEND_URL=<your-frontend-url>
```

## Testing

Test the API endpoints using curl, Postman, or Insomnia.

### Example: Sign Up
```bash
curl -X POST http://localhost:8080/api/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123","name":"Test User"}'
```

### Example: Get Mailboxes (with token)
```bash
curl -X GET http://localhost:8080/api/mailboxes \
  -H "Authorization: Bearer <your-access-token>"
```

## Dependency Management

This project uses **Go modules with vendoring** for dependency management.

### Why Vendor?

- ✅ **Reproducible builds** - Exact dependencies are committed
- ✅ **No network needed** - All dependencies are local
- ✅ **Faster CI/CD** - No download time
- ✅ **Security** - Review dependencies in version control

### Working with Dependencies

```bash
# Add a new dependency
go get github.com/some/package

# Update vendor directory
go mod vendor

# Verify vendor matches go.mod
go mod verify

# Clean up unused dependencies
go mod tidy
go mod vendor
```

### Build Modes

```bash
# Use vendor (default if vendor/ exists)
go build cmd/server/main.go

# Explicitly use vendor
go build -mod=vendor cmd/server/main.go

# Force use global cache (ignore vendor)
go build -mod=mod cmd/server/main.go

# Read-only mode
go build -mod=readonly cmd/server/main.go
```

## Mock Data

The application uses in-memory mock data for:
- Users (email/password and Google OAuth)
- Mailboxes (Inbox, Sent, Starred, Drafts, Archive, Trash)
- Emails with attachments

In production, replace with a real database (PostgreSQL, MongoDB, etc.)

## License

MIT

## Contributors

- Your Name - Initial work

## Acknowledgments

- Gin Web Framework
- JWT-Go
- Google OAuth2 API
