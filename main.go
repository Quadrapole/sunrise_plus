package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
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
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, c.EncoderFailedLogLine) {
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

func restartSunshine() (err error) {
	stopSunshine()
	err = startSunshine()
	if err != nil {
		return err
	}
	return nil
}

func stopSunshine() {
	stopSunshineCommandAndArgs := strings.Split(c.StopSunshineCommand, " ")
	stopSunshineCMD := exec.Command(stopSunshineCommandAndArgs[0], stopSunshineCommandAndArgs[1:]...)
	log.Println("Running stopSunshine command:", stopSunshineCMD.String())
	err := stopSunshineCMD.Run()
	if err != nil {
		log.Println("stopSunshine encountered an error - ignoring:", err)
	}
	log.Println("stopSunshine command completed without errors")
}

func startSunshine() (err error) {
	startSunshineCommandAndArgs := strings.Split(c.StartSunshineCommand, " ")
	startSunshineCMD := exec.Command(startSunshineCommandAndArgs[0], startSunshineCommandAndArgs[1:]...)
	log.Println("Running startSunshine command:", startSunshineCMD.String())
	err = startSunshineCMD.Start()
	if err != nil {
		return err
	}
	log.Println("startSunshine command completed without errors")
	return nil
}
