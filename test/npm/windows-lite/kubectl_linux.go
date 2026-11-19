package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	outputLimit        = 4 << 20
	convergenceLimit   = 90 * time.Second
	roleReadinessLimit = 10 * time.Minute
	pollInterval       = 5 * time.Second
	consecutiveRounds  = 3
	apiVersionKey      = "apiVersion"
	nameKey            = "name"
	kindKey            = "kind"
)

var (
	errOutputLimit = errors.New("windows-lite: command output exceeded 4 MiB")
	errCommand     = errors.New("windows-lite: command failed")
)

type boundedBuffer struct {
	buffer   bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > outputLimit {
		b.overflow = true
		return 0, errOutputLimit
	}
	_, _ = b.buffer.Write(p)
	return len(p), nil
}

type commandResult struct {
	stdout []byte
	stderr []byte
	code   int
}

type commandFunc func(context.Context, []string, []byte) (commandResult, error)

func invokeKubectl(ctx context.Context, args []string, input []byte) (commandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var groupMu sync.Mutex
	groupFinished := false
	terminateGroup := func() error {
		if groupFinished {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		if err != nil {
			return fmt.Errorf("terminate kubectl process group: %w", err)
		}
		return nil
	}
	cmd.Cancel = func() error {
		groupMu.Lock()
		defer groupMu.Unlock()
		return terminateGroup()
	}
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr boundedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Start()
	var cleanupErr error
	if err == nil {
		observedErr := waitWithoutReaping(cmd.Process.Pid)
		groupMu.Lock()
		var terminateErr error
		if !errors.Is(observedErr, syscall.ECHILD) {
			terminateErr = terminateGroup()
		}
		// Disable cancellation before Wait can release the group's reserved PID.
		groupFinished = true
		groupMu.Unlock()
		if errors.Is(terminateErr, os.ErrProcessDone) {
			terminateErr = nil
		}
		cleanupErr = errors.Join(observedErr, terminateErr)
		err = cmd.Wait()
	}
	result := commandResult{stdout: stdout.buffer.Bytes(), stderr: stderr.buffer.Bytes()}
	if cleanupErr != nil {
		return result, fmt.Errorf("kubectl process cleanup: %w", errors.Join(cleanupErr, err, ctx.Err()))
	}
	if ctx.Err() != nil {
		return result, fmt.Errorf("kubectl canceled: %w", ctx.Err())
	}
	if stdout.overflow || stderr.overflow {
		return result, errOutputLimit
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.code = exitErr.ExitCode()
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("run kubectl: %w", err)
	}
	return result, nil
}

func waitWithoutReaping(pid int) error {
	const waitPID = 1
	// Linux siginfo_t is 128 bytes; WNOWAIT keeps the group leader's PID reserved.
	var siginfo [16]uint64
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, waitPID, uintptr(pid),
			uintptr(unsafe.Pointer(&siginfo[0])), syscall.WEXITED|syscall.WNOWAIT, 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return fmt.Errorf("observe kubectl exit: %w", errno)
		}
		return nil
	}
}

type kubeClient struct {
	kubeconfig string
	command    commandFunc
}

func (k kubeClient) invoke(ctx context.Context, input []byte, args ...string) (commandResult, error) {
	return k.command(ctx, append([]string{"--kubeconfig", k.kubeconfig, "--request-timeout=25s"}, args...), input)
}

func (k kubeClient) checked(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	result, err := k.invoke(ctx, input, args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl %s: %w", args[0], err)
	}
	if result.code != 0 {
		message := strings.TrimSpace(string(result.stderr))
		if len(message) > 4096 {
			message = message[:4096]
		}
		return nil, fmt.Errorf("%w: kubectl %s exited %d: %s", errCommand, args[0], result.code, message)
	}
	return result.stdout, nil
}

func getJSON[T any](ctx context.Context, k kubeClient, args ...string) (T, error) {
	var value T
	data, err := k.checked(ctx, nil, append(args, "-o", "json")...)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("decoding kubectl %s: %w", args[0], err)
	}
	return value, nil
}

func createJSON[T any](ctx context.Context, k kubeClient, object any) (T, error) {
	var value T
	input, err := json.Marshal(object)
	if err != nil {
		return value, fmt.Errorf("encode Kubernetes object: %w", err)
	}
	data, err := k.checked(ctx, input, "create", "-f", "-", "-o", "json")
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("decoding created object: %w", err)
	}
	return value, nil
}

func (k kubeClient) deleteUID(ctx context.Context, apiPath, uid string) error {
	input, err := json.Marshal(map[string]any{
		apiVersionKey: "v1", kindKey: "DeleteOptions",
		"preconditions": map[string]string{"uid": uid},
	})
	if err != nil {
		return fmt.Errorf("encode UID precondition: %w", err)
	}
	_, err = k.checked(ctx, input, "delete", "--raw", apiPath, "-f", "-")
	return err
}

