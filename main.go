package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

var (
	c config

	// Track the log file size and last handled error time so we only react to
	// new Sunshine errors.
	lastLogSize            int64
	lastMonitorMissingTime time.Time
	lastEncoderFailureTime time.Time
)

// config controls how sunrise functions
type config struct {
	SunriseCheckSeconds     int
	SunshineLogPath         string
	MonitorIsOffLogLine     string
	EncoderFailedLogLine    string
	EncoderFailedLogLine2   string
	WakeMonitorSleepSeconds int
	StopSunshineCommand     string
	StartSunshineCommand    string
	WakeMonitorCommand      string
	EnableSunshineRestart   bool
	RestartOnEncoderFailure bool
}

func main() {
	configPath := flag.String("config", "/etc/sunrise/sunrise.cfg", "path to the sunrise config file")
	flag.Parse()

	_, err := toml.DecodeFile(*configPath, &c)
	if err != nil {
		log.Fatal("Error reading toml config file:", err)
	}

	log.Println("Starting sunrise monitoring service")
	log.Printf("Monitor patterns: %s", c.MonitorIsOffLogLine)
	log.Printf("Encoder patterns: %s", c.EncoderFailedLogLine)
	log.Printf("Restart on encoder failure: %v", c.RestartOnEncoderFailure)

	ticker := time.NewTicker(time.Duration(c.SunriseCheckSeconds) * time.Second)
	for {
		<-ticker.C

		// Check for monitor sleep errors (wake only, no restart)
		monitorSleep, err := isMonitorSleeping()
		if err != nil {
			log.Fatal("Unable to read log file:", err)
		}
		if monitorSleep {
			log.Println("Monitor sleep detected - waking monitor only")
			err := wakeMonitor()
			if err != nil {
				log.Println("Could not wake monitor:", err)
			}
			waitForMonitor()
			continue
		}

		// Check for encoder failures (restart sunshine if enabled)
		if c.RestartOnEncoderFailure {
			encoderFailed, err := isEncoderFailed()
			if err != nil {
				log.Fatal("Unable to read log file:", err)
			}
			if encoderFailed {
				log.Println("Encoder failure detected - restarting sunshine")
				err := restartSunshine()
				if err != nil {
					log.Println("Could not restart sunshine:", err)
				}
				waitForMonitor()
			}
		}
	}
}

// isMonitorSleeping checks for monitor sleep errors
func isMonitorSleeping() (isSleeping bool, err error) {
	log.Println("Checking if monitor is sleeping")
	logInfo, err := os.Stat(c.SunshineLogPath)
	if err != nil {
		return false, err
	}

	if logInfo.Size() < lastLogSize {
		log.Println("Sunshine log appears to have rotated; resetting tracking state")
		resetMonitorTracking()
	}

	lastLogSize = logInfo.Size()

	logFile, err := os.Open(c.SunshineLogPath)
	if err != nil {
		return false, err
	}
	defer logFile.Close()

	var latestOccurrence time.Time

	scanner := bufio.NewScanner(logFile)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, c.MonitorIsOffLogLine) {
			continue
		}

		entryTime, err := parseSunshineTimestamp(line)
		if err != nil {
			log.Printf("Unable to parse timestamp: %v", err)
			continue
		}

		if entryTime.After(latestOccurrence) {
			latestOccurrence = entryTime
		}
	}

	if err := scanner.Err(); err != nil {
		if isBufferOverflow(err) {
			log.Println("Detected corrupted log with lines too long - clearing log and restarting sunshine")
			return false, handleCorruptedLog()
		}
		return false, err
	}

	if latestOccurrence.IsZero() {
		log.Println("Monitor is not sleeping")
		return false, nil
	}

	if lastMonitorMissingTime.IsZero() || latestOccurrence.After(lastMonitorMissingTime) {
		lastMonitorMissingTime = latestOccurrence
		log.Println("Monitor sleep detected at", latestOccurrence.Format(time.RFC3339Nano))
		return true, nil
	}

	log.Println("Monitor sleep already handled at", lastMonitorMissingTime.Format(time.RFC3339Nano))
	return false, nil
}

// isBufferOverflow checks if scanner error is due to token too long
func isBufferOverflow(err error) bool {
	return err != nil && strings.Contains(err.Error(), "token too long")
}

// handleCorruptedLog clears the log and restarts sunshine
func handleCorruptedLog() error {
	log.Println("Truncating corrupted sunshine log")
	if err := os.Truncate(c.SunshineLogPath, 0); err != nil {
		log.Println("Failed to truncate log:", err)
		return err
	}
	log.Println("Log truncated successfully, restarting sunshine")
	return restartSunshine()
}

