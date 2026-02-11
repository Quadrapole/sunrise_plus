# AGENTS.md - Sunrise Plus

## Build Commands

```bash
# Build binary
go build -o sunrise .

# Build with Docker
./build-with-docker.bash

# Format code
gofmt -w main.go

# Lint (VS Code uses golangci-lint-v2)
golangci-lint run

# Vet
go vet ./...
```

## Testing

**No tests currently exist.** To add and run tests:

```bash
# Create test file
touch main_test.go

# Run all tests
go test ./...

# Run single test by name
go test -run TestFunctionName

# Run with verbose output
go test -v ./...

# Run with coverage report
go test -cover ./...

# Run tests matching pattern
go test -run "Test.*Monitor"

# Run benchmarks
go test -bench=.

# Run with race detector
go test -race ./...
```

Test files follow `*_test.go` naming convention.
Example test structure:
```go
func TestIsMonitorSleeping(t *testing.T) {
    // Setup
    c.SunshineLogPath = "/tmp/test.log"
    
    // Test
    result, err := isMonitorSleeping()
    
    // Assert
    if err != nil {
        t.Errorf("Expected no error, got %v", err)
    }
    if result != false {
        t.Errorf("Expected false, got %v", result)
    }
}
```

## Project Structure

```
sunrise/
├── main.go              # Main service logic (~465 lines)
│                        # - Ticker-based log monitoring
│                        # - Zombie process reaping
│                        # - Systemd integration
│                        # - 8-step restart sequence
├── go.mod               # Go module (go 1.24.4)
├── go.sum               # Dependency checksums
├── sunrise.cfg.example  # Config template (TOML format)
├── sunrise.service      # systemd user service file
├── build-with-docker.bash
├── README.md            # User documentation
├── LICENSE              # Project license
└── sunrise              # Compiled binary (gitignored)
```

## Code Style

### Imports
- Standard library first, blank line, then external deps
- Group: stdlib → external → local
- Use `goimports` or `gofmt` for formatting
- Avoid unused imports

```go
import (
    "bufio"
    "bytes"
    "fmt"
    "os"
    "os/exec"
    "strconv"
    "strings"
    "time"

    "github.com/BurntSushi/toml"
)
```

### Naming Conventions
- `camelCase` for unexported variables/functions
- `PascalCase` for exported variables/functions
- Acronyms: `PID`, `URL`, `HTTP`, `TOML` (all caps)
- Config struct fields use PascalCase with TOML tags
- Boolean variables: use `is` or `has` prefix (e.g., `isSleeping`, `hasFailed`)

```go
type config struct {
    SunriseCheckSeconds     int
    SunshineLogPath         string
    RestartOnEncoderFailure bool
}

var lastMonitorMissingTime time.Time
```

### Error Handling
- Always check errors: `if err != nil`
- Return errors with context using `fmt.Errorf("context: %w", err)`
- Fatal only in `main()`: `log.Fatal()`
- Log warnings but continue for recoverable errors
- Never ignore errors silently

```go
// Good
if err != nil {
    log.Println("Warning: could not wake monitor:", err)
    return err
}

// Better - with context
if err != nil {
    return fmt.Errorf("failed to start sunshine: %w", err)
}
```

### Functions
- Short, focused functions (< 50 lines when possible)
- Named returns for documentation: `(isSleeping bool, err error)`
- Comment all exported functions with purpose
- Group related functions together
- Avoid deep nesting - early returns preferred

```go
// isMonitorSleeping checks for monitor sleep errors in logs
func isMonitorSleeping() (isSleeping bool, err error) {
    // Implementation
}
```

### Logging
- Use `log.Println()` for info, `log.Printf()` for formatted
- Start logs with lowercase (except proper nouns/acronyms)
- Log before actions, log results after
- Use structured logging approach for key events

```go
log.Println("Starting sunshine...")
// ... do work ...
log.Println("Sunshine started successfully")

// For errors
log.Printf("Warning: SIGTERM to PID %d failed: %v", pid, err)
```

### Configuration
- TOML format in `/etc/sunrise/sunrise.cfg`
- Struct fields must be exported (PascalCase) for TOML decoding
- Provide sensible defaults in code
- Check for empty required values at startup
- Document all config options in README

```toml
SunriseCheckSeconds = 10
SunshineLogPath = "/home/USER/.config/sunshine/sunshine.log"
RestartOnEncoderFailure = true
```