func converge(ctx context.Context, limit time.Duration, rounds int, check func(context.Context) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	timer := time.NewTimer(pollInterval)
	defer timer.Stop()
	streak := 0
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("convergence deadline: %w", err)
		}
		ok, err := check(ctx)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("convergence deadline: %w", err)
		}
		if ok {
			streak++
			if streak == rounds {
				return nil
			}
		} else {
			streak = 0
		}
		timer.Reset(pollInterval)
		select {
		case <-ctx.Done():
			return fmt.Errorf("convergence deadline: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func parseProbe(result commandResult) (verdict, error) {
	if result.code == 0 && len(result.stdout) == 0 && len(result.stderr) == 0 {
		return allow, nil
	}
	// kubectl appends this ExitError after the remote process's stderr.
	if result.code == 1 && len(result.stdout) == 0 {
		for _, reason := range []string{"TIMEOUT", "REFUSED"} {
			if string(result.stderr) == reason+"\ncommand terminated with exit code 1\n" {
				return deny, nil
			}
		}
	}
	return "", fmt.Errorf("%w: probe exit %d, stdout %q, stderr %q",
		errCommand, result.code, result.stdout[:min(len(result.stdout), 512)], result.stderr[:min(len(result.stderr), 512)])
}

type objectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	UID               string            `json:"uid"`
	Generation        int64             `json:"generation"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	DeletionTimestamp *time.Time        `json:"deletionTimestamp"`
	Labels            map[string]string `json:"labels"`
	OwnerReferences   []ownerReference  `json:"ownerReferences"`
}

type ownerReference struct {
	UID        string `json:"uid"`
	Kind       string `json:"kind"`
	Controller *bool  `json:"controller"`
}

func hasController(owners []ownerReference, uid string) bool {
	for _, owner := range owners {
		if owner.UID == uid && owner.Kind == "DaemonSet" && owner.Controller != nil && *owner.Controller {
			return true
		}
	}
	return false
}

type resource struct {
	Metadata objectMeta `json:"metadata"`
}

type objectList[T any] struct {
	Items []T `json:"items"`
}

type condition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type windowsOptions struct {
	HostProcess   *bool  `json:"hostProcess"`
	RunAsUserName string `json:"runAsUserName"`
}

type securityContext struct {
	WindowsOptions windowsOptions `json:"windowsOptions"`
}

type containerSpec struct {
	Name            string          `json:"name"`
	Image           string          `json:"image"`
	Command         []string        `json:"command"`
	Args            []string        `json:"args"`
	SecurityContext securityContext `json:"securityContext"`
	Ports           []struct {
		Name          string `json:"name"`
		Protocol      string `json:"protocol"`
		ContainerPort int    `json:"containerPort"`
	} `json:"ports"`
	VolumeMounts []struct {
		Name        string `json:"name"`
		MountPath   string `json:"mountPath"`
		SubPath     string `json:"subPath"`
		SubPathExpr string `json:"subPathExpr"`
		ReadOnly    bool   `json:"readOnly"`
	} `json:"volumeMounts"`
}

type podSpec struct {
	NodeName        string            `json:"nodeName"`
	NodeSelector    map[string]string `json:"nodeSelector"`
	HostNetwork     bool              `json:"hostNetwork"`
	SecurityContext securityContext   `json:"securityContext"`
	Containers      []containerSpec   `json:"containers"`
	InitContainers  []containerSpec   `json:"initContainers"`
	Volumes         []struct {
		Name      string `json:"name"`
		ConfigMap *struct {
			Name  string `json:"name"`
			Items []struct {
				Key  string `json:"key"`
				Path string `json:"path"`
			} `json:"items"`
		} `json:"configMap"`
	} `json:"volumes"`
}

type pod struct {
	Metadata objectMeta `json:"metadata"`
	Spec     podSpec    `json:"spec"`
	Status   struct {
		Phase             string      `json:"phase"`
		PodIP             string      `json:"podIP"`
		Conditions        []condition `json:"conditions"`
		ContainerStatuses []struct {
			Name         string `json:"name"`
			ContainerID  string `json:"containerID"`
			ImageID      string `json:"imageID"`
			Ready        bool   `json:"ready"`
			RestartCount int    `json:"restartCount"`
			State        struct {
				Running *struct {
					StartedAt time.Time `json:"startedAt"`
				} `json:"running"`
			} `json:"state"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

type node struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Unschedulable bool `json:"unschedulable"`
	} `json:"spec"`
	Status struct {
		Conditions []condition `json:"conditions"`
		NodeInfo   struct {
			OperatingSystem string `json:"operatingSystem"`
			Architecture    string `json:"architecture"`
			KernelVersion   string `json:"kernelVersion"`
			OSImage         string `json:"osImage"`
		} `json:"nodeInfo"`
	} `json:"status"`
}

type daemonSet struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Selector struct {
			MatchLabels      map[string]string `json:"matchLabels"`
			MatchExpressions []json.RawMessage `json:"matchExpressions"`
		} `json:"selector"`
		Template struct {
			Spec podSpec `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		ObservedGeneration int64 `json:"observedGeneration"`
		Desired            int   `json:"desiredNumberScheduled"`
		Ready              int   `json:"numberReady"`
		Updated            int   `json:"updatedNumberScheduled"`
		Unavailable        int   `json:"numberUnavailable"`
	} `json:"status"`
}

func isReady(conditions []condition) bool {
	for _, value := range conditions {
		if value.Type == "Ready" {
			return value.Status == "True"
		}
	}
	return false
}