// isEncoderFailed checks for encoder initialization failures
func isEncoderFailed() (failed bool, err error) {
	log.Println("Checking for encoder failures")
	logInfo, err := os.Stat(c.SunshineLogPath)
	if err != nil {
		return false, err
	}

	lastLogSize = logInfo.Size()

	logFile, err := os.Open(c.SunshineLogPath)
	if err != nil {
		return false, err
	}
	defer logFile.Close()

	var latestOccurrence time.Time

	scanner := bufio.NewScanner(logFile)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, c.EncoderFailedLogLine) &&
			!strings.Contains(line, c.EncoderFailedLogLine2) {
			continue
		}

		entryTime, err := parseSunshineTimestamp(line)
		if err != nil {
			log.Printf("Unable to parse timestamp: %v", err)
			continue
		}

		if entryTime.After(latestOccurrence) {
			latestOccurrence = entryTime
		}
	}

	if err := scanner.Err(); err != nil {
		if isBufferOverflow(err) {
			log.Println("Detected corrupted log with lines too long - clearing log and restarting sunshine")
			return false, handleCorruptedLog()
		}
		return false, err
	}

	if latestOccurrence.IsZero() {
		log.Println("No encoder failures detected")
		return false, nil
	}

	if lastEncoderFailureTime.IsZero() || latestOccurrence.After(lastEncoderFailureTime) {
		lastEncoderFailureTime = latestOccurrence
		log.Println("Encoder failure detected at", latestOccurrence.Format(time.RFC3339Nano))
		return true, nil
	}

	log.Println("Encoder failure already handled at", lastEncoderFailureTime.Format(time.RFC3339Nano))
	return false, nil
}

func wakeMonitor() (err error) {
	wakeMonitorCommandAndArgs := strings.Split(c.WakeMonitorCommand, " ")
	wakeCMD := exec.Command(wakeMonitorCommandAndArgs[0], wakeMonitorCommandAndArgs[1:]...)
	log.Println("Running wakeMonitor command:", wakeCMD.String())
	err = wakeCMD.Run()
	if err != nil {
		return err
	}
	log.Println("wakeMonitor command completed without errors")
	return nil
}

func resetMonitorTracking() {
	lastMonitorMissingTime = time.Time{}
	lastEncoderFailureTime = time.Time{}
}

func parseSunshineTimestamp(line string) (time.Time, error) {
	endIdx := strings.Index(line, "]")
	if !strings.HasPrefix(line, "[") || endIdx == -1 {
		return time.Time{}, fmt.Errorf("sunshine log line missing timestamp brackets")
	}

	timePortion := line[1:endIdx]
	t, err := time.ParseInLocation("2006-01-02 15:04:05.000", timePortion, time.Local)
	if err != nil {
		return time.Time{}, err
	}

	return t, nil
}

func waitForMonitor() {
	log.Println("Waiting", c.WakeMonitorSleepSeconds, "seconds for monitor to come up")
	time.Sleep(time.Duration(c.WakeMonitorSleepSeconds) * time.Second)
}

func restartSunshine() error {
	log.Println("=== Starting Sunshine restart sequence ===")

	if systemdAvailable() {
		log.Println("Using systemctl restart sunshine...")
		cmd := exec.Command("systemctl", "--user", "restart", "sunshine")
		if err := cmd.Run(); err != nil {
			log.Println("systemctl restart failed, falling back to manual restart:", err)
			return restartSunshineManual()
		}
		log.Println("systemctl restart completed")

		if err := waitForServiceActive("sunshine", 30); err != nil {
			return fmt.Errorf("sunshine service did not become active: %w", err)
		}

		log.Println("Clearing sunshine logs...")
		if err := os.Truncate(c.SunshineLogPath, 0); err != nil {
			log.Println("Warning: could not truncate log:", err)
		}

		return nil
	}

	return restartSunshineManual()
}

func restartSunshineManual() error {
	log.Println("Using manual process restart...")

	if err := stopSunshineProperly(); err != nil {
		log.Println("Warning: stopSunshine encountered error:", err)
	}

	log.Println("Waiting 3 seconds for graceful shutdown...")
	time.Sleep(3 * time.Second)

	if err := killAllSunshineProcesses(); err != nil {
		log.Println("Warning: killAllSunshineProcesses encountered error:", err)
	}

	waitSeconds := 5
	log.Printf("Waiting %d seconds for processes to terminate...", waitSeconds)
	time.Sleep(time.Duration(waitSeconds) * time.Second)

	if count := countSunshineProcesses(); count > 0 {
		log.Printf("Warning: %d sunshine process(es) still remain", count)
		forceKillAllSunshine()
		time.Sleep(2 * time.Second)
	}

	log.Println("Clearing sunshine logs...")
	if err := os.Truncate(c.SunshineLogPath, 0); err != nil {
		log.Println("Warning: could not truncate log:", err)
	}

	log.Println("Starting sunshine...")
	if err := startSunshineProperly(); err != nil {
		return fmt.Errorf("failed to start sunshine: %w", err)
	}

	log.Println("Waiting 5 seconds for sunshine to initialize...")
	time.Sleep(5 * time.Second)

	if count := countSunshineProcesses(); count == 0 {
		return fmt.Errorf("sunshine failed to start")
	}

	log.Printf("Sunshine restart complete")
	return nil
}

