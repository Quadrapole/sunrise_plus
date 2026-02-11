# Sunrise Plus

A monitor-restart service written for Sunshine Linux Hosts with **intelligent error handling**!

> **⚠️ Fork Notice**: This is a fork of [samurailink3/sunrise](https://github.com/samurailink3/sunrise) with significant improvements for handling encoder failures and monitor wake functionality. All original credit goes to the upstream maintainers.
>
> **Upstream**: https://github.com/samurailink3/sunrise

---

## What does this do?

The great [Sunshine](https://app.lizardbyte.dev/Sunshine/) game stream application has a critical issue on Linux desktops: When the display sleeps, Sunshine doesn't wake it up and instead errors out (see [this GitHub discussion](https://github.com/orgs/LizardByte/discussions/439)). This makes using Sunshine with Linux-on-the-Desktop a frustrating experience.

**Sunrise Plus** solves these problems with **conditional restart logic**:

| Error Type | Action | Why |
|------------|--------|-----|
| Monitor Sleep (`"Error: Couldn't find monitor"`) | Wake monitor only | Display is just asleep, no restart needed |
| Encoder Failure (`"Fatal: Unable to find display or encoder"`) | **Restart Sunshine** | Encoder initialization failed, needs full restart |
| Session Error (`"Error: Failed to create session:"`) | Wake monitor only | Display power issue, not encoder problem |

---

## Improvements Over Original Sunrise

### 1. Conditional Restart Logic

**Original Sunrise:** Restarted Sunshine for **any** error

**Sunrise Plus:** Smart error detection
- Monitor sleep errors → Wake display only (no restart)
- Encoder failures → Restart Sunshine service
- No more unnecessary service restarts

### 2. Multiple Encoder Error Patterns

Added support for multiple encoder failure patterns:
- `EncoderFailedLogLine` - Primary pattern
- `EncoderFailedLogLine2` - Secondary pattern for additional coverage

### 3. Enhanced Process Management

- Prefers **systemd** for all start/stop operations
- Proper process lifecycle management
- Prevents zombie process accumulation

### 4. Improved Monitor Wake

- KDE native power management support via `qdbus6`
- More reliable than generic mouse movement
- Better compatibility with Wayland

### 5. Self-Healing Log Handling

- Increased scanner buffer to 1MB
- Automatic detection and recovery from corrupted log entries
- Service continues monitoring without manual intervention

---

## Installation

### Prerequisites

- Go 1.19+ (or use Docker build script)
- systemd
- qdbus or ydotool for waking the monitor
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

# Encoder failure patterns
EncoderFailedLogLine = "Fatal: Unable to find display or encoder during startup."
EncoderFailedLogLine2 = "Error: Video failed to find working encoder"

WakeMonitorSleepSeconds = 10

# Wake command (choose one for your DE):
# KDE (Recommended): Uses native power management
WakeMonitorCommand = "/usr/bin/qdbus6 org.kde.Solid.PowerManagement /org/kde/Solid/PowerManagement org.kde.Solid.PowerManagement.wakeup"

# Alternative for GNOME/Wayland:
# WakeMonitorCommand = "/usr/bin/ydotool mousemove -- 1 1 && /usr/bin/ydotool mousemove -- -1 -1"

# Enable encoder failure restart
RestartOnEncoderFailure = true
```

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

### Encoder Failures on NVIDIA + Wayland

If you're using NVIDIA GPU with Wayland and getting encoder failures:

1. **Switch to X11**: Log out, select X11 at login screen
2. **Or use dummy HDMI plug**: Keeps display active
3. **Or disable display sleep**: In your DE power settings

### Monitor Not Waking

If your monitor doesn't wake when connecting:

1. Try the KDE native command manually: `qdbus6 org.kde.Solid.PowerManagement /org/kde/Solid/PowerManagement wakeup`
2. If that works, make sure it's set in your config
3. For GNOME, try: `gdbus call --session --dest org.gnome.SettingsDaemon.Power --object-path /org/gnome/SettingsDaemon/Power --method org.gnome.SettingsDaemon.Power.Wake`

---

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `SunriseCheckSeconds` | 10 | How often to check logs (seconds) |
| `SunshineLogPath` | `""` | Path to sunshine.log |
| `MonitorIsOffLogLine` | `""` | Pattern for monitor sleep errors |
| `EncoderFailedLogLine` | `""` | Pattern for encoder failures (primary) |
| `EncoderFailedLogLine2` | `""` | Pattern for encoder failures (secondary) |
| `WakeMonitorSleepSeconds` | 10 | Wait time after waking monitor |
| `WakeMonitorCommand` | `""` | Command to wake display |
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
2. **Multiple encoder patterns**: `EncoderFailedLogLine` and `EncoderFailedLogLine2`
3. **Independent tracking**: `lastEncoderFailureTime` separate from `lastMonitorMissingTime`
4. **Better logging**: Shows which patterns are being monitored
5. **Conditional logic**: Monitor wake only for sleep errors, restart only for encoder failures
6. **Enhanced buffer**: 1MB scanner buffer for handling larger log lines
7. **Systemd integration**: Prefers systemd for process management
8. **Self-healing**: Automatic recovery from log corruption

### Why This Matters

The NVIDIA 5090 + Wayland combination (and other modern GPUs) can experience encoder initialization failures that require a full Sunshine restart. Simply waking the monitor doesn't help because the encoder never initialized in the first place.

Sunrise Plus detects this specific failure mode and handles it appropriately with conditional logic - wake the display for sleep issues, restart the service for encoder failures.

---

## Credits

- **Original Sunrise**: [samurailink3](https://github.com/samurailink3/sunrise) - The foundation that made this possible

This fork adds conditional restart logic, multiple encoder error patterns, enhanced process management, and improved monitor wake functionality.

---

## Contributing

Tested on:
- Arch Linux + NVIDIA GPU + Wayland + KDE
- Debian Trixie + KDE
- GNOME Wayland

If you have other working configurations, please share your `WakeMonitorCommand`!

---

## License

Same as original Sunrise - see LICENSE file.
