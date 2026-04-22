# GroupFDN CLI

A command-line interface for interacting with the Group FDN API.

## Installation

```bash
go build -o groupfdn
```

Or install to your Go bin:
```bash
go install github.com/9d4/groupfdn@latest
```

## Configuration

Tokens are stored in XDG config directory:
- Linux: `~/.config/groupfdn/config.json`
- macOS: `~/Library/Application Support/groupfdn/config.json`

## Usage

```bash
# Using groupfdn
groupfdn [command] [flags]

# Or using the fdn alias (when installed)
fdn [command] [flags]
```

### Global Flags

- `-f, --format string` - Output format: `table`, `simple`, or `json` (default: `table`)

### Authentication (OTP-based Login)

The login process uses two-factor authentication with OTP:

```bash
# Login (interactive mode)
$ groupfdn auth login
Email: user@example.com
Password: ********
Requesting OTP...
OTP code has been sent to your email
OTP code: 123456
Login successful

# With flags (for scripting)
$ groupfdn auth login -e user@example.com -p password
Requesting OTP...
OTP code has been sent to your email
OTP code: 123456
Login successful
```

## Features

- **Authentication**: OTP-based two-factor login with hidden password input, auto token refresh on 401
- **Token Storage**: Securely stored in XDG config directory
- **Output Formats**: Table, simple (TSV), and JSON
- **Attendance Management**: Check-in/out, daily activities, reports
- **Pagination**: Explicit page control for list commands
- **Rate Limiting**: Handles API rate limits with proper error messages

## API Endpoints

Base URL: `https://group.fdn.my.id/api`

The CLI implements the following endpoint groups:
- **Auth**: `/auth/login`, `/auth/logout`, `/auth/me`, `/auth/refresh`, `/auth/otp/send-otp`, `/auth/otp/verify-otp`
- **Attendance**: All attendance endpoints including checkin/out, activities, and reports

## Development

```bash
# Run tests
go test ./...

# Build
go build -o groupfdn
```
