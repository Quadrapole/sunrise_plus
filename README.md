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

**Sunrise Plus** solves all three problems with **conditional restart logic**:

| Error Type | Action | Why |
|------------|--------|-----|
| Monitor Sleep (`"Error: Couldn't find monitor"`) | Wake monitor only | Display is just asleep, no restart needed |
| Encoder Failure (`"Fatal: Unable to find display or encoder"`) | **Restart Sunshine** | Encoder initialization failed, needs full restart |
| Session Error (`"Error: Failed to create session:"`) | Wake monitor only | Display power issue, not encoder problem |

---

## Why Sunrise Plus?

### The Original Problem

The original Sunrise would restart Sunshine for **any** error, which caused:
- Unnecessary service restarts when the monitor was just asleep
- Accumulation of zombie processes when encoder failures occurred
- 503 errors persisting because the root cause (encoder initialization) wasn't addressed

### Our Solution

**Smart Error Detection**: We distinguish between **monitor sleep errors** (wake display) and **encoder failures** (restart service).

**Real-world scenario that was fixed**:
```
User tries to connect via Moonlight
→ Sunshine log shows: "Fatal: Unable to find display or encoder during startup."
→ Original Sunrise: Does nothing (wrong error pattern)
→ Sunrise Plus: Detects encoder failure, restarts Sunshine
→ User can now connect!
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
StopSunshineCommand = "/usr/bin/killall sunshine"
StartSunshineCommand = "/usr/bin/sunshine"

# Wake command (choose one for your DE):
# KDE: WakeMonitorCommand = "/usr/bin/qdbus6 org.kde.Solid.PowerManagement /org/kde/Solid/PowerManagement org.kde.Solid.PowerManagement.wakeup"
# GNOME/Wayland: 
WakeMonitorCommand = "/usr/bin/ydotool mousemove -- 1 1"

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

### Zombie Processes

If Sunshine becomes a zombie (defunct process):

```bash
# Check for zombies
ps aux | grep sunshine

# If you see: [sunshine] <defunct>
# Stop sunrise (which reaps the zombie), then restart:
systemctl --user stop sunrise
killall -9 sunshine  # Force kill if needed
sunshine &  # Start manually or via sunrise
systemctl --user start sunrise
```

### Encoder Failures on NVIDIA + Wayland

If you're using NVIDIA GPU with Wayland and getting encoder failures:

1. **Switch to X11**: Log out, select X11 at login screen
2. **Or use dummy HDMI plug**: Keeps display active
3. **Or disable display sleep**: In your DE power settings

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

### Why This Matters

The NVIDIA 5090 + Wayland combination (and other modern GPUs) can experience encoder initialization failures that require a full Sunshine restart. Simply waking the monitor doesn't help because the encoder never initialized in the first place.

Sunrise Plus detects this specific failure mode and handles it appropriately.

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
