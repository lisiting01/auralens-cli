package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/lisiting01/auralens-cli/internal/daemon"
	"github.com/lisiting01/auralens-cli/internal/output"
	"github.com/spf13/cobra"
)

const schedulerPollInterval = 10 * time.Second

var agentScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Start a persistent scheduler that repeatedly spawns fresh agent workers",
	Long: `Run a lightweight persistent process that spawns a brand-new "auralens agent run"
instance on a fixed interval. Each worker starts with a clean context and exits
after completion, avoiding context accumulation.

The interval counts from the moment the previous worker finishes, not a fixed
wall-clock frequency, so workers never stack up.

Examples:
  auralens agent schedule --engine claude --skill ./auralens-ops.md --interval 120 --daemon
  auralens agent schedule --engine claude --prompt "Process new research" --interval 300 --daemon
  auralens agent schedule status
  auralens agent schedule stop
  auralens agent schedule stop --name my-scheduler --force`,
	RunE: runAgentScheduleStart,
}

var agentScheduleStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running schedulers",
	RunE:  runAgentScheduleStatus,
}

var agentScheduleStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a scheduler (waits for current worker to finish, or use --force)",
	RunE:  runAgentScheduleStop,
}

func initAgentScheduleCmd() {
	agentCmd.AddCommand(agentScheduleCmd)
	agentScheduleCmd.AddCommand(agentScheduleStatusCmd)
	agentScheduleCmd.AddCommand(agentScheduleStopCmd)

	agentScheduleCmd.Flags().String("engine", "claude", "AI engine: claude, codex, generic")
	agentScheduleCmd.Flags().String("engine-path", "", "Path to AI engine binary (defaults to engine name)")
	agentScheduleCmd.Flags().String("engine-model", "", "AI model name")
	agentScheduleCmd.Flags().StringP("prompt", "p", "", "Mission prompt for each worker")
	agentScheduleCmd.Flags().String("skill", "", "Path to a skill file (.md) injected as the mission")
	agentScheduleCmd.Flags().String("work-dir", "", "Working directory for each AI sub-instance")
	agentScheduleCmd.Flags().Int("interval", 300, "Seconds to wait after a worker completes before spawning the next one")
	agentScheduleCmd.Flags().String("name", "default", "Scheduler instance name (allows running multiple schedulers)")
	agentScheduleCmd.Flags().Bool("daemon", false, "Run as a background daemon")

	// Internal flags used by daemon self-re-exec.
	agentScheduleCmd.Flags().Bool("_schedulize", false, "internal: running as scheduler daemon child")
	agentScheduleCmd.Flags().String("_scheduler-name", "", "internal: scheduler name for daemon child")
	_ = agentScheduleCmd.Flags().MarkHidden("_schedulize")
	_ = agentScheduleCmd.Flags().MarkHidden("_scheduler-name")

	agentScheduleStopCmd.Flags().String("name", "", "Scheduler name to stop (empty = stop all)")
	agentScheduleStopCmd.Flags().Bool("force", false, "Kill immediately without waiting for current worker")
}

// ── start ─────────────────────────────────────────────────────────────────────

func runAgentScheduleStart(cmd *cobra.Command, args []string) error {
	_, err := requireAuth()
	if err != nil {
		return nil
	}

	engineName, _ := cmd.Flags().GetString("engine")
	enginePath, _ := cmd.Flags().GetString("engine-path")
	engineModel, _ := cmd.Flags().GetString("engine-model")
	prompt, _ := cmd.Flags().GetString("prompt")
	skillPath, _ := cmd.Flags().GetString("skill")
	workDir, _ := cmd.Flags().GetString("work-dir")
	intervalSecs, _ := cmd.Flags().GetInt("interval")
	name, _ := cmd.Flags().GetString("name")
	daemonMode, _ := cmd.Flags().GetBool("daemon")
	isChild, _ := cmd.Flags().GetBool("_schedulize")
	childName, _ := cmd.Flags().GetString("_scheduler-name")

	if prompt == "" && skillPath == "" {
		output.Error("Provide --prompt or --skill to give the agent a mission")
		return nil
	}
	if enginePath == "" {
		enginePath = engineName
	}

	if isChild {
		name = childName
	}

	ss, err := daemon.NewSchedulerState(name)
	if err != nil {
		return fmt.Errorf("create scheduler state: %w", err)
	}

	if !isChild {
		running, pid, _ := ss.IsRunning()
		if running {
			output.Warn(fmt.Sprintf("Scheduler %q already running (PID %d). Stop it first.", name, pid))
			return nil
		}
	}

	info := &daemon.SchedulerInfo{
		Name:         name,
		Engine:       engineName,
		EnginePath:   enginePath,
		EngineModel:  engineModel,
		Prompt:       prompt,
		SkillFile:    skillPath,
		WorkDir:      workDir,
		IntervalSecs: intervalSecs,
		PID:          os.Getpid(),
		StartedAt:    time.Now(),
	}

	if !isChild && daemonMode {
		return startSchedulerDaemon(cmd, info)
	}

	// Foreground or daemon-child: run the scheduler loop.
	if err := ss.WriteInfo(info); err != nil {
		return fmt.Errorf("write scheduler info: %w", err)
	}
	if err := ss.WritePID(os.Getpid()); err != nil {
		return fmt.Errorf("write scheduler pid: %w", err)
	}
	defer ss.ClearPID()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runSchedulerLoop(ctx, ss, info)
	return nil
}

