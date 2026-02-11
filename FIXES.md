# Major Fixes and Improvements

This document details the critical issues discovered and fixed in Sunrise Plus.

## Issue 1: Zombie Process Accumulation (CRITICAL)

**Problem:** The original Sunrise used `killall sunshine` followed by direct process execution (`sunshine &`). When Sunshine crashed:
- The killed process became a zombie (defunct) because no parent called `wait()`
- Repeated crashes accumulated zombie processes
- Zombies blocked network ports and prevented new connections
- Users got persistent 503 errors

**Impact:** Users had to physically access their PC to restart services.

**Solution:** Implemented 8-step restart sequence with proper process management:

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

**Key Improvements:**
- `systemdAvailable()` - Auto-detects systemd and uses `systemctl` commands
- `getSunshinePIDs()` - Reads `/proc/[pid]/comm` to find ALL processes including zombies
- `killAllSunshineProcesses()` - Graduated kill: SIGTERM first, then SIGKILL
- `startSunshineProperly()` - Uses goroutine with `cmd.Wait()` to prevent zombies when using direct commands
- `countSunshineProcesses()` - Verifies processes actually terminated before starting new ones

**Files Changed:** `main.go` - Added 200+ lines of process management code

---

## Issue 2: Sunrise Not Monitoring Startup Errors

**Problem:** Sunrise service started AFTER sunshine service, so it missed errors during Sunshine's startup phase.

**Impact:** Encoder failures during boot went undetected until the next check cycle (up to 10 seconds).

**Solution:** Modified service dependencies so sunrise starts BEFORE sunshine:

```ini
# sunrise.service
[Unit]
After=graphical-session.target ydotoold.service
Before=sunshine.service  # Start monitoring before sunshine boots
Wants=ydotoold.service

# sunshine.service  
[Unit]
After=sunrise.service ydotoold.service  # Wait for monitoring
Wants=sunrise.service
```

**Result:** Sunrise now monitors Sunshine from the moment it starts, catching encoder failures immediately.

**Files Changed:** `sunrise.service`, `~/.config/systemd/user/sunshine.service`

---

## Issue 3: Broken KDE Autostart Command

**Problem:** The KDE autostart file at `~/.config/autostart/dev.lizardbyte.app.Sunshine.desktop` had invalid syntax:
```bash
Exec=/usr/bin/env systemctl start --u sunshine  # WRONG: --u is invalid
```

When this command failed, KDE fell back to launching Sunshine directly as a standalone process outside systemd control.

**Impact:** Two Sunshine processes running simultaneously = port conflicts = 503 errors.

**Solution:** Fixed the autostart command:
```bash
Exec=/usr/bin/env systemctl --user start sunshine  # CORRECT
```

Also disabled `StartupNotify` to prevent timing issues.

**Files Changed:** `~/.config/autostart/dev.lizardbyte.app.Sunshine.desktop`

---

## Issue 4: Duplicate ydotoold Processes

**Problem:** Two ydotoold instances were running:
- System service (PID 2598, started Feb 7)
- User service (PID 1141131, with correct socket path)

**Impact:** Display wake commands may fail or behave unpredictably.

**Solution:** 
- Killed old system ydotoold process
- User service now uses correct socket path: `/run/user/1000/.ydotool_socket`
- Created `~/.config/systemd/user/ydotoold.service` for proper user-level management

**Files Changed:** Created `~/.config/systemd/user/ydotoold.service`

---

## Issue 5: Port 48010 Blocking

**Problem:** Zombie Sunshine process held port 48010 (RTSP server), preventing new connections.

**Error:** `Fatal: Couldn't bind RTSP server to port [48010], Address already in use`

**Solution:** The 8-step restart sequence now explicitly:
1. Kills ALL processes by PID (including zombies)
2. Verifies ports are released before starting new instance
3. Clears logs for clean start

---

## Issue 6: Display Sleep Detection

**Problem:** When the display/TV was off, Sunshine failed with "Failed to create session" but Sunrise didn't detect this as a monitor sleep error.

**Solution:** Sunrise now:
1. Monitors for "Error: Couldn't find monitor" pattern
2. Runs wake command via ydotool when display is detected as asleep
3. Waits configured seconds before allowing connections

---

## Summary of Service Architecture

Current startup order:
```
Desktop Session Start
    ↓
ydotoold.service (display wake capability)
    ↓
sunrise.service (log monitoring)
    ↓  
sunshine.service (game streaming, monitored from birth)
```

All services auto-start on login and are properly managed by systemd.

---

## Testing the Fixes

To verify everything works:

```bash
# Check all services
systemctl --user status sunshine sunrise ydotoold

# Check no zombie processes
ps aux | grep sunshine | grep -v grep

# Check ports
ss -tlnp | grep -E "4798|4799|4800|48010"

# Monitor logs
journalctl --user -u sunshine -f &
journalctl --user -u sunrise -f
```

Try connecting via Moonlight - the system should now:
1. Auto-wake display if sleeping
2. Auto-restart Sunshine on encoder failure (via systemd)
3. Properly manage all processes (no zombies)

---

## Technical Details

**Before (Broken):**
- Direct commands → zombie processes → port blocks → 503 errors
- Sunrise missed startup errors
- Autostart syntax error → standalone processes

**After (Fixed):**
- Systemd integration → proper process reaping
- Sunrise monitors from boot
- Correct autostart → single systemd-managed instance
- 8-step restart → clean process management

**Key Functions Added:**
- `systemdAvailable()` - Detect systemd availability
- `stopSunshineProperly()` / `startSunshineProperly()` - Systemd-first approach
- `getSunshinePIDs()` - Process discovery via /proc
- `killAllSunshineProcesses()` - Graduated kill strategy
- `restartSunshine()` - 8-step sequence with verification
