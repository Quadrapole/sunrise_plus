# Sunrise Plus

A monitor-restart service written for Sunshine Linux Hosts with **intelligent error handling**!

> **⚠️ Fork Notice**: This is a fork of [samurailink3/sunrise](https://github.com/samurailink3/sunrise) with significant improvements for handling encoder failures and zombie process issues. All original credit goes to the upstream maintainers.
>
> **Upstream**: https://github.com/samurailink3/sunrise

---

## What does this do?

The great [Sunshine](https://app.lizardbyte.dev/Sunshine/) game stream application has critical issues on Linux desktops:

1. **Monitor Sleep Issue**: When the display sleeps, Sunshine doesn't wake it up and instead errors out ([GitHub discussion](https://github.com/orgs/LizardByte/discussions/439))
2. **Encoder Failure Issue**: Sunshine sometimes fails to initialize video encoders (nvenc, vaapi, software), causing **503 errors** when trying to connect
3. **Zombie Process Issue**: When Sunshine crashes repeatedly, it can become a defunct zombie process that blocks new connections
4. **Log Corruption Issue**: Sunshine can output binary/corrupted data creating massive 4MB+ single lines, crashing log parsers

**Sunrise Plus** solves all four problems with **conditional restart logic** and **self-healing**:

| Error Type | Action | Why |
|------------|--------|-----|
| Monitor Sleep (`"Error: Couldn't find monitor"`) | Wake monitor only | Display is just asleep, no restart needed |
| Encoder Failure (`"Fatal: Unable to find display or encoder"`) | **Restart Sunshine** | Encoder initialization failed, needs full restart |
| Session Error (`"Error: Failed to create session:"`) | Wake monitor only | Display power issue, not encoder problem |
| **Log Corruption** (lines > 1MB) | **Clear log & restart** | Binary data in log crashes parser |

---

## Why Sunrise Plus?

### The Original Problem

The original Sunrise would restart Sunshine for **any** error, which caused:
- Unnecessary service restarts when the monitor was just asleep
- Accumulation of zombie processes when encoder failures occurred
- 503 errors persisting because the root cause (encoder initialization) wasn't addressed

### Our Solution

**Smart Error Detection**: We distinguish between **monitor sleep errors** (wake display) and **encoder failures** (restart service).

**Self-Healing Log Corruption**: When Sunshine outputs binary data or corrupted lines that exceed 1MB, Sunrise Plus automatically:
1. Detects the buffer overflow error
2. Clears the corrupted log file
3. Restarts Sunshine with a clean slate
4. Continues monitoring without manual intervention

**Real-world scenarios that were fixed**:

*Encoder Failure:*
```
User tries to connect via Moonlight
→ Sunshine log shows: "Fatal: Unable to find display or encoder during startup."
→ Original Sunrise: Does nothing (wrong error pattern)
→ Sunrise Plus: Detects encoder failure, restarts Sunshine
→ User can now connect!
```

*Log Corruption:*
```
Sunshine crashes and outputs 4MB of binary data on one line
→ Original Sunrise: Crashes with "token too long" error, enters restart loop
→ Sunrise Plus: Detects corrupted log, clears it, restarts Sunshine
→ Service recovers automatically, no manual fix needed!
```

---

## Installation

### Prerequisites

- Go 1.19+ (or use Docker build script)
- systemd
- ydotool or qdbus for waking the monitor
- Sunshine already installed and configured

### Build

```bash
cd /path/to/sunrise  # wherever you cloned this
go build -o sunrise .
# Or use Docker:
# ./build-with-docker.bash
```

### Install

```bash
# Create directories
sudo mkdir -p /opt/sunrise /etc/sunrise

# Copy binary
sudo cp sunrise /opt/sunrise/sunrise
sudo chmod +x /opt/sunrise/sunrise

# Copy config
sudo cp sunrise.cfg.example /etc/sunrise/sunrise.cfg

# Edit config for your system
sudo nano /etc/sunrise/sunrise.cfg
```

### Configure

Edit `/etc/sunrise/sunrise.cfg`:

```toml
SunriseCheckSeconds = 10
SunshineLogPath = "/home/YOUR_USERNAME/.config/sunshine/sunshine.log"

# Monitor sleep error - wakes monitor, does NOT restart Sunshine
MonitorIsOffLogLine = "Error: Couldn't find monitor"

# Encoder failure - restarts Sunshine automatically
EncoderFailedLogLine = "Fatal: Unable to find display or encoder during startup."

WakeMonitorSleepSeconds = 10

# ⚠️ IMPORTANT: Direct commands are deprecated! ⚠️
# Sunrise Plus now prefers systemd for proper process management.
# These are only used as fallbacks when systemd is unavailable:
StopSunshineCommand = "/usr/bin/killall sunshine"
StartSunshineCommand = "/usr/bin/sunshine"

# Wake command (choose one for your DE):
# KDE: WakeMonitorCommand = "/usr/bin/qdbus6 org.kde.Solid.PowerManagement /org/kde/Solid/PowerManagement org.kde.Solid.PowerManagement.wakeup"
# GNOME/Wayland: 
WakeMonitorCommand = "/usr/bin/ydotool mousemove -- 1 1"

# Enable encoder failure restart
RestartOnEncoderFailure = true
```

**Note on Process Management:** Sunrise Plus automatically detects and uses systemd when available. Direct commands (`StopSunshineCommand`/`StartSunshineCommand`) are only used as fallbacks. Using systemd prevents zombie process accumulation.

### Start Service

```bash
# Copy service file
cp sunrise.service $HOME/.config/systemd/user/sunrise.service

# Reload and enable
systemctl --user daemon-reload
systemctl --user enable sunrise
systemctl --user start sunrise

# Check status
systemctl --user status sunrise
journalctl --user -u sunrise -f
```

---

## Troubleshooting

### 503 Errors

If you're getting 503 errors when trying to connect:

1. Check Sunshine logs: `tail -50 ~/.config/sunshine/sunshine.log`
2. Look for encoder failures: `grep "Fatal: Unable to find display or encoder" ~/.config/sunshine/sunshine.log`
3. Check if Sunrise is running: `systemctl --user status sunrise`
4. Check Sunrise logs: `journalctl --user -u sunrise -n 20`

### Zombie Processes & Proper Process Management

**The Bug:** When restarting Sunshine using direct commands (`killall` + direct process execution), crashed processes become **zombies** (defunct processes) that block new connections and cause 503 errors.

**Why This Happens:** 
- The original Sunrise used `StartSunshineCommand = "/usr/bin/sunshine"` which creates orphaned processes
- When Sunshine crashes, these processes become zombies because no parent calls `wait()` on them
- Accumulated zombies block ports and prevent new connections

**The Fix:** Sunrise Plus now:
1. **Prefers systemd** for all start/stop operations (proper process reaping)
2. **Explicitly kills all processes** by PID read from `/proc` when systemd is unavailable
3. **Uses goroutines with `cmd.Wait()`** when direct execution is necessary
4. **Verifies processes actually terminated** before starting new ones

**If you see zombie processes:**

```bash
# Check for zombies
ps aux | grep sunshine

# If you see: [sunshine] <defunct>
# Restart via systemd (properly reaps zombies):
systemctl --user restart sunrise

# Or manually clean up:
systemctl --user stop sunrise
killall -9 sunshine  # Force kill if needed
truncate -s 0 ~/.config/sunshine/sunshine.log  # Clear logs
systemctl --user start sunshine
systemctl --user start sunrise
```

**Important:** Always use `systemctl --user restart sunshine` instead of `killall` + manual restart when possible. Systemd properly reaps child processes and prevents zombie accumulation.

### Encoder Failures on NVIDIA + Wayland

If you're using NVIDIA GPU with Wayland and getting encoder failures:

1. **Switch to X11**: Log out, select X11 at login screen
2. **Or use dummy HDMI plug**: Keeps display active
3. **Or disable display sleep**: In your DE power settings

### Log File Too Large / Corrupted

If Sunrise crashes with "token too long" or the log file grows to multiple MB:

**Sunrise Plus handles this automatically** with self-healing, but you can also manually clean:

```bash
# Clear the log immediately
truncate -s 0 ~/.config/sunshine/sunshine.log

# Restart services
systemctl --user restart sunshine sunrise
```

**Preventive maintenance**: Set up daily log cleanup (included in repo):

```bash
# Enable daily log cleanup at 3am
systemctl --user enable clean-sunshine-log.timer
systemctl --user start clean-sunshine-log.timer
```

---

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `SunriseCheckSeconds` | 10 | How often to check logs (seconds) |
| `SunshineLogPath` | `""` | Path to sunshine.log |
| `MonitorIsOffLogLine` | `""` | Pattern for monitor sleep errors |
| `EncoderFailedLogLine` | `""` | Pattern for encoder failures |
| `WakeMonitorSleepSeconds` | 10 | Wait time after waking monitor |
| `WakeMonitorCommand` | `""` | Command to wake display |
| `StopSunshineCommand` | `""` | Command to stop Sunshine |
| `StartSunshineCommand` | `""` | Command to start Sunshine |
| `RestartOnEncoderFailure` | false | Enable encoder failure restart |

---

## How It Works

```
Sunrise Plus runs every N seconds
       ↓
Check Sunshine logs
       ↓
┌─────────────────┬─────────────────┐
↓                 ↓                 ↓
Monitor Sleep   Encoder Failure   Nothing
"Couldn't       "Unable to find    detected
find monitor"   display or encoder"
       ↓                 ↓
Wake monitor    Restart Sunshine
       ↓                 ↓
Continue        Wait & continue
```

---

## Technical Details

### Changes from Original Sunrise

1. **Split error handling**: Separate functions for `isMonitorSleeping()` and `isEncoderFailed()`
2. **New config options**: `EncoderFailedLogLine` and `RestartOnEncoderFailure`
3. **Independent tracking**: `lastEncoderFailureTime` separate from `lastMonitorMissingTime`
4. **Better logging**: Shows which patterns are being monitored
5. **Conditional logic**: Monitor wake only for sleep errors, restart only for encoder failures
6. **Self-healing log corruption**: Detects buffer overflow, clears log, restarts Sunshine
7. **Increased buffer size**: 1MB (up from 64KB) to handle larger lines before triggering self-healing
8. **Daily log cleanup**: Systemd timer to prevent log growth (optional)
9. **Proper process management**: 8-step restart sequence with zombie reaping and systemd integration

### Why This Matters

The NVIDIA 5090 + Wayland combination (and other modern GPUs) can experience encoder initialization failures that require a full Sunshine restart. Simply waking the monitor doesn't help because the encoder never initialized in the first place.

Sunrise Plus detects this specific failure mode and handles it appropriately.

### Self-Healing Mechanism

**The Problem:**
When Sunshine crashes or encounters certain DRM/KMS errors, it can output binary data or repetitive error messages that create single log lines exceeding 64KB (sometimes 4MB+). Go's `bufio.Scanner` has a default 64KB line limit, causing it to fail with "token too long".

**The Solution:**
1. **Buffer Increase**: Scanner buffer increased to 1MB to handle moderately long lines
2. **Overflow Detection**: When scanner returns "token too long" error, it's caught by `isBufferOverflow()`
3. **Automatic Recovery**: `handleCorruptedLog()` truncates the log and restarts Sunshine
4. **No Crash Loop**: Instead of systemd restarting a crashing Sunrise 365+ times, Sunrise fixes itself and continues

**Result**: Unattended reliability - even if Sunshine outputs garbage, Sunrise Plus recovers automatically.

### Process Management & Zombie Prevention

**The Problem:**
The original Sunrise used `killall sunshine` followed by direct process execution (`sunshine &`). When Sunshine crashed:
1. The killed process became a zombie (defunct) because no parent called `wait()`
2. Repeated crashes accumulated zombie processes
3. Zombies blocked network ports and prevented new connections
4. Users got persistent 503 errors

**The Solution:**
Sunrise Plus implements an 8-step restart sequence:

```
restartSunshine()
├── 1. Stop via systemd (preferred) or killall (fallback)
├── 2. Wait 3s for graceful shutdown
├── 3. Kill all processes by PID (reads /proc directly)
├── 4. Wait 5s for termination
├── 5. Verify no processes remain (force kill if needed)
├── 6. Clear logs for fresh start
├── 7. Start via systemd (preferred) or direct command
└── 8. Verify sunshine actually started
```

**Key improvements:**
- **Systemd integration**: Automatically detects and uses `systemctl --user stop/start sunshine` when available
- **PID-based killing**: Reads `/proc/[pid]/comm` to find and kill every sunshine process including zombies
- **Graduated kill strategy**: SIGTERM first, then SIGKILL for stragglers
- **Async reaping**: When using direct commands, uses goroutine with `cmd.Wait()` to prevent zombies
- **Verification**: Actually confirms processes terminated before starting new ones

**Why systemd matters:**
- Systemd tracks processes via cgroups, not just PIDs
- When a service stops, systemd reaps all child processes automatically
- No zombie accumulation even with repeated crashes
- Better logging via `journalctl --user -u sunshine`

---

## Credits

- **Original Sunrise**: [samurailink3](https://github.com/samurailink3/sunrise) - The foundation that made this possible
- **Sunrise Plus Improvements**: 
  - **Quadrapole** (@quadrapole) - Real-world testing, debugging, and feature requirements
  - **Kimi K2.5** (via OhMyOpenCode Sisyphus agent) - Code architecture, implementation, and diagnostics

This collaboration solved a complex multi-layered issue involving:
- Systemd service management
- Process zombie reaping
- Log parsing and error classification  
- NVIDIA/Wayland compatibility quirks
- Moonlight/Sunshine protocol debugging

---

## Contributing

This is working for:
- Arch Linux + NVIDIA RTX 5090 + Wayland + KDE
- Debian Trixie + KDE
- GNOME Wayland

If you have other working configurations, please share your `WakeMonitorCommand`!

---

## License

Same as original Sunrise - see LICENSE file.