// startSchedulerDaemon re-execs this binary as a detached background process.
func startSchedulerDaemon(cobraCmd *cobra.Command, info *daemon.SchedulerInfo) error {
	ss, err := daemon.NewSchedulerState(info.Name)
	if err != nil {
		return fmt.Errorf("create scheduler state: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	childArgs := []string{"agent", "schedule", "--_schedulize", "--_scheduler-name", info.Name,
		"--engine", info.Engine,
		"--interval", fmt.Sprintf("%d", info.IntervalSecs),
		"--name", info.Name,
	}
	if info.EnginePath != "" && info.EnginePath != info.Engine {
		childArgs = append(childArgs, "--engine-path", info.EnginePath)
	}
	if info.EngineModel != "" {
		childArgs = append(childArgs, "--engine-model", info.EngineModel)
	}
	if info.Prompt != "" {
		childArgs = append(childArgs, "--prompt", info.Prompt)
	}
	if info.SkillFile != "" {
		childArgs = append(childArgs, "--skill", info.SkillFile)
	}
	if info.WorkDir != "" {
		childArgs = append(childArgs, "--work-dir", info.WorkDir)
	}
	if baseURLOverride != "" {
		childArgs = append(childArgs, "--base-url", baseURLOverride)
	}

	child := exec.Command(exePath, childArgs...)
	applyDetachAttrs(child)

	logFile, err := ss.OpenLog()
	if err != nil {
		return fmt.Errorf("open scheduler log: %w", err)
	}
	child.Stdout = logFile
	child.Stderr = logFile

	if err := child.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start scheduler daemon: %w", err)
	}

	childPID := child.Process.Pid
	child.Process.Release()
	logFile.Close()

	time.Sleep(500 * time.Millisecond)
	if !processStillAlive(childPID) {
		output.Error("Scheduler exited immediately. Check logs: " + ss.LogPath())
		return nil
	}

	output.Success(fmt.Sprintf("Scheduler %q started (PID %d)", info.Name, childPID))
	fmt.Printf("  Engine:   %s\n", output.Bold(info.Engine))
	if info.EngineModel != "" {
		fmt.Printf("  Model:    %s\n", info.EngineModel)
	}
	if info.SkillFile != "" {
		fmt.Printf("  Skill:    %s\n", info.SkillFile)
	} else if info.Prompt != "" {
		p := info.Prompt
		if len(p) > 80 {
			p = p[:80] + "..."
		}
		fmt.Printf("  Prompt:   %s\n", p)
	}
	fmt.Printf("  Interval: every %ds after completion\n", info.IntervalSecs)
	fmt.Printf("  Logs:     %s\n", ss.LogPath())
	fmt.Printf("  Stop:     auralens agent schedule stop --name %s\n", info.Name)
	return nil
}

// ── scheduler loop ────────────────────────────────────────────────────────────