func waitForServiceActive(serviceName string, timeoutSeconds int) error {
	log.Printf("Waiting up to %d seconds for %s to be active...", timeoutSeconds, serviceName)

	for i := 0; i < timeoutSeconds; i++ {
		cmd := exec.Command("systemctl", "--user", "is-active", serviceName)
		if err := cmd.Run(); err == nil {
			log.Printf("Service %s is active", serviceName)
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %s", serviceName)
}

func stopSunshineProperly() error {
	if systemdAvailable() {
		log.Println("Stopping sunshine via systemd...")
		cmd := exec.Command("systemctl", "--user", "stop", "sunshine")
		if err := cmd.Run(); err != nil {
			log.Println("systemctl stop failed, falling back to killall:", err)
		} else {
			log.Println("systemctl stop completed")
			return nil
		}
	}

	log.Println("Stopping sunshine via configured command...")
	parts := strings.Fields(c.StopSunshineCommand)
	if len(parts) == 0 {
		return fmt.Errorf("no stop command configured")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	return cmd.Run()
}

func startSunshineProperly() error {
	if systemdAvailable() {
		log.Println("Starting sunshine via systemd...")
		cmd := exec.Command("systemctl", "--user", "start", "sunshine")
		if err := cmd.Run(); err != nil {
			log.Println("systemctl start failed, falling back to direct command:", err)
		} else {
			log.Println("systemctl start completed")
			return nil
		}
	}

	log.Println("Starting sunshine via configured command...")
	parts := strings.Fields(c.StartSunshineCommand)
	if len(parts) == 0 {
		return fmt.Errorf("no start command configured")
	}
	cmd := exec.Command(parts[0], parts[1:]...)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Sunshine process exited with error: %v", err)
		}
	}()

	return nil
}

func killAllSunshineProcesses() error {
	log.Println("Killing all sunshine processes...")

	pids := getSunshinePIDs()
	if len(pids) == 0 {
		log.Println("No sunshine processes found")
		return nil
	}

	log.Printf("Found %d sunshine process(es) to kill: %v", len(pids), pids)

	for _, pid := range pids {
		log.Printf("Sending SIGTERM to PID %d...", pid)
		if err := killProcess(pid, 15); err != nil {
			log.Printf("SIGTERM to PID %d failed: %v", pid, err)
		}
	}

	time.Sleep(2 * time.Second)

	remainingPids := getSunshinePIDs()
	if len(remainingPids) > 0 {
		log.Printf("Force killing %d remaining process(es): %v", len(remainingPids), remainingPids)
		for _, pid := range remainingPids {
			if err := killProcess(pid, 9); err != nil {
				log.Printf("SIGKILL to PID %d failed: %v", pid, err)
			}
		}
	}

	return nil
}

func forceKillAllSunshine() {
	log.Println("Force killing all sunshine processes with SIGKILL...")
	cmd := exec.Command("killall", "-9", "sunshine")
	cmd.Run()
}

func getSunshinePIDs() []int {
	var pids []int

	entries, err := os.ReadDir("/proc")
	if err != nil {
		log.Println("Could not read /proc:", err)
		return pids
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		commPath := fmt.Sprintf("/proc/%d/comm", pid)
		data, err := os.ReadFile(commPath)
		if err != nil {
			continue
		}

		processName := strings.TrimSpace(string(data))
		if processName == "sunshine" {
			pids = append(pids, pid)
		}
	}

	return pids
}

func killProcess(pid int, signal int) error {
	cmd := exec.Command("kill", fmt.Sprintf("-%d", signal), strconv.Itoa(pid))
	return cmd.Run()
}

func countSunshineProcesses() int {
	return len(getSunshinePIDs())
}

func systemdAvailable() bool {
	cmd := exec.Command("systemctl", "--user", "is-system-running")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	cmd.Run()
	status := strings.TrimSpace(out.String())
	return status == "running" || status == "degraded"
}