## Dependencies

Only one external dependency:
- `github.com/BurntSushi/toml v1.5.0` - TOML configuration parsing

Standard library packages used:
- `bufio` - Buffered log file reading
- `bytes` - Buffer for command output
- `flag` - Command-line flag parsing
- `fmt` - Formatting and error wrapping
- `log` - Logging
- `os` - File operations, process management
- `os/exec` - External command execution
- `strconv` - String to int conversion
- `strings` - String manipulation
- `time` - Timing and timestamps

## Deployment

Binary location: `/opt/sunrise/sunrise`
Config location: `/etc/sunrise/sunrise.cfg`

Install steps:
```bash
# Build
go build -o sunrise .

# Install binary
sudo mkdir -p /opt/sunrise
sudo cp sunrise /opt/sunrise/sunrise
sudo chmod +x /opt/sunrise/sunrise

# Install config
sudo mkdir -p /etc/sunrise
sudo cp sunrise.cfg.example /etc/sunrise/sunrise.cfg
sudo nano /etc/sunrise/sunrise.cfg  # Edit for your system

# Install systemd service
cp sunrise.service $HOME/.config/systemd/user/sunrise.service
systemctl --user daemon-reload
systemctl --user enable sunrise
systemctl --user start sunrise

# Monitor logs
journalctl --user -u sunrise -f
```

## Key Patterns & Architecture

### 1. Ticker-Based Monitoring
```go
ticker := time.NewTicker(time.Duration(c.SunriseCheckSeconds) * time.Second)
for {
    <-ticker.C
    // Check logs for errors
}
```

### 2. Process Management
- Read `/proc/[pid]/comm` directly for PID discovery
- Graduated kill strategy: SIGTERM → wait → SIGKILL
- Verify processes actually terminated before continuing

### 3. Zombie Prevention
When using direct command execution (not systemd):
```go
cmd := exec.Command(path, args...)
if err := cmd.Start(); err != nil {
    return err
}

// Detach and reap properly
go func() {
    if err := cmd.Wait(); err != nil {
        log.Printf("Process exited with error: %v", err)
    }
}()
```

### 4. Graceful Shutdown
```go
// 1. SIGTERM for graceful shutdown
killProcess(pid, 15)
time.Sleep(2 * time.Second)

// 2. SIGKILL for force kill if still running
remainingPids := getSunshinePIDs()
if len(remainingPids) > 0 {
    for _, pid := range remainingPids {
        killProcess(pid, 9)
    }
}
```

### 5. Log Scanning
Use `bufio.Scanner` with increased buffer for handling large log lines:
```go
scanner := bufio.NewScanner(logFile)
scanner.Buffer(make([]byte, 1024), 1024*1024) // 1MB buffer
for scanner.Scan() {
    line := scanner.Text()
    // Process line
}
```

### 6. Systemd Integration
Always prefer systemd when available:
```go
func systemdAvailable() bool {
    cmd := exec.Command("systemctl", "--user", "is-system-running")
    // Check if running or degraded
}
```

## Common Tasks

### Adding a New Error Pattern
1. Add config field to `config` struct
2. Add pattern check in appropriate function (`isMonitorSleeping` or `isEncoderFailed`)
3. Update README with new config option
4. Add example to `sunrise.cfg.example`

### Adding Process Management Features
1. Update `restartSunshine()` sequence
2. Ensure proper cleanup in all error paths
3. Add verification steps
4. Update systemd fallback logic

### Debugging Tips
```bash
# Check for zombie processes
ps aux | grep sunshine

# View sunshine logs
tail -f ~/.config/sunshine/sunshine.log

# View sunrise logs
journalctl --user -u sunrise -f

# Test config parsing
go run main.go -config /etc/sunrise/sunrise.cfg
```

## Git Workflow

```bash
# Make changes
gofmt -w main.go
go vet ./...
go build -o sunrise .

# Test manually
systemctl --user restart sunrise

# Commit
git add main.go
git commit -m "feat: description of change"
git push origin main
```

## VS Code Settings

The project includes `.vscode/settings.json`:
```json
{
    "go.lintTool": "golangci-lint-v2"
}
```

## Important Notes

- **No tests exist yet** - any new features should include tests
- **Systemd is preferred** over direct commands for proper process reaping
- **8-step restart sequence** ensures clean process management
- **Log file can grow large** - scanner buffer increased to 1MB
- **Process zombies** were the main issue with original implementation