// runSchedulerLoop is the core scheduler logic that runs inside the daemon child.
// It spawns a fresh "auralens agent run" worker on the configured interval,
// waits for completion, then waits interval seconds before spawning again.
func runSchedulerLoop(ctx context.Context, ss *daemon.SchedulerState, info *daemon.SchedulerInfo) {
	logf := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "[scheduler:%s] "+format+"\n", append([]any{info.Name}, a...)...)
	}

	exePath, err := os.Executable()
	if err != nil {
		logf("cannot resolve executable: %v", err)
		return
	}

	ticker := time.NewTicker(schedulerPollInterval)
	defer ticker.Stop()

	type workerResult struct {
		completedAt time.Time
		err         error
	}
	var (
		workerDone    chan workerResult
		currentCancel context.CancelFunc
	)

	spawnWorker := func() {
		workerID := daemon.GenerateWorkerID()
		info.CurrentWorkerID = workerID
		_ = ss.WriteInfo(info)

		workerArgs := buildSchedulerWorkerArgs(exePath, workerID, info)
		logf("round %d: spawning worker %s", info.Round+1, workerID)

		ww, err := daemon.NewWorkerState(workerID)
		if err != nil {
			logf("create worker state: %v", err)
			info.CurrentWorkerID = ""
			_ = ss.WriteInfo(info)
			return
		}

		wlogFile, err := ww.OpenLog()
		if err != nil {
			logf("open worker log: %v", err)
			info.CurrentWorkerID = ""
			_ = ss.WriteInfo(info)
			return
		}

		workerCtx, cancel := context.WithCancel(context.Background())
		currentCancel = cancel

		child := exec.CommandContext(workerCtx, workerArgs[0], workerArgs[1:]...)
		child.Stdout = wlogFile
		child.Stderr = wlogFile

		if err := child.Start(); err != nil {
			wlogFile.Close()
			cancel()
			logf("start worker: %v", err)
			info.CurrentWorkerID = ""
			_ = ss.WriteInfo(info)
			return
		}

		logf("worker %s started (PID %d), log: %s", workerID, child.Process.Pid, ww.LogPath())

		workerDone = make(chan workerResult, 1)
		go func() {
			defer wlogFile.Close()
			waitErr := child.Wait()
			workerDone <- workerResult{completedAt: time.Now(), err: waitErr}
		}()
	}

	shouldSpawnNow := func() bool {
		if info.CurrentWorkerID != "" {
			return false
		}
		if info.LastCompletedAt == nil {
			return true
		}
		return time.Since(*info.LastCompletedAt) >= time.Duration(info.IntervalSecs)*time.Second
	}

	if shouldSpawnNow() {
		spawnWorker()
	}

	for {
		select {
		case <-ctx.Done():
			// Graceful shutdown: wait for the current worker to finish.
			if workerDone != nil {
				logf("shutting down, waiting for current worker to finish...")
				res := <-workerDone
				if res.err != nil {
					logf("worker finished with error: %v", res.err)
				} else {
					logf("worker finished cleanly")
				}
			}
			if currentCancel != nil {
				currentCancel()
			}
			return

		case res := <-workerDone:
			if currentCancel != nil {
				currentCancel()
				currentCancel = nil
			}
			workerDone = nil
			now := res.completedAt
			info.LastCompletedAt = &now
			info.Round++
			info.CurrentWorkerID = ""
			_ = ss.WriteInfo(info)
			if res.err != nil {
				logf("round %d finished with error: %v (next in %ds)", info.Round, res.err, info.IntervalSecs)
			} else {
				logf("round %d completed (next in %ds)", info.Round, info.IntervalSecs)
			}

		case <-ticker.C:
			if shouldSpawnNow() {
				spawnWorker()
			} else if info.CurrentWorkerID == "" && info.LastCompletedAt != nil {
				remaining := time.Duration(info.IntervalSecs)*time.Second - time.Since(*info.LastCompletedAt)
				if remaining > 0 {
					logf("idle, next spawn in %s", remaining.Round(time.Second))
				}
			}
		}
	}
}

// buildSchedulerWorkerArgs constructs the argument list for a worker child process.
func buildSchedulerWorkerArgs(exePath, workerID string, info *daemon.SchedulerInfo) []string {
	args := []string{exePath, "agent", "run",
		"--_daemonize",
		"--_worker-id", workerID,
		"--engine", info.Engine,
	}
	if info.EnginePath != "" && info.EnginePath != info.Engine {
		args = append(args, "--engine-path", info.EnginePath)
	}
	if info.EngineModel != "" {
		args = append(args, "--engine-model", info.EngineModel)
	}
	if info.Prompt != "" {
		args = append(args, "--prompt", info.Prompt)
	}
	if info.SkillFile != "" {
		args = append(args, "--skill", info.SkillFile)
	}
	if info.WorkDir != "" {
		args = append(args, "--work-dir", info.WorkDir)
	}
	if baseURLOverride != "" {
		args = append(args, "--base-url", baseURLOverride)
	}
	return args
}

// ── status ────────────────────────────────────────────────────────────────────

