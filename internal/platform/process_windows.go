//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareProcess(cmd *exec.Cmd) (func() error, func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, nil, err
	}
	// gh joins the job before it can create git or credential-helper children.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return windows.TerminateJobObject(job, 1)
	}
	started := func() error {
		process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
		if err != nil {
			return err
		}
		defer windows.CloseHandle(process)
		if err := windows.AssignProcessToJobObject(job, process); err != nil {
			return fmt.Errorf("无法管理子进程：%w", err)
		}
		resume := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")
		status, _, _ := resume.Call(uintptr(process))
		if status != 0 {
			return fmt.Errorf("恢复子进程失败：0x%x", status)
		}
		return nil
	}
	cleanup := func() {
		// Wait for the job's active-process count before the clone service can
		// remove files. TerminateProcess/TerminateJobObject are asynchronous.
		_ = windows.TerminateJobObject(job, 1)
		deadline := time.Now().Add(3 * time.Second)
		for {
			var accounting struct {
				TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
				PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
			}
			err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
			if err != nil || accounting.ActiveProcesses == 0 || time.Now().After(deadline) {
				break
			}
			time.Sleep(time.Millisecond)
		}
		windows.CloseHandle(job)
	}
	return started, cleanup, nil
}