func runAgentScheduleStatus(cmd *cobra.Command, args []string) error {
	schedulers, err := daemon.ListSchedulers()
	if err != nil {
		return err
	}
	if len(schedulers) == 0 {
		if outputJSON {
			fmt.Println("[]")
			return nil
		}
		fmt.Println(output.Faint("No schedulers found."))
		return nil
	}

	type schedStatus struct {
		Name            string `json:"name"`
		IntervalSecs    int    `json:"interval_secs"`
		Status          string `json:"status"`
		Round           int    `json:"round"`
		LastCompletedAt string `json:"last_completed_at,omitempty"`
		NextStart       string `json:"next_start,omitempty"`
		PID             int    `json:"pid,omitempty"`
		LogPath         string `json:"log_path"`
	}

	var rows []schedStatus
	for _, ss := range schedulers {
		info, _ := ss.ReadInfo()
		running, pid, _ := ss.IsRunning()

		if info == nil && !running {
			continue
		}

		row := schedStatus{LogPath: ss.LogPath()}
		if info != nil {
			row.Name = info.Name
			row.IntervalSecs = info.IntervalSecs
			row.Round = info.Round
			if info.LastCompletedAt != nil {
				row.LastCompletedAt = info.LastCompletedAt.Format("2006-01-02 15:04:05")
				remaining := time.Duration(info.IntervalSecs)*time.Second - time.Since(*info.LastCompletedAt)
				if running {
					if info.CurrentWorkerID != "" {
						row.Status = "running"
						row.NextStart = "-"
					} else if remaining > 0 {
						row.Status = "idle"
						row.NextStart = fmt.Sprintf("in %s", remaining.Round(time.Second))
					} else {
						row.Status = "idle"
						row.NextStart = "imminent"
					}
				} else {
					row.Status = "stopped"
				}
			} else {
				if running {
					row.Status = "running"
					row.NextStart = "-"
				} else {
					row.Status = "stopped"
				}
			}
		} else {
			row.Name = ss.Name()
			if running {
				row.Status = "running"
			} else {
				row.Status = "stopped"
			}
		}
		if running {
			row.PID = pid
		}
		rows = append(rows, row)
	}

	if outputJSON {
		return output.JSON(rows)
	}
	if len(rows) == 0 {
		fmt.Println(output.Faint("No schedulers found."))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tINTERVAL\tSTATUS\tROUND\tLAST COMPLETED\tNEXT START")
	for _, r := range rows {
		interval := fmt.Sprintf("%ds", r.IntervalSecs)
		lc := r.LastCompletedAt
		if lc == "" {
			lc = "-"
		}
		ns := r.NextStart
		if ns == "" {
			ns = "-"
		}
		statusStr := r.Status
		switch r.Status {
		case "running":
			statusStr = output.Green("running")
		case "idle":
			statusStr = output.Yellow("idle")
		case "stopped":
			statusStr = output.Faint("stopped")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			r.Name, interval, statusStr, r.Round, lc, ns)
	}
	w.Flush()
	return nil
}

// ── stop ──────────────────────────────────────────────────────────────────────

func runAgentScheduleStop(cmd *cobra.Command, args []string) error {
	targetName, _ := cmd.Flags().GetString("name")
	force, _ := cmd.Flags().GetBool("force")

	schedulers, err := daemon.ListSchedulers()
	if err != nil {
		return err
	}
	if len(schedulers) == 0 {
		output.Warn("No schedulers found")
		return nil
	}

	stopped := 0
	for _, ss := range schedulers {
		info, _ := ss.ReadInfo()
		name := ss.Name()
		if info != nil {
			name = info.Name
		}

		if targetName != "" && name != targetName {
			continue
		}

		running, pid, _ := ss.IsRunning()
		if !running {
			if targetName != "" {
				output.Warn(fmt.Sprintf("Scheduler %q is not running", name))
			}
			_ = ss.Cleanup()
			continue
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			output.Warn(fmt.Sprintf("Scheduler %q: find process %d: %v", name, pid, err))
			continue
		}

		if force {
			if err := terminateProcess(proc); err != nil {
				output.Warn(fmt.Sprintf("Scheduler %q: terminate PID %d: %v", name, pid, err))
				continue
			}
			_ = ss.ClearPID()
			output.Success(fmt.Sprintf("Scheduler %q force-stopped (PID %d)", name, pid))
		} else {
			// Graceful: send SIGTERM and let the scheduler finish the current worker.
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				// Fallback to terminate on Windows where SIGTERM may not be supported.
				if err2 := terminateProcess(proc); err2 != nil {
					output.Warn(fmt.Sprintf("Scheduler %q: signal PID %d: %v", name, pid, err))
					continue
				}
			}
			output.Success(fmt.Sprintf("Scheduler %q signalled to stop gracefully (PID %d)", name, pid))
			fmt.Println("  It will finish the current worker round before exiting. Use --force to stop immediately.")
		}
		stopped++
	}

	if stopped == 0 && targetName != "" {
		output.Warn(fmt.Sprintf("Scheduler %q not found or not running", targetName))
	} else if stopped == 0 {
		output.Warn("No running schedulers to stop")
	}
	return nil
}
